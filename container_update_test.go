package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestReplacementPreservesDeploymentSettings(t *testing.T) {
	var old inspectedContainer
	if err := json.Unmarshal([]byte(`{"Id":"old","State":{"Running":true},"Config":{"Image":"old-image","Hostname":"generated","Entrypoint":["/k9s-web-ui"],"Cmd":["serve","--username","operator","--context","example"],"Env":["TZ=UTC"],"Labels":{"owner":"example"}},"HostConfig":{"NetworkMode":"host","ReadonlyRootfs":true,"Memory":268435456,"CapDrop":["ALL"],"RestartPolicy":{"Name":"unless-stopped"},"Mounts":[{"Source":"/private/socket","Target":"/host","Type":"bind","ReadOnly":true},{"Source":"/private/state","Target":"/state","Type":"bind"}]},"Mounts":[{"Type":"bind","Source":"/private/socket","Destination":"/host"},{"Type":"bind","Source":"/private/state","Destination":"/state"}]}`), &old); err != nil {
		t.Fatal(err)
	}
	payload, err := replacementConfig(old, "new-image")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"Cmd", "Env", "Labels", "Entrypoint"} {
		if !reflect.DeepEqual(payload[key], old.Config[key]) {
			t.Errorf("changed %s", key)
		}
	}
	if !reflect.DeepEqual(payload["HostConfig"], old.HostConfig) {
		t.Fatal("lost security settings, limits, mounts or restart policy")
	}
	if string(payload["Image"]) != `"new-image"` || string(old.Config["Image"]) != `"old-image"` {
		t.Fatal("image replacement mutated original config")
	}
	old.State.Running = false
	if _, err := replacementConfig(old, "new-image"); err == nil {
		t.Fatal("accepted stopped container")
	}
}

func TestContainerUpdateRollback(t *testing.T) {
	for _, scenario := range []string{"success", "create-failure", "start-failure", "readiness-failure"} {
		t.Run(scenario, func(t *testing.T) {
			var operations []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				op := r.Method + " " + r.URL.Path
				operations = append(operations, op)
				if (scenario == "create-failure" && r.URL.Path == "/containers/create") || (scenario == "start-failure" && r.URL.Path == "/containers/new/start") {
					w.WriteHeader(500)
					w.Write([]byte(`{"message":"injected failure"}`))
					return
				}
				if r.URL.Path == "/containers/create" {
					w.WriteHeader(201)
					w.Write([]byte(`{"Id":"new"}`))
					return
				}
				w.WriteHeader(204)
			}))
			defer server.Close()
			d := dockerAPI{client: server.Client(), base: server.URL}
			err := replaceContainer(d, "dashboard", inspectedContainer{ID: "old"}, map[string]string{"Image": "target"}, func() error {
				if scenario == "readiness-failure" {
					return errors.New("new dashboard did not start")
				}
				return nil
			})
			joined := strings.Join(operations, "\n")
			if scenario == "success" {
				if err != nil || !strings.Contains(joined, "DELETE /containers/old") || strings.Contains(joined, "POST /containers/old/start") {
					t.Fatalf("success path: %v, %s", err, joined)
				}
			} else if scenario == "create-failure" {
				if err == nil || len(operations) != 1 {
					t.Fatalf("changed running container after create failure: %v %s", err, joined)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), "previous container restored") || strings.Contains(joined, "DELETE /containers/old") {
					t.Fatalf("rollback lost old container: %v %s", err, joined)
				}
				wantTail := []string{"DELETE /containers/new", "POST /containers/old/rename", "POST /containers/old/start"}
				if !reflect.DeepEqual(operations[len(operations)-3:], wantTail) {
					t.Fatalf("incorrect rollback: %s", joined)
				}
			}
		})
	}
}
