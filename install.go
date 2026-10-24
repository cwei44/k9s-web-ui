// Copyright 2026 K9s Web UI contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type installation struct {
	Options       options `json:"options"`
	Systemd       bool    `json:"systemd"`
	ApacheConfig  string  `json:"apacheConfig,omitempty"`
	ApacheBackup  string  `json:"apacheBackup,omitempty"`
	ApacheSnippet string  `json:"apacheSnippet,omitempty"`
	ApacheCommand string  `json:"apacheCommand,omitempty"`
}

func verifyBundle() ([]string, error) {
	f, e := os.Open(filepath.Join(bundle, "SHA256SUMS"))
	if e != nil {
		return nil, e
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	files := []string{}
	for s.Scan() {
		line := s.Text()
		if len(line) < 67 {
			return nil, errors.New("invalid SHA256SUMS")
		}
		name := line[66:]
		if filepath.IsAbs(name) || filepath.Clean(name) != name || strings.HasPrefix(name, "../") {
			return nil, errors.New("unsafe manifest path")
		}
		p := filepath.Join(bundle, name)
		f, e := os.Open(p)
		if e != nil {
			return nil, e
		}
		h := sha256.New()
		_, e = io.Copy(h, f)
		f.Close()
		if e != nil {
			return nil, e
		}
		if hex.EncodeToString(h.Sum(nil)) != line[:64] {
			return nil, fmt.Errorf("checksum mismatch: %s", name)
		}
		files = append(files, name)
	}
	return files, s.Err()
}
func copyFile(src, dst string, mode os.FileMode) error {
	in, e := os.Open(src)
	if e != nil {
		return e
	}
	defer in.Close()
	if e = os.MkdirAll(filepath.Dir(dst), 0755); e != nil {
		return e
	}
	out, e := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, in)
	ce := out.Close()
	if e != nil {
		return e
	}
	return ce
}
func quote(s string) string {
	s = strings.ReplaceAll(s, "%", "%%")
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
func serveArgs(o options) []string {
	a := []string{"serve", "--listen", o.Listen, "--kubeconfig", o.Kubeconfig, "--kubectl", o.Kubectl, "--password-file", o.PasswordFile, "--username", o.Username, "--gpu-namespace", o.GPUNamespace, "--gpu-selector", o.GPUSelector}
	if o.Context != "" {
		a = append(a, "--context", o.Context)
	}
	if o.Namespace != "" {
		a = append(a, "--namespace", o.Namespace)
	}
	if o.Offline {
		a = append(a, "--offline")
	}
	if o.NoAuth {
		a = append(a, "--no-auth")
	}
	return a
}
func serviceText(prefix string, o options) string {
	cmd := []string{quote(filepath.Join(prefix, "k9s-web-ui"))}
	for _, a := range serveArgs(o) {
		cmd = append(cmd, quote(a))
	}
	home, _ := os.UserHomeDir()
	return "[Unit]\nDescription=Portable K9s UI and GPU monitoring\nWants=network-online.target\nAfter=network-online.target\n\n[Service]\nType=simple\nUser=root\nWorkingDirectory=" + quote(prefix) + "\nEnvironment=" + quote("HOME="+home) + "\nEnvironment=" + quote("PATH="+os.Getenv("PATH")) + "\nExecStart=" + strings.Join(cmd, " ") + "\nRestart=on-failure\nRestartSec=5\nNoNewPrivileges=true\n\n[Install]\nWantedBy=multi-user.target\n"
}
func portFree(address string) bool {
	l, e := net.Listen("tcp", address)
	if e != nil {
		return false
	}
	l.Close()
	return true
}
func command(cmd string, args ...string) (string, error) {
	out, e := exec.Command(cmd, args...).CombinedOutput()
	if e != nil {
		return string(out), fmt.Errorf("%s %v: %s", cmd, args, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
func apacheExecutable() string {
	for _, c := range []string{"apache2ctl", "apachectl", "httpd", "apache2"} {
		if p, e := exec.LookPath(c); e == nil {
			return p
		}
	}
	return ""
}

var rootRE = regexp.MustCompile(`HTTPD_ROOT="([^"]+)"`)
var confRE = regexp.MustCompile(`SERVER_CONFIG_FILE="([^"]+)"`)
var apacheVersionRE = regexp.MustCompile(`Apache/(\d+)\.(\d+)\.(\d+)`)

func apacheSetup(ctl, override, prefix string, port int) (config, body, snippet string, err error) {
	v, e := command(ctl, "-V")
	if e != nil {
		err = e
		return
	}
	vm := apacheVersionRE.FindStringSubmatch(v)
	if len(vm) == 0 {
		err = errors.New("cannot determine Apache version")
		return
	}
	patch, _ := strconv.Atoi(vm[3])
	if vm[1] != "2" || vm[2] != "4" || patch < 47 {
		err = errors.New("Apache 2.4.47+ required; use standalone mode on older servers")
		return
	}
	rm, cm := rootRE.FindStringSubmatch(v), confRE.FindStringSubmatch(v)
	if len(rm) == 0 || len(cm) == 0 {
		err = errors.New("cannot determine Apache configuration path")
		return
	}
	config = cm[1]
	if !filepath.IsAbs(config) {
		config = filepath.Join(rm[1], config)
	}
	if override != "" {
		config = override
	}
	// Config inspection and later reload use the selected configuration.
	inc, e := command(ctl, "-t", "-f", config, "-D", "DUMP_INCLUDES")
	if e != nil {
		err = e
		return
	}
	incRE := regexp.MustCompile(`(?m)^\s*\([^)]*\)\s+(/.*)$`)
	conflicts := regexp.MustCompile(`(?mi)^\s*(?:<VirtualHost|ProxyPass(?:Match)?\s+"?(?:/"?\s|/k9s))`)
	for _, m := range incRE.FindAllStringSubmatch(inc, -1) {
		b, e := os.ReadFile(strings.TrimSpace(m[1]))
		if e != nil {
			err = e
			return
		}
		if conflicts.Match(b) {
			err = fmt.Errorf("custom virtual host or conflicting proxy in %s; use --mode standalone and integrate the supplied Apache example manually", m[1])
			return
		}
	}
	modules, e := command(ctl, "-M", "-f", config)
	if e != nil {
		err = e
		return
	}
	loads := ""
	for _, name := range []string{"proxy", "proxy_http", "alias", "authz_core"} {
		if strings.Contains(modules, name+"_module") {
			continue
		}
		found := ""
		for _, dir := range []string{filepath.Join(rm[1], "modules"), "/usr/lib/httpd/modules", "/usr/lib64/httpd/modules", "/usr/lib/apache2/modules"} {
			p := filepath.Join(dir, "mod_"+name+".so")
			if _, e := os.Stat(p); e == nil {
				found = p
				break
			}
		}
		if found == "" {
			err = fmt.Errorf("Apache module %s is absent; standalone mode needs no Apache packages", name)
			return
		}
		loads += "<IfModule !" + name + "_module>\n  LoadModule " + name + "_module " + quote(found) + "\n</IfModule>\n"
	}
	snippet = filepath.Join(rm[1], "k9s-web-ui.conf")
	if _, e = os.Stat(snippet); e == nil {
		err = fmt.Errorf("refusing to overwrite existing %s", snippet)
		return
	}
	base := fmt.Sprintf("http://127.0.0.1:%d/k9s/", port)
	body = loads + "# Portable K9s: authentication is enforced by the backend.\nProxyPass \"/k9s/\" \"" + base + "\" upgrade=websocket timeout=90\nProxyPassReverse \"/k9s/\" \"" + base + "\"\nRedirectMatch 302 \"^/k9s$\" \"/k9s/\"\n<Location \"/k9s/\">\n  ProxyPreserveHost On\n  Require all granted\n</Location>\n"
	return
}
func install(args []string) (err error) {
	if os.Geteuid() != 0 {
		return errors.New("install requires root for service setup; use 'serve' without root")
	}
	o := defaults()
	o.Listen = ""
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	optionsFlags(fs, &o)
	prefix := fs.String("prefix", "/opt/k9s-web-ui", "Installation directory (must not already exist)")
	mode := fs.String("mode", "auto", "auto, apache, or standalone")
	apacheConfig := fs.String("apache-config", "", "Main Apache configuration override")
	backendPort := fs.Int("backend-port", 19090, "Local frontend port when Apache owns port 80")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *mode != "auto" && *mode != "apache" && *mode != "standalone" {
		return errors.New("mode must be auto, apache, or standalone")
	}
	if !filepath.IsAbs(*prefix) || strings.ContainsAny(*prefix, "\n\r\"%") {
		return errors.New("prefix must be an absolute path without quotes, newlines or percent signs")
	}
	if _, e := os.Stat(*prefix); !os.IsNotExist(e) {
		return errors.New("installation directory already exists; use another --prefix to avoid overwriting data")
	}
	if *backendPort < 1 || *backendPort > 65535 {
		return errors.New("invalid backend port")
	}
	files, e := verifyBundle()
	if e != nil {
		return fmt.Errorf("package integrity check: %w", e)
	}
	if e = prepare(&o); e != nil {
		return e
	}
	ctl := apacheExecutable()
	useApache := *mode == "apache" || (*mode == "auto" && !portFree(":80") && ctl != "")
	inst := installation{}
	var apacheBody string
	if useApache {
		if ctl == "" {
			return errors.New("Apache not found; use --mode standalone")
		}
		inst.ApacheConfig, apacheBody, inst.ApacheSnippet, e = apacheSetup(ctl, *apacheConfig, *prefix, *backendPort)
		if e != nil {
			if *mode == "apache" {
				return e
			}
			fmt.Println("Apache auto-integration skipped:", e)
			useApache = false
		} else {
			inst.ApacheCommand = ctl
			o.Listen = fmt.Sprintf("127.0.0.1:%d", *backendPort)
		}
	}
	if !useApache && o.Listen == "" {
		o.Listen = "0.0.0.0:80"
		if !portFree(":80") {
			o.Listen = "0.0.0.0:8080"
		}
	}
	if !portFree(o.Listen) {
		return fmt.Errorf("listen address %s is busy; choose --listen or --backend-port", o.Listen)
	}
	_, sdErr := os.Stat("/run/systemd/system")
	_, cmdErr := exec.LookPath("systemctl")
	inst.Systemd = sdErr == nil && cmdErr == nil
	unit := "/etc/systemd/system/k9s-web-ui.service"
	if inst.Systemd {
		if _, e := os.Stat(unit); !os.IsNotExist(e) {
			return errors.New("k9s-web-ui service already exists")
		}
	}
	if e = os.MkdirAll(*prefix, 0755); e != nil {
		return e
	}
	// Keep failed installation files for diagnosis, but roll back Apache and service activation.
	started, modified := false, false
	defer func() {
		if err != nil {
			if modified {
				copyFile(inst.ApacheBackup, inst.ApacheConfig, 0644)
				command(ctl, "-t", "-f", inst.ApacheConfig)
				command(ctl, "-k", "graceful", "-f", inst.ApacheConfig)
				os.Remove(inst.ApacheSnippet)
			}
			if started {
				stopInstallation(*prefix, inst)
			}
			if inst.Systemd {
				command("systemctl", "disable", "k9s-web-ui.service")
				os.Remove(unit)
				command("systemctl", "daemon-reload")
			}
			fmt.Fprintln(os.Stderr, "Installation did not complete; retained files at", *prefix)
		}
	}()
	for _, name := range append(files, "SHA256SUMS") {
		info, e := os.Stat(filepath.Join(bundle, name))
		if e != nil {
			return e
		}
		if e = copyFile(filepath.Join(bundle, name), filepath.Join(*prefix, name), info.Mode().Perm()); e != nil {
			return e
		}
	}
	if strings.HasPrefix(o.Kubectl, bundle+string(os.PathSeparator)) {
		o.Kubectl = filepath.Join(*prefix, strings.TrimPrefix(o.Kubectl, bundle+string(os.PathSeparator)))
	}
	// Resolve config paths now, without copying credentials out of their current location.
	kcs := filepath.SplitList(o.Kubeconfig)
	for i, p := range kcs {
		kcs[i], e = filepath.Abs(p)
		if e != nil {
			return e
		}
	}
	o.Kubeconfig = strings.Join(kcs, string(os.PathListSeparator))
	if !o.NoAuth {
		o.PasswordFile, e = filepath.Abs(o.PasswordFile)
		if e != nil {
			return e
		}
		if o.PasswordFile == defaults().PasswordFile {
			o.PasswordFile = filepath.Join(*prefix, "etc", "dashboard-password")
		}
		if _, e = password(o.PasswordFile); e != nil {
			return e
		}
	}
	inst.Options = o
	if e = os.MkdirAll(filepath.Join(*prefix, "etc"), 0700); e != nil {
		return e
	}
	if inst.Systemd {
		if e = os.WriteFile(unit, []byte(serviceText(*prefix, o)), 0644); e != nil {
			return e
		}
		if _, e = command("systemctl", "daemon-reload"); e != nil {
			return e
		}
		started = true
		if _, e = command("systemctl", "enable", "--now", "k9s-web-ui.service"); e != nil {
			return e
		}
	} else {
		started = true
		if e = daemon(*prefix, o); e != nil {
			return e
		}
	}
	if e = waitReady(o); e != nil {
		return e
	}
	if useApache {
		inst.ApacheBackup = filepath.Join(*prefix, "etc", "apache-before.conf")
		if e = copyFile(inst.ApacheConfig, inst.ApacheBackup, 0600); e != nil {
			return e
		}
		if e = os.WriteFile(inst.ApacheSnippet, []byte(apacheBody), 0644); e != nil {
			return e
		}
		modified = true
		f, e := os.OpenFile(inst.ApacheConfig, os.O_APPEND|os.O_WRONLY, 0)
		if e != nil {
			return e
		}
		_, e = f.WriteString("\nInclude " + quote(inst.ApacheSnippet) + "\n")
		f.Close()
		if e != nil {
			return e
		}
		if _, e = command(ctl, "-t", "-f", inst.ApacheConfig); e != nil {
			return e
		}
		if _, e = command(ctl, "-k", "graceful", "-f", inst.ApacheConfig); e != nil {
			return e
		}
		test := o
		test.Listen = "127.0.0.1:80"
		if e = waitReady(test); e != nil {
			return fmt.Errorf("Apache route verification: %w", e)
		}
	}
	b, _ := json.MarshalIndent(inst, "", "  ")
	if e = os.WriteFile(filepath.Join(*prefix, "etc", "installation.json"), b, 0600); e != nil {
		return e
	}
	fmt.Println("Installed:", *prefix)
	if useApache {
		fmt.Println("Open http://<server-ip>/k9s/ (existing Apache port 80)")
	} else {
		fmt.Println("Open http://" + o.Listen + "/k9s/ (replace wildcard address with the server IP)")
	}
	if !o.NoAuth {
		fmt.Println("Login username:", o.Username)
		fmt.Println("Read the generated password with: cat", o.PasswordFile)
	}
	fmt.Println("Manage:", filepath.Join(*prefix, "k9s-web-ui"), "start | stop | status")
	return nil
}
func waitReady(o options) error {
	address := o.Listen
	if strings.HasPrefix(address, "0.0.0.0:") {
		address = strings.Replace(address, "0.0.0.0:", "127.0.0.1:", 1)
	}
	pass := ""
	if !o.NoAuth {
		var e error
		pass, e = password(o.PasswordFile)
		if e != nil {
			return e
		}
	}
	c := http.Client{Timeout: 2 * time.Second}
	var last error
	for i := 0; i < 45; i++ {
		req, _ := http.NewRequest("GET", "http://"+address+"/k9s/", nil)
		if !o.NoAuth {
			req.SetBasicAuth(o.Username, pass)
		}
		r, e := c.Do(req)
		if e == nil {
			b, _ := io.ReadAll(r.Body)
			r.Body.Close()
			if r.StatusCode == 200 && strings.Contains(string(b), "/k9s/gpu.js") {
				return nil
			}
			last = fmt.Errorf("HTTP %d or wrong application at %s", r.StatusCode, address)
		} else {
			last = e
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("service readiness failed: %v", last)
}
func daemon(prefix string, o options) error {
	f, e := os.OpenFile(filepath.Join(prefix, "etc", "service.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	cmd := exec.Command(filepath.Join(prefix, "k9s-web-ui"), serveArgs(o)...)
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if e = cmd.Start(); e != nil {
		return e
	}
	pid := cmd.Process.Pid
	cmd.Process.Release()
	return os.WriteFile(filepath.Join(prefix, "etc", "service.pid"), []byte(strconv.Itoa(pid)), 0600)
}
func stopInstallation(prefix string, inst installation) error {
	if inst.Systemd {
		_, e := command("systemctl", "stop", "k9s-web-ui.service")
		return e
	}
	b, e := os.ReadFile(filepath.Join(prefix, "etc", "service.pid"))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	pid, e := strconv.Atoi(strings.TrimSpace(string(b)))
	if e != nil {
		return e
	}
	exe, e := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil || exe != filepath.Join(prefix, "k9s-web-ui") {
		return errors.New("PID does not match this installation; refusing to signal it")
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}
func control(action string) error {
	b, e := os.ReadFile(filepath.Join(bundle, "etc", "installation.json"))
	if e != nil {
		return fmt.Errorf("not an installed service: %w", e)
	}
	var inst installation
	if e = json.Unmarshal(b, &inst); e != nil {
		return e
	}
	switch action {
	case "stop":
		return stopInstallation(bundle, inst)
	case "start":
		if inst.Systemd {
			_, e = command("systemctl", "start", "k9s-web-ui.service")
		} else {
			b, _ := os.ReadFile(filepath.Join(bundle, "etc", "service.pid"))
			pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
			exe, _ := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
			if exe == filepath.Join(bundle, "k9s-web-ui") {
				return errors.New("service is already running")
			}
			e = daemon(bundle, inst.Options)
		}
		if e == nil {
			e = waitReady(inst.Options)
		}
		return e
	case "status":
		if inst.Systemd {
			out, e := command("systemctl", "status", "k9s-web-ui.service", "--no-pager")
			fmt.Print(out)
			return e
		}
		e = waitReady(inst.Options)
		if e == nil {
			fmt.Println("Service is responding at", inst.Options.Listen)
		}
		return e
	}
	return nil
}
