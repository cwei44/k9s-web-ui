// Copyright 2026 K9s Web UI contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGPUParsingDuplicatesAndMIG(t *testing.T) {
	metrics := `DCGM_FI_DEV_GPU_UTIL{gpu="0",UUID="GPU-one",modelName="Test",pod="a",namespace="ns"} 42
DCGM_FI_DEV_GPU_UTIL{gpu="0",UUID="GPU-one",pod="b",namespace="ns"} 42
DCGM_FI_DEV_FB_USED{gpu="0",UUID="GPU-one"} 1024
DCGM_FI_DEV_FB_FREE{gpu="0",UUID="GPU-one"} 3072
DCGM_FI_DEV_GPU_UTIL{gpu="0",UUID="GPU-one",GPU_I_ID="1"} 25
DCGM_FI_DEV_GPU_UTIL{gpu="1",UUID="GPU-two"} 9223372036854775794
DCGM_FI_DEV_FB_USED{gpu="1",UUID="GPU-two"} NaN
`
	rows := parseMetrics(metrics, "node-a")
	if len(rows) != 3 {
		t.Fatalf("want three physical/MIG identities, got %d", len(rows))
	}
	for _, d := range rows {
		if d.UUID == "GPU-two" && (d.Utilization != nil || d.MemoryUsed != nil) {
			t.Fatal("sentinels must be null")
		}
		if d.UUID == "GPU-one" && d.Instance == "" {
			if d.Utilization == nil || *d.Utilization != 42 || len(d.Workloads) != 2 {
				t.Fatalf("duplicate metrics: %+v", d)
			}
			if d.MemoryPercent == nil || *d.MemoryPercent != 25 {
				t.Fatalf("memory ratio: %+v", d)
			}
		}
	}
}
func TestGPUCacheMarksFailureStale(t *testing.T) {
	calls := 0
	c := newGPUCache(options{})
	c.collect = func(options) gpuSnapshot {
		calls++
		if calls == 1 {
			return gpuSnapshot{ObservedAt: "original", Devices: []device{{GPU: "0"}}, Healthy: 1}
		}
		return gpuSnapshot{Errors: map[string]string{"discovery": "denied"}}
	}
	c.get()
	c.get()
	if calls != 1 {
		t.Fatal("cache did not suppress repeated discovery")
	}
	c.checked = time.Now().Add(-time.Minute)
	s := c.get()
	if !s.Stale || s.ObservedAt != "original" || len(s.Devices) != 1 || s.Healthy != 0 || s.Errors["discovery"] != "denied" {
		t.Fatalf("bad stale snapshot: %+v", s)
	}
}
func TestAuthenticationAndPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "etc", "password")
	p, e := password(path)
	if e != nil || len(p) < 32 {
		t.Fatalf("password: %q %v", p, e)
	}
	again, _ := password(path)
	if p != again {
		t.Fatal("password changed on restart")
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0600 {
		t.Fatal("password permissions")
	}
	h := auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), "admin", p)
	for _, good := range []bool{false, true} {
		r := httptest.NewRequest("GET", "/k9s/api/gpu", nil)
		if good {
			r.SetBasicAuth("admin", p)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		want := 401
		if good {
			want = 204
		}
		if w.Code != want {
			t.Fatalf("want %d got %d", want, w.Code)
		}
	}
}
func TestVendorVersionAndUnitQuoting(t *testing.T) {
	n, e := minor("v1.35.5+vmware.1")
	if e != nil || n != 35 {
		t.Fatal(n, e)
	}
	if _, e = minor("invalid"); e == nil {
		t.Fatal("accepted invalid version")
	}
	unit := serviceText("/opt/k9s test", options{Kubeconfig: "/a/kube config", Context: "a%b"})
	if !strings.Contains(unit, `"/opt/k9s test/k9s-web-ui"`) || !strings.Contains(unit, `"a%%b"`) {
		t.Fatal(unit)
	}
}
func TestManifestRejectsTraversalAndTampering(t *testing.T) {
	old := bundle
	bundle = t.TempDir()
	defer func() { bundle = old }()
	os.WriteFile(filepath.Join(bundle, "SHA256SUMS"), []byte(strings.Repeat("0", 64)+"  ../outside\n"), 0600)
	if _, e := verifyBundle(); e == nil {
		t.Fatal("accepted traversal")
	}
	os.WriteFile(filepath.Join(bundle, "file"), []byte("tampered"), 0600)
	os.WriteFile(filepath.Join(bundle, "SHA256SUMS"), []byte(strings.Repeat("0", 64)+"  file\n"), 0600)
	if _, e := verifyBundle(); e == nil {
		t.Fatal("accepted wrong checksum")
	}
}

func TestHostKubernetesDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KUBECONFIG", "")
	o := defaults()
	if o.Kubeconfig != filepath.Join(home, ".kube", "config") || o.Context != "" {
		t.Fatalf("must use host default and current context: %+v", o)
	}
	paths := filepath.Join(home, "cluster-a") + ":" + filepath.Join(home, "cluster-b")
	t.Setenv("KUBECONFIG", paths)
	o = defaults()
	if o.Kubeconfig != paths {
		t.Fatal("KUBECONFIG path list was not retained")
	}
	if strings.Contains(strings.Join(o.args("get", "pods"), " "), "--context") {
		t.Fatal("default must not pin a context")
	}
	service := serviceText("/opt/k9s-web-ui", o)
	if !strings.Contains(service, quote(paths)) || strings.Contains(service, `"--context"`) {
		t.Fatal("service must retain host paths and follow current-context")
	}
}
