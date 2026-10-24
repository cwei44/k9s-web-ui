// Copyright 2026 K9s Web UI contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

type dockerAPI struct {
	client *http.Client
	base   string
}

func (d dockerAPI) request(method, path string, body any, result any) error {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequest(method, d.base+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var problem struct{ Message string }
		json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&problem)
		return fmt.Errorf("Docker %s %s: HTTP %d: %s", method, path, response.StatusCode, problem.Message)
	}
	if result != nil {
		return json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(result)
	}
	return nil
}

func localDockerAPI() (dockerAPI, func(), error) {
	endpoint := os.Getenv("DOCKER_HOST")
	if endpoint == "" {
		// Docker 18 has no contexts; current versions can select a local socket.
		if value, err := command("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}"); err == nil {
			endpoint = strings.TrimSpace(value)
		} else {
			endpoint = "unix:///var/run/docker.sock"
		}
	}
	if !strings.HasPrefix(endpoint, "unix:///") {
		return dockerAPI{}, nil, errors.New("updates require a local Linux Docker Unix socket")
	}
	socket := strings.TrimPrefix(endpoint, "unix://")
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socket)
	}}
	d := dockerAPI{client: &http.Client{Transport: transport, Timeout: 60 * time.Second}, base: "http://docker"}
	close := transport.CloseIdleConnections
	var version struct{ APIVersion string }
	if err := d.request("GET", "/version", nil, &version); err != nil {
		close()
		return dockerAPI{}, nil, err
	}
	if !regexp.MustCompile(`^1\.\d+$`).MatchString(version.APIVersion) {
		close()
		return dockerAPI{}, nil, errors.New("invalid Docker API version")
	}
	d.base += "/v" + version.APIVersion
	return d, close, nil
}

type inspectedContainer struct {
	ID         string `json:"Id"`
	Config     map[string]json.RawMessage
	HostConfig json.RawMessage
	State      struct{ Running bool }
	Mounts     []struct{ Type, Source, Destination string }
}

// Clone the actual Docker settings, including mounts, limits, environment,
// command, authentication flags and restart policy. Only the image changes.
func replacementConfig(old inspectedContainer, image string) (map[string]json.RawMessage, error) {
	var host struct {
		NetworkMode string
		AutoRemove  bool
	}
	if err := json.Unmarshal(old.HostConfig, &host); err != nil {
		return nil, err
	}
	if host.NetworkMode != "host" || host.AutoRemove {
		return nil, errors.New("update expects the installer's host-network container without auto-remove")
	}
	if !old.State.Running || old.ID == "" {
		return nil, errors.New("start the existing dashboard container before updating")
	}
	var entrypoint []string
	if json.Unmarshal(old.Config["Entrypoint"], &entrypoint) != nil || len(entrypoint) != 1 || entrypoint[0] != "/k9s-web-ui" {
		return nil, errors.New("container does not use the managed dashboard entrypoint")
	}
	if len(old.Mounts) != 2 {
		return nil, errors.New("expected only the dashboard state and host-helper bind mounts")
	}
	seen := map[string]bool{}
	for _, mount := range old.Mounts {
		if mount.Type != "bind" || (mount.Destination != "/host" && mount.Destination != "/state") {
			return nil, errors.New("unrecognized container mounts")
		}
		seen[mount.Destination] = true
	}
	if !seen["/host"] || !seen["/state"] {
		return nil, errors.New("host-helper or dashboard-state mount is missing")
	}
	payload := make(map[string]json.RawMessage, len(old.Config)+1)
	for k, value := range old.Config {
		payload[k] = value
	}
	payload["Image"], _ = json.Marshal(image)
	// Docker fills these during inspect; host networking must inherit them anew.
	payload["Hostname"] = json.RawMessage(`""`)
	payload["Domainname"] = json.RawMessage(`""`)
	payload["HostConfig"] = old.HostConfig
	return payload, nil
}

func replaceContainer(d dockerAPI, name string, old inspectedContainer, payload any, ready func() error) (err error) {
	suffix := fmt.Sprint(time.Now().UnixNano())
	var created struct {
		ID string `json:"Id"`
	}
	if err = d.request("POST", "/containers/create?name="+url.QueryEscape(name+"-update-"+suffix), payload, &created); err != nil {
		return err
	}
	if created.ID == "" {
		return errors.New("Docker did not return a replacement container ID")
	}
	stopped, renamed := false, false
	defer func() {
		if err == nil {
			return
		}
		var recovery []error
		if e := d.request("DELETE", "/containers/"+created.ID+"?force=true", nil, nil); e != nil {
			recovery = append(recovery, e)
		}
		if renamed {
			if e := d.request("POST", "/containers/"+old.ID+"/rename?name="+url.QueryEscape(name), nil, nil); e != nil {
				recovery = append(recovery, e)
			}
		}
		if stopped {
			if e := d.request("POST", "/containers/"+old.ID+"/start", nil, nil); e != nil {
				recovery = append(recovery, e)
			}
		}
		if len(recovery) > 0 {
			err = fmt.Errorf("%w; automatic rollback also reported: %v; previous container ID: %s", err, errors.Join(recovery...), old.ID)
		} else {
			err = fmt.Errorf("%w; previous container restored", err)
		}
	}()
	// Mark before the call so a lost response still attempts recovery.
	stopped = true
	if err = d.request("POST", "/containers/"+old.ID+"/stop?t=10", nil, nil); err != nil {
		return err
	}
	renamed = true
	if err = d.request("POST", "/containers/"+old.ID+"/rename?name="+url.QueryEscape(name+"-previous-"+suffix), nil, nil); err != nil {
		return err
	}
	if err = d.request("POST", "/containers/"+created.ID+"/rename?name="+url.QueryEscape(name), nil, nil); err != nil {
		return err
	}
	if err = d.request("POST", "/containers/"+created.ID+"/start", nil, nil); err != nil {
		return err
	}
	if err = ready(); err != nil {
		return err
	}
	// A cleanup error must not undo a successfully verified update.
	if e := d.request("DELETE", "/containers/"+old.ID, nil, nil); e != nil {
		fmt.Fprintln(os.Stderr, "Update succeeded; old stopped container needs cleanup:", old.ID, e)
	}
	return nil
}

func updateContainer(args []string) error {
	if os.Geteuid() != 0 {
		return errors.New("container-update must run as root on the Docker host")
	}
	flags := flag.NewFlagSet("container-update", flag.ContinueOnError)
	image := flags.String("image", "k9s-web-ui:local", "Locally built or loaded replacement image")
	prefix := flags.String("prefix", "/opt/k9s-web-ui-container", "Existing installation directory")
	name := flags.String("container", managedContainer, "Existing managed container name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(*prefix) || !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`).MatchString(*name) {
		return errors.New("invalid update arguments")
	}
	metadata := filepath.Join(*prefix, "etc", "container.json")
	data, err := os.ReadFile(metadata)
	if err != nil {
		return fmt.Errorf("read existing installation metadata: %w", err)
	}
	var state containerState
	if err = json.Unmarshal(data, &state); err != nil {
		return err
	}
	if filepath.Clean(state.Prefix) != filepath.Clean(*prefix) {
		return errors.New("installation metadata prefix does not match")
	}
	lock, err := os.OpenFile(filepath.Join(*prefix, "etc", "container-update.lock"), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return errors.New("another container update is running")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	d, close, err := localDockerAPI()
	if err != nil {
		return err
	}
	defer close()
	var old inspectedContainer
	if err = d.request("GET", "/containers/"+url.PathEscape(*name)+"/json", nil, &old); err != nil {
		return err
	}
	var target struct {
		ID string `json:"Id"`
	}
	if err = d.request("GET", "/images/"+url.PathEscape(*image)+"/json", nil, &target); err != nil {
		return fmt.Errorf("replacement image must already be built or loaded: %w", err)
	}
	if target.ID == "" {
		return errors.New("replacement image has no ID")
	}
	payload, err := replacementConfig(old, target.ID)
	if err != nil {
		return err
	}
	var cmd []string
	if json.Unmarshal(old.Config["Cmd"], &cmd) != nil || len(cmd) == 0 || cmd[0] != "serve" {
		return errors.New("unrecognized dashboard command")
	}
	o := defaults()
	serveFlags := flag.NewFlagSet("existing-serve", flag.ContinueOnError)
	optionsFlags(serveFlags, &o)
	if err = serveFlags.Parse(cmd[1:]); err != nil {
		return err
	}
	if serveFlags.NArg() != 0 || o.HostBridge != "/host/bridge.sock" || o.Listen != state.Listen {
		return errors.New("existing container does not match installation metadata")
	}
	passwordRel, err := filepath.Rel("/state", o.PasswordFile)
	if !o.NoAuth && (err != nil || passwordRel == ".." || strings.HasPrefix(passwordRel, "../")) {
		return errors.New("password must be inside the existing dashboard state mount")
	}
	for _, mount := range old.Mounts {
		if mount.Destination == "/host" && filepath.Clean(mount.Source) != filepath.Clean(filepath.Dir(state.Socket)) {
			return errors.New("host-helper mount does not match metadata")
		}
		if mount.Destination == "/state" {
			o.PasswordFile = filepath.Join(mount.Source, passwordRel)
		}
	}
	if !o.NoAuth {
		if _, err = os.Stat(o.PasswordFile); err != nil {
			return fmt.Errorf("existing password file must be present: %w", err)
		}
	}
	if err = checkHostBridge(state.Socket); err != nil {
		return err
	}
	if err = waitReady(o); err != nil {
		return fmt.Errorf("existing dashboard is not ready; no changes made: %w", err)
	}
	if state.ApacheCommand != "" {
		proxy := o
		proxy.Listen = "127.0.0.1:80"
		if err = waitReady(proxy); err != nil {
			return fmt.Errorf("existing Apache route is not ready; no changes made: %w", err)
		}
	}
	fmt.Println("Updating dashboard container; preserving the current login, host helper and Apache configuration.")
	err = replaceContainer(d, *name, old, payload, func() error {
		if e := waitReady(o); e != nil {
			return e
		}
		if state.ApacheCommand != "" {
			proxy := o
			proxy.Listen = "127.0.0.1:80"
			if e := waitReady(proxy); e != nil {
				return e
			}
		}
		state.Image = *image
		updated, e := json.MarshalIndent(state, "", "  ")
		if e != nil {
			return e
		}
		// Rename atomically so a failed write never damages installation metadata.
		tmp := metadata + ".next"
		defer os.Remove(tmp)
		if e = os.WriteFile(tmp, updated, 0600); e != nil {
			return e
		}
		return os.Rename(tmp, metadata)
	})
	if err != nil {
		return err
	}
	fmt.Println("Updated to k9s-web-ui", packageVersion, "— http://<host-ip>/k9s/ (same username and password)")
	return nil
}
