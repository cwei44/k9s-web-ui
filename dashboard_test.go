// Copyright 2026 K9s Web UI contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSnapshotUsesOneContextAndReportsPartialErrors(t *testing.T) {
	var mu sync.Mutex
	selected := "cluster-a"
	d := &dashboard{}
	d.run = func(o options, args ...string) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		if strings.Join(args, " ") == "config current-context" {
			return []byte(selected), nil
		}
		if o.Context != selected {
			t.Errorf("snapshot request not pinned: %q", o.Context)
		}
		if args[len(args)-1] == "/version" {
			return []byte(`{"gitVersion":"v1.35.9"}`), nil
		}
		if args[1] == "deployments.apps" {
			return nil, errors.New("forbidden")
		}
		return []byte(`{"items":[{"metadata":{"name":"test"}}]}`), nil
	}
	s := d.getSnapshot()
	if !s.Connected || s.Context != "cluster-a" || len(s.Data["Pods"]) != 1 || s.Errors["Deployments"] != "forbidden" {
		t.Fatalf("snapshot: %+v", s)
	}
	selected = "cluster-b"
	d.checked = time.Now().Add(-time.Minute)
	if d.getSnapshot().Context != "cluster-b" {
		t.Fatal("did not follow host context change")
	}
	d.checked = time.Time{}
	d.run = func(options, ...string) ([]byte, error) { return nil, errors.New("Unauthorized") }
	s = d.getSnapshot()
	if s.Connected || len(s.Data["Pods"]) != 0 || s.Errors["connection"] != "Unauthorized" {
		t.Fatal("connection failure leaked previous data")
	}
}

func TestReadOnlyAPIAndParameterValidation(t *testing.T) {
	calls := []string{}
	d := &dashboard{options: options{Context: "test", Namespace: "allowed"}}
	d.run = func(o options, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		return []byte("test result"), nil
	}
	g := newGPUCache(options{})
	g.collect = func(options) gpuSnapshot { return gpuSnapshot{Devices: []device{}, Errors: map[string]string{}} }
	handler := auth(d.handler(g), "admin", "test-password")
	cases := []struct {
		method, path string
		status       int
	}{
		{"GET", "/k9s/", 200},
		{"GET", "/k9s/gpu.js", 200},
		{"POST", "/k9s/api/manifest?kind=Pods&name=test&namespace=allowed", 405},
		{"GET", "/k9s/api/manifest?kind=Secrets&name=test&namespace=allowed", 400},
		{"GET", "/k9s/api/manifest?kind=Pods&name=--help&namespace=allowed", 400},
		{"GET", "/k9s/api/manifest?kind=Pods&name=test&namespace=other", 403},
		{"GET", "/k9s/api/logs?kind=Nodes&name=test", 400},
		{"GET", "/k9s/api/manifest?kind=Pods&name=test&namespace=allowed", 200},
		{"GET", "/k9s/api/logs?kind=Pods&name=test&namespace=allowed", 200},
		{"GET", "/k9s/api/unknown", 404},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.SetBasicAuth("admin", "test-password")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if tc.path == "/k9s/" && !strings.Contains(w.Body.String(), "/k9s/gpu.js") {
			t.Error("installer readiness marker missing from embedded HTML")
		}
		if w.Code != tc.status {
			t.Errorf("%s %s: got %d, want %d", tc.method, tc.path, w.Code, tc.status)
		}
	}
	if len(calls) != 2 || calls[0] != "get pods test -o yaml -n allowed" || !strings.Contains(calls[1], "--limit-bytes=1048576") {
		t.Fatalf("unexpected commands: %v", calls)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/k9s/api/gpu", nil))
	if w.Code != 401 {
		t.Fatal("GPU API is not authenticated")
	}
}

func TestGPUContextSwitchDoesNotReuseOtherCluster(t *testing.T) {
	c := newGPUCache(options{})
	c.value = gpuSnapshot{Context: "old", Devices: []device{{GPU: "0"}}}
	c.collect = func(options) gpuSnapshot {
		return gpuSnapshot{Context: "new", Devices: []device{}, Errors: map[string]string{"discovery": "unavailable"}}
	}
	result := c.get()
	if result.Stale || len(result.Devices) != 0 || result.Context != "new" {
		t.Fatal("old-cluster devices were reused")
	}
	b, e := json.Marshal(result)
	if e != nil || !strings.Contains(string(b), `"devices":[]`) {
		t.Fatal("empty GPU list must be an array")
	}
}
