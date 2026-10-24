package main

import (
	"net"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHostCommandAllowlist(t *testing.T) {
	o := defaults()
	allowed := [][]string{
		{"config", "current-context"}, {"version", "--client", "-o", "json"},
		{"get", "--raw", "/version"}, {"get", "--raw", "/apis/metrics.k8s.io/v1beta1/nodes"},
		{"get", "--raw", "/apis/metrics.k8s.io/v1beta1/pods"},
		{"get", "--raw", "/apis/metrics.k8s.io/v1beta1/namespaces/team/pods"},
		{"get", "--raw", "/api/v1/namespaces/gpu/pods/exporter-1:9400/proxy/metrics"},
		{"get", "pods", "-o", "json", "-l", "app=nvidia-dcgm-exporter", "-A"},
		{"get", "pods", "-o", "json", "-l", "app.kubernetes.io/name=dcgm-exporter", "-A"},
		{"get", "pods", "pod-1", "-o", "yaml", "-n", "team"},
		{"get", "nodes", "node-1", "-o", "yaml"},
		{"logs", "pod-1", "--tail=200", "--limit-bytes=1048576", "--timestamps=true", "--all-containers=true", "-n", "team"},
	}
	for _, resource := range resources {
		args := []string{"get", resource, "-o", "json"}
		if resource != "nodes" {
			args = append(args, "-A")
		}
		allowed = append(allowed, args)
	}
	for _, args := range allowed {
		if !allowedHostCommand(o, args) {
			t.Errorf("rejected dashboard command: %v", args)
		}
	}
	denied := [][]string{
		{}, {"delete", "pods", "pod-1"}, {"exec", "pod-1", "--", "sh"},
		{"get", "secrets", "-o", "json", "-A"}, {"config", "view", "--raw"},
		{"get", "--raw", "/api/v1/secrets"},
		{"get", "--raw", "/api/v1/namespaces/gpu/pods/exporter-1:9400/proxy/../../secrets"},
		{"get", "pods", "-o", "json", "--kubeconfig", "/etc/private"},
		{"get", "pods", "-o", "json", "-n", "--help"},
		{"get", "pods", "--token=evil", "-o", "yaml", "-n", "team"},
		{"get", "pods", "-o", "json", "-l", "app=unrelated", "-A"},
		{"logs", "pod-1", "--follow", "-n", "team"},
	}
	for _, args := range denied {
		if allowedHostCommand(o, args) {
			t.Errorf("accepted unauthorized command: %v", args)
		}
	}
	o.Namespace, o.GPUNamespace = "team", "gpu"
	for _, args := range [][]string{
		{"get", "pods", "-o", "json", "-A"},
		{"get", "pods", "-o", "json", "-n", "other"},
		{"get", "--raw", "/apis/metrics.k8s.io/v1beta1/pods"},
		{"get", "--raw", "/apis/metrics.k8s.io/v1beta1/namespaces/other/pods"},
		{"get", "--raw", "/api/v1/namespaces/other/pods/exporter:9400/proxy/metrics"},
	} {
		if allowedHostCommand(o, args) {
			t.Errorf("namespace escaped: %v", args)
		}
	}
}

func TestHostBridgeUsesHostOptionsAndCurrentContext(t *testing.T) {
	host := defaults()
	host.Kubectl, host.Kubeconfig = "/host/bin/vendor-kubectl", "/host/login/config"
	socket := filepath.Join(t.TempDir(), "bridge.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	var current atomic.Value
	current.Store("first")
	handler := hostBridgeHandler(host, func(o options, args ...string) ([]byte, error) {
		if o.Kubectl != host.Kubectl || o.Kubeconfig != host.Kubeconfig {
			t.Error("lost host execution options")
		}
		if slices.Equal(args, []string{"config", "current-context"}) {
			return []byte(current.Load().(string)), nil
		}
		return []byte(o.Context), nil
	})
	server := &http.Server{Handler: handler}
	go server.Serve(listener)
	defer server.Close()
	client := options{HostBridge: socket, Kubectl: "/missing-client", Kubeconfig: "/missing-config", Offline: true}
	if err := prepare(&client); err != nil {
		t.Fatal(err)
	}
	for _, context := range []string{"first", "second"} {
		current.Store(context)
		got, err := client.run("config", "current-context")
		if err != nil || string(got) != context {
			t.Fatalf("context update: %q %v", got, err)
		}
	}
	client.Context = "snapshot-context"
	got, err := client.run("get", "--raw", "/version")
	if err != nil || string(got) != client.Context {
		t.Fatalf("context pin: %q %v", got, err)
	}
	_, err = client.run("delete", "pods", "anything")
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("mutation accepted: %v", err)
	}
}
