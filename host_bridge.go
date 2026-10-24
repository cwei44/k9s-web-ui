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
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"
)

type bridgeRequest struct {
	Context string   `json:"context"`
	Args    []string `json:"args"`
}
type bridgeResponse struct {
	Output []byte `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

func bridgeClient(socket string) (*http.Client, *http.Transport) {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", socket)
	}}
	return &http.Client{Transport: transport, Timeout: 25 * time.Second}, transport
}
func checkHostBridge(socket string) error {
	client, transport := bridgeClient(socket)
	defer transport.CloseIdleConnections()
	response, err := client.Get("http://host/healthz")
	if err != nil {
		return fmt.Errorf("host kubectl helper unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("host helper health: HTTP %d", response.StatusCode)
	}
	return nil
}
func callHostBridge(o options, args ...string) ([]byte, error) {
	body, err := json.Marshal(bridgeRequest{Context: o.Context, Args: args})
	if err != nil {
		return nil, err
	}
	client, transport := bridgeClient(o.HostBridge)
	defer transport.CloseIdleConnections()
	response, err := client.Post("http://host/execute", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("host kubectl helper: %w", err)
	}
	defer response.Body.Close()
	var value bridgeResponse
	if err = json.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(&value); err != nil {
		return nil, err
	}
	if value.Error != "" {
		return nil, errors.New(value.Error)
	}
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("host helper: HTTP %d", response.StatusCode)
	}
	return value.Output, nil
}

var exporterPath = regexp.MustCompile(`^/api/v1/namespaces/([a-z0-9][a-z0-9-]*)/pods/([a-z0-9][a-z0-9.-]*):9400/proxy/metrics$`)
var namespacedMetrics = regexp.MustCompile(`^/apis/metrics\.k8s\.io/v1beta1/namespaces/([a-z0-9][a-z0-9-]*)/pods$`)

// This is deliberately a command allowlist, not a general kubectl proxy.
func allowedHostCommand(o options, args []string) bool {
	if len(args) == 0 || len(args) > 16 {
		return false
	}
	for _, arg := range args {
		if len(arg) > 1024 || strings.ContainsAny(arg, "\x00\r\n") {
			return false
		}
	}
	if slices.Equal(args, []string{"config", "current-context"}) || slices.Equal(args, []string{"version", "--client", "-o", "json"}) {
		return true
	}
	if len(args) == 3 && args[0] == "get" && args[1] == "--raw" {
		path := args[2]
		if path == "/version" || path == "/apis/metrics.k8s.io/v1beta1/nodes" {
			return true
		}
		if path == "/apis/metrics.k8s.io/v1beta1/pods" {
			return o.Namespace == ""
		}
		if m := namespacedMetrics.FindStringSubmatch(path); m != nil {
			return o.Namespace == "" || o.Namespace == m[1]
		}
		if m := exporterPath.FindStringSubmatch(path); m != nil {
			return o.GPUNamespace == "auto" || o.GPUNamespace == m[1]
		}
		return false
	}
	scope := ""
	if len(args) >= 2 && args[len(args)-2] == "-n" {
		scope = args[len(args)-1]
		if len(scope) > 63 || !namespaceRE.MatchString(scope) {
			return false
		}
		args = args[:len(args)-2]
	} else if args[len(args)-1] == "-A" {
		scope = "*"
		args = args[:len(args)-1]
	}
	if len(args) >= 4 && args[0] == "get" {
		resource := args[1]
		known := false
		for _, name := range resources {
			if resource == name {
				known = true
			}
		}
		if !known {
			return false
		}
		// Exporter discovery is separately scoped from resource views.
		if len(args) == 6 && resource == "pods" && args[2] == "-o" && args[3] == "json" && args[4] == "-l" {
			selectorOK := args[5] == o.GPUSelector || (o.GPUSelector == "app=nvidia-dcgm-exporter" && args[5] == "app.kubernetes.io/name=dcgm-exporter")
			return selectorOK && ((o.GPUNamespace == "auto" && scope == "*") || scope == o.GPUNamespace)
		}
		if resource == "nodes" {
			if scope != "" {
				return false
			}
		} else {
			if scope == "" || (o.Namespace != "" && scope != o.Namespace) {
				return false
			}
		}
		if len(args) == 4 {
			return args[2] == "-o" && args[3] == "json"
		}
		return len(args) == 5 && len(args[2]) <= 253 && nameRE.MatchString(args[2]) && args[3] == "-o" && args[4] == "yaml" && scope != "*"
	}
	if len(args) == 6 && args[0] == "logs" && scope != "" && scope != "*" && (o.Namespace == "" || o.Namespace == scope) {
		return len(args[1]) <= 253 && nameRE.MatchString(args[1]) && slices.Equal(args[2:], []string{"--tail=200", "--limit-bytes=1048576", "--timestamps=true", "--all-containers=true"})
	}
	return false
}

func hostBridgeHandler(o options, run func(options, ...string) ([]byte, error)) http.Handler {
	slots := make(chan struct{}, 8)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "method not allowed", 405)
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok", "kubectl": o.Kubectl})
	})
	mux.HandleFunc("/execute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			writeJSON(w, 405, bridgeResponse{Error: "POST required"})
			return
		}
		var req bridgeRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&req) != nil || len(req.Context) > 1024 || strings.ContainsAny(req.Context, "\x00\r\n") || !allowedHostCommand(o, req.Args) {
			writeJSON(w, 400, bridgeResponse{Error: "This host helper only permits the dashboard's read-only commands"})
			return
		}
		if o.Context != "" && req.Context != "" && req.Context != o.Context {
			writeJSON(w, 403, bridgeResponse{Error: "Context is restricted by the host helper"})
			return
		}
		selected := o
		if req.Context != "" {
			selected.Context = req.Context
		}
		select {
		case slots <- struct{}{}:
		case <-r.Context().Done():
			return
		}
		defer func() { <-slots }()
		output, err := run(selected, req.Args...)
		if err != nil {
			writeJSON(w, 502, bridgeResponse{Error: err.Error()})
			return
		}
		writeJSON(w, 200, bridgeResponse{Output: output})
	})
	return mux
}

func hostBridgeCommand(args []string) error {
	o := defaults()
	o.Kubectl = "kubectl"
	flags := flag.NewFlagSet("host-bridge", flag.ContinueOnError)
	optionsFlags(flags, &o)
	socket := flags.String("socket", "/run/k9s-web-ui-host/bridge.sock", "Private Unix socket")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || o.HostBridge != "" {
		return errors.New("invalid host helper arguments")
	}
	client, err := exec.LookPath(o.Kubectl)
	if err != nil {
		return fmt.Errorf("host kubectl: %w", err)
	}
	o.Kubectl, err = filepath.Abs(client)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(*socket) {
		return errors.New("socket must be absolute")
	}
	if err = os.MkdirAll(filepath.Dir(*socket), 0700); err != nil {
		return err
	}
	if st, e := os.Lstat(*socket); e == nil {
		if st.Mode()&os.ModeSocket == 0 {
			return errors.New("refusing to replace a non-socket path")
		}
		conn, e := net.DialTimeout("unix", *socket, time.Second)
		if e == nil {
			conn.Close()
			return errors.New("host helper is already running")
		}
		if err = os.Remove(*socket); err != nil {
			return err
		}
	}
	listener, err := net.Listen("unix", *socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(*socket)
	if err = os.Chmod(*socket, 0600); err != nil {
		return err
	}
	server := &http.Server{Handler: hostBridgeHandler(o, func(o options, args ...string) ([]byte, error) { return o.run(args...) }), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	log.Printf("Host helper ready: %s; using %s with the host Kubernetes login", *socket, o.Kubectl)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case <-signals:
	case err = <-done:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	server.Shutdown(ctx)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
