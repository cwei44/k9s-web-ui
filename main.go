// Copyright 2026 K9s Web UI contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
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
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed web/*
var assets embed.FS

const packageVersion = "1.3.0"

var bundle string

type options struct {
	HostBridge                                                                                         string
	Kubeconfig, Context, Namespace, Kubectl, Listen, PasswordFile, Username, GPUNamespace, GPUSelector string
	Offline, NoAuth                                                                                    bool
}

func defaults() options {
	home, _ := os.UserHomeDir()
	kc := os.Getenv("KUBECONFIG")
	if kc == "" {
		kc = filepath.Join(home, ".kube", "config")
	}
	config, _ := os.UserConfigDir()
	return options{Kubeconfig: kc, Kubectl: filepath.Join(bundle, "bin", "kubectl"), Listen: "127.0.0.1:8080", PasswordFile: filepath.Join(config, "k9s-web-ui", "password"), Username: "admin", GPUNamespace: "auto", GPUSelector: "app=nvidia-dcgm-exporter"}
}
func optionsFlags(fs *flag.FlagSet, o *options) {
	fs.StringVar(&o.HostBridge, "host-bridge", o.HostBridge, "Unix socket for the host kubectl helper (no client download or kubeconfig inside container)")
	fs.StringVar(&o.Kubeconfig, "kubeconfig", o.Kubeconfig, "Optional override; defaults to host KUBECONFIG or ~/.kube/config")
	fs.StringVar(&o.Context, "context", o.Context, "Kubernetes context; default is current context")
	fs.StringVar(&o.Namespace, "namespace", o.Namespace, "Limit dashboard resources to a namespace")
	fs.StringVar(&o.Kubectl, "kubectl", o.Kubectl, "kubectl override; defaults to bundled, host, or verified download")
	fs.StringVar(&o.Listen, "listen", o.Listen, "Frontend listen address")
	fs.StringVar(&o.PasswordFile, "password-file", o.PasswordFile, "Dashboard password file; generated if absent")
	fs.StringVar(&o.Username, "username", o.Username, "Dashboard HTTP login username")
	fs.StringVar(&o.GPUNamespace, "gpu-namespace", o.GPUNamespace, "DCGM namespace, or auto to discover across namespaces")
	fs.StringVar(&o.GPUSelector, "gpu-selector", o.GPUSelector, "DCGM exporter pod selector")
	fs.BoolVar(&o.Offline, "offline", false, "Never download a compatible kubectl; require a bundled/cached compatible client")
	fs.BoolVar(&o.NoAuth, "no-auth", false, "Explicitly disable dashboard authentication (trusted network only)")
}
func main() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	exe, _ = filepath.EvalSymlinks(exe)
	bundle = filepath.Dir(exe)
	os.Setenv("PATH", filepath.Join(bundle, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := os.Stat(filepath.Join(bundle, "certs", "ca-bundle.crt")); err == nil && os.Getenv("SSL_CERT_FILE") == "" {
		os.Setenv("SSL_CERT_FILE", filepath.Join(bundle, "certs", "ca-bundle.crt"))
	}
	if len(os.Args) < 2 {
		usage()
		return
	}
	switch os.Args[1] {
	case "serve", "doctor":
		o := defaults()
		fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
		optionsFlags(fs, &o)
		fs.Parse(os.Args[2:])
		if fs.NArg() != 0 {
			log.Fatal("Unexpected positional arguments")
		}
		if err = prepare(&o); err == nil && os.Args[1] == "serve" {
			err = serve(o)
		}
	case "install":
		err = install(os.Args[2:])
	case "container-install":
		err = installContainer(os.Args[2:])
	case "container-update":
		err = updateContainer(os.Args[2:])
	case "host-bridge":
		err = hostBridgeCommand(os.Args[2:])
	case "start", "stop", "status":
		err = control(os.Args[1])
	case "version":
		fmt.Println("k9s-web-ui", packageVersion)
	case "help", "--help", "-h":
		usage()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		log.Fatal(err)
	}
}
func usage() {
	fmt.Println(`K9s Web UI — read-only Kubernetes dashboard with NVIDIA GPU metrics
  ./k9s-web-ui serve [--listen 0.0.0.0:8080]
  ./k9s-web-ui doctor [--offline]
  ./k9s-web-ui install [--mode auto|apache|standalone]   (run as root)
  ./scripts/install-container.sh                       (Docker deployment, run on host as root)
  ./scripts/update-container.sh                        (update an existing container deployment)
  ./k9s-web-ui version
Use a command's --help for its options. Authentication is enabled by default.
Uses the current Kubernetes context from KUBECONFIG or ~/.kube/config.
No kubeconfig argument is needed when kubectl already works for this account.`)
}
func (o options) args(a ...string) []string {
	r := []string{"--request-timeout=12s"}
	if o.Context != "" {
		r = append(r, "--context", o.Context)
	}
	return append(r, a...)
}
func (o options) run(a ...string) ([]byte, error) {
	if o.HostBridge != "" {
		return callHostBridge(o, a...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, o.Kubectl, o.args(a...)...)
	cmd.Env = append(os.Environ(), "KUBECONFIG="+o.Kubeconfig)
	out, e := cmd.Output()
	if e != nil {
		if ee, ok := e.(*exec.ExitError); ok {
			msg := strings.TrimSpace(string(ee.Stderr))
			if msg == "" {
				msg = e.Error()
			}
			if len(msg) > 700 {
				msg = msg[:700]
			}
			return nil, fmt.Errorf("kubectl: %s", msg)
		}
		return nil, e
	}
	return out, nil
}

var versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

func minor(v string) (int, error) {
	m := versionPattern.FindStringSubmatch(v)
	if len(m) == 0 || m[1] != "1" {
		return 0, fmt.Errorf("unsupported Kubernetes version %q", v)
	}
	return strconv.Atoi(m[2])
}
func prepare(o *options) error {
	if o.HostBridge != "" {
		if err := checkHostBridge(o.HostBridge); err != nil {
			return err
		}
		log.Printf("Using host kubectl through %s; Kubernetes login stays on the host", o.HostBridge)
		return nil
	}
	if resolved, e := exec.LookPath(o.Kubectl); e == nil {
		o.Kubectl, _ = filepath.Abs(resolved)
	}

	if st, err := os.Stat(o.Kubectl); err != nil || st.Mode()&0111 == 0 {
		if candidate, err := exec.LookPath("kubectl"); err == nil && o.Kubectl == filepath.Join(bundle, "bin", "kubectl") {
			o.Kubectl, _ = filepath.Abs(candidate)
		} else if o.Kubectl == filepath.Join(bundle, "bin", "kubectl") {
			cache, err := os.UserCacheDir()
			if err != nil {
				return err
			}
			candidate := filepath.Join(cache, "k9s-web-ui", "kubectl", "v1.35.9", "kubectl")
			if !validCached(candidate) {
				if o.Offline {
					return errors.New("kubectl is unavailable; install it or run once without --offline")
				}
				if err = downloadKubectl("v1.35.9", candidate); err != nil {
					return err
				}
			}
			o.Kubectl = candidate
		} else {
			return fmt.Errorf("required executable missing: %s", o.Kubectl)
		}
	}
	existing := 0
	for _, p := range filepath.SplitList(o.Kubeconfig) {
		if p == "" {
			continue
		}
		if _, e := os.Stat(p); e != nil {
			if os.IsNotExist(e) {
				continue
			}
			return fmt.Errorf("kubeconfig %s: %w", p, e)
		}
		existing++
	}
	if existing == 0 {
		return errors.New("no host kubeconfig found; sign in to Kubernetes with kubectl for this account first")
	}
	out, e := o.run("config", "view", "--minify", "-o", "json")
	if e != nil {
		return e
	}
	var cfg struct {
		CurrentContext string `json:"current-context"`
		Users          []struct {
			User struct {
				Exec *struct {
					Command string `json:"command"`
				} `json:"exec"`
				AuthProvider *struct {
					Name string `json:"name"`
				} `json:"auth-provider"`
			} `json:"user"`
		} `json:"users"`
	}
	if e = json.Unmarshal(out, &cfg); e != nil {
		return e
	}
	for _, u := range cfg.Users {
		if u.User.Exec != nil {
			cmd := u.User.Exec.Command
			_, e := exec.LookPath(cmd)
			if e != nil && !strings.Contains(cmd, "/") {
				return fmt.Errorf("kubeconfig requires external credential helper %q. Install/sign in with that provider's CLI, then retry; provider credentials cannot be bundled", cmd)
			}
		}
	}
	client, e := o.run("version", "--client", "-o", "json")
	if e != nil {
		return e
	}
	var cv struct {
		ClientVersion struct {
			GitVersion string `json:"gitVersion"`
		} `json:"clientVersion"`
	}
	if e = json.Unmarshal(client, &cv); e != nil {
		return e
	}
	server, e := o.run("get", "--raw", "/version")
	if e != nil {
		return fmt.Errorf("cluster access failed (check kubeconfig, VPN and provider login): %w", e)
	}
	var sv struct {
		GitVersion string `json:"gitVersion"`
	}
	if e = json.Unmarshal(server, &sv); e != nil {
		return e
	}
	cm, e := minor(cv.ClientVersion.GitVersion)
	if e != nil {
		return e
	}
	sm, e := minor(sv.GitVersion)
	if e != nil {
		return e
	}
	if cm-sm > 1 || sm-cm > 1 {
		matched := versionPattern.FindString(sv.GitVersion)
		if !strings.HasPrefix(matched, "v") {
			matched = "v" + matched
		}
		cache, e := os.UserCacheDir()
		if e != nil {
			return e
		}
		target := filepath.Join(cache, "k9s-web-ui", "kubectl", matched, "kubectl")
		// Both cached and downloaded clients are validated against their stored checksum.
		if !validCached(target) {
			if o.Offline {
				return fmt.Errorf("bundled kubectl %s is outside supported skew for %s. Supply --kubectl /compatible/client or run once without --offline to resolve it", cv.ClientVersion.GitVersion, sv.GitVersion)
			}
			if e = downloadKubectl(matched, target); e != nil {
				return e
			}
		}
		o.Kubectl = target
	}
	log.Printf("Host Kubernetes context %q; cluster %s reachable; kubectl: %s", cfg.CurrentContext, sv.GitVersion, o.Kubectl)
	return nil
}
func validCached(path string) bool {
	b, e := os.ReadFile(path)
	if e != nil {
		return false
	}
	sum, e := os.ReadFile(path + ".sha256")
	if e != nil {
		return false
	}
	s := sha256.Sum256(b)
	return strings.TrimSpace(string(sum)) == hex.EncodeToString(s[:])
}
func getURL(url string, limit int64) ([]byte, error) {
	c := http.Client{Timeout: 3 * time.Minute}
	r, e := c.Get(url)
	if e != nil {
		return nil, e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, fmt.Errorf("download %s: HTTP %d", url, r.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if int64(len(b)) > limit {
		return nil, errors.New("download exceeds size limit")
	}
	return b, e
}
func downloadKubectl(version, target string) error {
	url := "https://dl.k8s.io/release/" + version + "/bin/linux/amd64/kubectl"
	log.Printf("Resolving compatible kubectl %s from Kubernetes", version)
	sum, e := getURL(url+".sha256", 1024)
	if e != nil {
		return e
	}
	b, e := getURL(url, 150<<20)
	if e != nil {
		return e
	}
	s := sha256.Sum256(b)
	if hex.EncodeToString(s[:]) != strings.TrimSpace(string(sum)) {
		return errors.New("kubectl checksum mismatch")
	}
	if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(target), ".kubectl-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Chmod(0755); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(f.Name(), target); e != nil {
		return e
	}
	return os.WriteFile(target+".sha256", sum, 0600)
}
func password(path string) (string, error) {
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return "", e
		}
		raw := make([]byte, 18)
		if _, e = rand.Read(raw); e != nil {
			return "", e
		}
		b = []byte(hex.EncodeToString(raw))
		f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return "", e
		}
		_, e = f.Write(append(b, '\n'))
		ce := f.Close()
		if e != nil {
			return "", e
		}
		if ce != nil {
			return "", ce
		}
		log.Printf("Generated dashboard password: %s (read this file to log in)", path)
	} else if e != nil {
		return "", e
	}
	p := strings.TrimSpace(string(b))
	if p == "" {
		return "", errors.New("dashboard password file is empty")
	}
	return p, nil
}
func auth(next http.Handler, user, pass string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		uh, ph := sha256.Sum256([]byte(u)), sha256.Sum256([]byte(p))
		wantU, wantP := sha256.Sum256([]byte(user)), sha256.Sum256([]byte(pass))
		if !ok || subtle.ConstantTimeCompare(uh[:], wantU[:]) != 1 || subtle.ConstantTimeCompare(ph[:], wantP[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="K9s portable", charset="UTF-8"`)
			http.Error(w, "Authentication required", 401)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func serve(o options) error {
	pass := ""
	var err error
	if !o.NoAuth {
		pass, err = password(o.PasswordFile)
		if err != nil {
			return err
		}
	}
	listener, err := net.Listen("tcp", o.Listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	var handler http.Handler = dashboardHandler(o)
	if !o.NoAuth {
		handler = auth(handler, o.Username, pass)
	}
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	errs := make(chan error, 1)
	go func() { errs <- srv.Serve(listener) }()
	log.Printf("K9s Web UI ready: http://%s/k9s/", listener.Addr())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case <-signals:
	case err = <-errs:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
