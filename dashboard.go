// Copyright 2026 K9s Web UI contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// All arguments come from fixed resource lists or validated names. No shell,
// arbitrary kubectl command, mutation endpoint, or secret resource is exposed.
type dashboard struct {
	options options
	run     func(options, ...string) ([]byte, error)
	mu      sync.Mutex
	cached  snapshot
	checked time.Time
}

type snapshot struct {
	Context    string                       `json:"context"`
	Connected  bool                         `json:"connected"`
	ObservedAt string                       `json:"observedAt"`
	Data       map[string][]json.RawMessage `json:"data"`
	Errors     map[string]string            `json:"errors"`
}

var resources = map[string]string{
	"Pods": "pods", "Deployments": "deployments.apps", "Services": "services",
	"Nodes": "nodes", "Events": "events", "Storage": "persistentvolumeclaims",
}
var nameRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)
var namespaceRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (d *dashboard) current() (options, error) {
	o := d.options
	if o.Context != "" {
		return o, nil
	}
	b, e := d.run(o, "config", "current-context")
	if e != nil {
		return o, e
	}
	o.Context = strings.TrimSpace(string(b))
	if o.Context == "" {
		return o, fmt.Errorf("the host has no current Kubernetes context")
	}
	return o, nil
}

func (d *dashboard) getSnapshot() snapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.checked.IsZero() && time.Since(d.checked) < 5*time.Second {
		return d.cached
	}
	s := snapshot{ObservedAt: time.Now().UTC().Format(time.RFC3339), Data: map[string][]json.RawMessage{}, Errors: map[string]string{}}
	for key := range resources {
		s.Data[key] = []json.RawMessage{}
	}
	s.Data["podMetrics"] = []json.RawMessage{}
	s.Data["nodeMetrics"] = []json.RawMessage{}
	o, e := d.current()
	s.Context = o.Context
	if e != nil {
		s.Errors["connection"] = e.Error()
	} else if _, e = d.run(o, "get", "--raw", "/version"); e != nil {
		s.Errors["connection"] = e.Error()
	} else {
		s.Connected = true
		var mu sync.Mutex
		var wg sync.WaitGroup
		fetch := func(key string, args ...string) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				b, err := d.run(o, args...)
				var list struct {
					Items []json.RawMessage `json:"items"`
				}
				if err == nil {
					err = json.Unmarshal(b, &list)
				}
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					s.Errors[key] = err.Error()
				} else if list.Items != nil {
					s.Data[key] = list.Items
				}
			}()
		}
		for key, resource := range resources {
			args := []string{"get", resource, "-o", "json"}
			if key != "Nodes" {
				if o.Namespace == "" {
					args = append(args, "-A")
				} else {
					args = append(args, "-n", o.Namespace)
				}
			}
			fetch(key, args...)
		}
		podPath := "/apis/metrics.k8s.io/v1beta1/pods"
		if o.Namespace != "" {
			podPath = "/apis/metrics.k8s.io/v1beta1/namespaces/" + url.PathEscape(o.Namespace) + "/pods"
		}
		fetch("podMetrics", "get", "--raw", podPath)
		fetch("nodeMetrics", "get", "--raw", "/apis/metrics.k8s.io/v1beta1/nodes")
		wg.Wait()
	}
	d.cached = s
	d.checked = time.Now()
	return s
}

func (d *dashboard) detail(w http.ResponseWriter, r *http.Request, logs bool) {
	q := r.URL.Query()
	kind, name, namespace := q.Get("kind"), q.Get("name"), q.Get("namespace")
	resource, ok := resources[kind]
	if !ok || len(name) > 253 || !nameRE.MatchString(name) || (logs && kind != "Pods") {
		writeJSON(w, 400, map[string]string{"error": "Unsupported resource kind or invalid name"})
		return
	}
	if kind != "Nodes" {
		if len(namespace) > 63 || !namespaceRE.MatchString(namespace) {
			writeJSON(w, 400, map[string]string{"error": "A valid namespace is required"})
			return
		}
		if d.options.Namespace != "" && namespace != d.options.Namespace {
			writeJSON(w, 403, map[string]string{"error": "Resource is outside the configured namespace"})
			return
		}
	}
	args := []string{"get", resource, name, "-o", "yaml"}
	if logs {
		args = []string{"logs", name, "--tail=200", "--limit-bytes=1048576", "--timestamps=true", "--all-containers=true"}
	}
	if kind != "Nodes" {
		args = append(args, "-n", namespace)
	}
	o, e := d.current()
	var b []byte
	if e == nil {
		b, e = d.run(o, args...)
	}
	if e != nil {
		writeJSON(w, 502, map[string]string{"error": e.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"text": string(b)})
}

func dashboardHandler(o options) http.Handler {
	d := &dashboard{options: o, run: func(o options, args ...string) ([]byte, error) { return o.run(args...) }}
	gpus := newGPUCache(o)
	gpus.collect = func(options) gpuSnapshot {
		selected, err := d.current()
		if err != nil {
			return gpuSnapshot{Devices: []device{}, Errors: map[string]string{"context": err.Error()}}
		}
		value := collectGPU(selected)
		value.Context = selected.Context
		return value
	}
	return d.handler(gpus)
}

func (d *dashboard) handler(gpus *gpuCache) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("/k9s/api/snapshot", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, d.getSnapshot()) })
	mux.HandleFunc("/k9s/api/logs", func(w http.ResponseWriter, r *http.Request) { d.detail(w, r, true) })
	mux.HandleFunc("/k9s/api/manifest", func(w http.ResponseWriter, r *http.Request) { d.detail(w, r, false) })
	mux.HandleFunc("/k9s/api/gpu", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, gpus.get()) })
	mux.HandleFunc("/k9s/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 404, map[string]string{"error": "Unknown API endpoint"})
	})
	web, _ := fs.Sub(assets, "web")
	mux.Handle("/k9s/", http.StripPrefix("/k9s/", http.FileServer(http.FS(web))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/k9s" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/k9s/", http.StatusTemporaryRedirect)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Allow", "GET, HEAD")
			writeJSON(w, 405, map[string]string{"error": "This dashboard is read-only"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}
