// Copyright 2026 K9s Web UI contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type workload struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
}
type device struct {
	Node            string     `json:"node"`
	GPU             string     `json:"gpu"`
	UUID            string     `json:"uuid"`
	Instance        string     `json:"instance"`
	ComputeInstance string     `json:"computeInstance"`
	Model           string     `json:"model"`
	Utilization     *float64   `json:"utilization"`
	MemoryUsed      *float64   `json:"memoryUsedMiB"`
	MemoryFree      *float64   `json:"memoryFreeMiB"`
	MemoryReserved  *float64   `json:"memoryReservedMiB"`
	MemoryUsable    *float64   `json:"memoryUsableMiB"`
	MemoryPercent   *float64   `json:"memoryPercent"`
	Workloads       []workload `json:"workloads"`
}
type gpuSnapshot struct {
	Context    string            `json:"context,omitempty"`
	ObservedAt string            `json:"observedAt"`
	Source     string            `json:"source"`
	Devices    []device          `json:"devices"`
	Expected   int               `json:"exportersExpected"`
	Healthy    int               `json:"exportersHealthy"`
	Errors     map[string]string `json:"errors"`
	Stale      bool              `json:"stale"`
}

var metricRE = regexp.MustCompile(`^(DCGM_[A-Z0-9_]+)\{(.*)\}\s+(\S+)`)
var labelRE = regexp.MustCompile(`(\w+)="((?:\\.|[^"\\])*)"`)

func parseMetrics(text, node string) []device {
	m := map[string]*device{}
	for _, line := range strings.Split(text, "\n") {
		match := metricRE.FindStringSubmatch(line)
		if len(match) == 0 {
			continue
		}
		if match[1] != "DCGM_FI_DEV_GPU_UTIL" && match[1] != "DCGM_FI_DEV_FB_USED" && match[1] != "DCGM_FI_DEV_FB_FREE" && match[1] != "DCGM_FI_DEV_FB_RESERVED" {
			continue
		}
		labels := map[string]string{}
		valid := true
		for _, l := range labelRE.FindAllStringSubmatch(match[2], -1) {
			v, e := strconv.Unquote(`"` + l[2] + `"`)
			if e != nil {
				valid = false
				break
			}
			labels[l[1]] = v
		}
		if !valid {
			continue
		}
		value, e := strconv.ParseFloat(match[3], 64)
		if e != nil {
			continue
		}
		if labels["gpu"] == "" && labels["UUID"] == "" {
			continue
		}
		key := strings.Join([]string{node, labels["UUID"], labels["gpu"], labels["GPU_I_ID"], labels["GPU_CI_ID"]}, "\x00")
		d := m[key]
		if d == nil {
			d = &device{Node: node, GPU: labels["gpu"], UUID: labels["UUID"], Instance: labels["GPU_I_ID"], ComputeInstance: labels["GPU_CI_ID"], Model: labels["modelName"], Workloads: []workload{}}
			m[key] = d
		}
		if labels["pod"] != "" {
			w := workload{labels["namespace"], labels["pod"], labels["container"]}
			found := false
			for _, v := range d.Workloads {
				if v == w {
					found = true
				}
			}
			if !found {
				d.Workloads = append(d.Workloads, w)
			}
		}
		ceiling := 1e12
		var field **float64
		switch match[1] {
		case "DCGM_FI_DEV_GPU_UTIL":
			field = &d.Utilization
			ceiling = 100
		case "DCGM_FI_DEV_FB_USED":
			field = &d.MemoryUsed
		case "DCGM_FI_DEV_FB_FREE":
			field = &d.MemoryFree
		case "DCGM_FI_DEV_FB_RESERVED":
			field = &d.MemoryReserved
		}
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > ceiling {
			continue
		}
		if *field == nil || value > **field {
			v := value
			*field = &v
		}
	}
	rows := []device{}
	for _, d := range m {
		if d.MemoryUsed != nil && d.MemoryFree != nil {
			total := *d.MemoryUsed + *d.MemoryFree
			d.MemoryUsable = &total
			if total > 0 {
				pct := *d.MemoryUsed / total * 100
				d.MemoryPercent = &pct
			}
		}
		rows = append(rows, *d)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].GPU < rows[j].GPU })
	return rows
}

type podRecord struct {
	Metadata struct{ Name, Namespace string }
	Spec     struct{ NodeName string }
	Status   struct{ Phase string }
}

func discover(o options) ([]podRecord, error) {
	run := func(selector string) ([]podRecord, error) {
		args := []string{"get", "pods", "-o", "json", "-l", selector}
		if o.GPUNamespace == "auto" {
			args = append(args, "-A")
		} else {
			args = append(args, "-n", o.GPUNamespace)
		}
		b, e := o.run(args...)
		if e != nil {
			return nil, e
		}
		var l struct{ Items []podRecord }
		e = json.Unmarshal(b, &l)
		return l.Items, e
	}
	pods, e := run(o.GPUSelector)
	if e == nil && len(pods) == 0 && o.GPUSelector == "app=nvidia-dcgm-exporter" {
		return run("app.kubernetes.io/name=dcgm-exporter")
	}
	return pods, e
}
func collectGPU(o options) gpuSnapshot {
	result := gpuSnapshot{ObservedAt: time.Now().UTC().Format(time.RFC3339), Source: "NVIDIA DCGM", Devices: []device{}, Errors: map[string]string{}}
	pods, e := discover(o)
	if e != nil {
		result.Errors["discovery"] = e.Error()
		return result
	}
	result.Expected = len(pods)
	if len(pods) == 0 {
		result.Errors["discovery"] = "No DCGM exporter found. Install NVIDIA DCGM Exporter on the GPU nodes, or set --gpu-namespace / --gpu-selector. GPU allocation alone is not utilization."
		return result
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, p := range pods {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var rows []device
			var err error
			if p.Status.Phase != "Running" {
				err = fmt.Errorf("exporter is not running")
			} else {
				path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s:9400/proxy/metrics", p.Metadata.Namespace, p.Metadata.Name)
				var b []byte
				b, err = o.run("get", "--raw", path)
				if err == nil {
					rows = parseMetrics(string(b), p.Spec.NodeName)
					if len(rows) == 0 {
						err = fmt.Errorf("no supported GPU samples returned")
					}
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				result.Errors[p.Metadata.Namespace+"/"+p.Metadata.Name] = err.Error()
			} else {
				result.Healthy++
				result.Devices = append(result.Devices, rows...)
			}
		}()
	}
	wg.Wait()
	unique := map[string]device{}
	for _, d := range result.Devices {
		key := strings.Join([]string{d.Node, d.UUID, d.GPU, d.Instance, d.ComputeInstance}, "\x00")
		unique[key] = d
	}
	result.Devices = []device{}
	for _, d := range unique {
		result.Devices = append(result.Devices, d)
	}
	sort.Slice(result.Devices, func(i, j int) bool {
		a, b := result.Devices[i], result.Devices[j]
		return a.Node+"/"+a.GPU+"/"+a.Instance < b.Node+"/"+b.GPU+"/"+b.Instance
	})
	return result
}

type gpuCache struct {
	mu      sync.Mutex
	o       options
	checked time.Time
	value   gpuSnapshot
	collect func(options) gpuSnapshot
}

func newGPUCache(o options) *gpuCache { return &gpuCache{o: o, collect: collectGPU} }
func (c *gpuCache) get() gpuSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.checked.IsZero() && time.Since(c.checked) < 15*time.Second {
		return c.value
	}
	v := c.collect(c.o)
	if len(v.Devices) == 0 && len(c.value.Devices) > 0 && v.Context == c.value.Context {
		old := c.value
		old.Errors = v.Errors
		old.Healthy = 0
		old.Stale = true
		v = old
	}
	c.value = v
	c.checked = time.Now()
	return v
}
