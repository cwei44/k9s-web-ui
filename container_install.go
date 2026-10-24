// Copyright 2026 K9s Web UI contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const bridgeUnit = "k9s-web-ui-host.service"
const managedContainer = "k9s-web-ui"

type apacheChange struct {
	Path, Backup, Contents string
	Existed                bool
	Mode                   os.FileMode
}
type containerState struct {
	Image, Prefix, Socket, Listen, ApacheCommand, ApacheConfig string
	Systemd, ApacheWasRunning                                  bool
	BridgePID                                                  int
	Changes                                                    []apacheChange
}

func containerListen(hasApache, port80Free bool, backendPort int) (string, error) {
	if hasApache {
		return fmt.Sprintf("127.0.0.1:%d", backendPort), nil
	}
	if !port80Free {
		return "", errors.New("port 80 is occupied and Apache/httpd was not found; free port 80 before installing")
	}
	return "0.0.0.0:80", nil
}

func migrateManagedApache(original string, port int) (string, error) {
	if !strings.HasPrefix(original, "# Managed by setup-k9s-httpd.sh.") {
		return "", errors.New("existing /k9s route is not managed by the legacy installer")
	}
	target := "http://127.0.0.1:9090/k9s/"
	if !strings.Contains(original, target) {
		return "", errors.New("unrecognized legacy backend; configure the existing /k9s route manually")
	}
	lines := []string{}
	for _, line := range strings.Split(original, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "IncludeOptional ") && strings.Contains(line, "/k9s-gpu-routes.conf") {
			continue
		}
		lines = append(lines, strings.ReplaceAll(line, target, fmt.Sprintf("http://127.0.0.1:%d/k9s/", port)))
	}
	return strings.Replace(strings.Join(lines, "\n"), "# Managed by setup-k9s-httpd.sh. Existing k9s-ui backend, Apache port 80.", "# Managed by k9s-web-ui container installer. Original access rules retained.", 1), nil
}

func planContainerApache(ctl, override, prefix string, port int) (string, []apacheChange, error) {
	version, err := command(ctl, "-V")
	if err != nil {
		return "", nil, err
	}
	rootMatch := rootRE.FindStringSubmatch(version)
	if len(rootMatch) == 0 {
		return "", nil, errors.New("cannot locate Apache ServerRoot")
	}
	legacy := filepath.Join(rootMatch[1], "k9s-ui.conf")
	ignored := map[string]bool{}
	var migrated string
	if original, e := os.ReadFile(legacy); e == nil && strings.HasPrefix(string(original), "# Managed by setup-k9s-httpd.sh.") {
		migrated, err = migrateManagedApache(string(original), port)
		if err != nil {
			return "", nil, err
		}
		ignored[legacy] = true
		ignored[filepath.Join(rootMatch[1], "k9s-gpu-routes.conf")] = true
	}
	config, body, snippet, err := apacheSetupIgnoring(ctl, override, prefix, port, ignored)
	if err != nil {
		return "", nil, err
	}
	if migrated != "" {
		// Only migrate a file Apache actually includes; preserve Require/auth rules.
		includes, e := command(ctl, "-t", "-f", config, "-D", "DUMP_INCLUDES")
		if e != nil {
			return "", nil, e
		}
		if !strings.Contains(includes, legacy) {
			return "", nil, errors.New("legacy route file exists but is not included by this Apache configuration")
		}
		return config, []apacheChange{{Path: legacy, Contents: migrated}}, nil
	}
	original, err := os.ReadFile(config)
	if err != nil {
		return "", nil, err
	}
	return config, []apacheChange{{Path: snippet, Contents: body}, {Path: config, Contents: string(original) + "\nInclude " + quote(snippet) + "\n"}}, nil
}

func bridgeArguments(o options, socket string) []string {
	args := []string{"host-bridge", "--socket", socket, "--kubectl", o.Kubectl, "--kubeconfig", o.Kubeconfig, "--gpu-namespace", o.GPUNamespace, "--gpu-selector", o.GPUSelector}
	if o.Context != "" {
		args = append(args, "--context", o.Context)
	}
	if o.Namespace != "" {
		args = append(args, "--namespace", o.Namespace)
	}
	return args
}
func bridgeEnvironment() string {
	keys := []string{"HOME", "PATH", "KUBECONFIG", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "no_proxy", "all_proxy", "SSL_CERT_FILE", "SSL_CERT_DIR", "AWS_PROFILE", "AWS_DEFAULT_PROFILE", "AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE", "AWS_REGION", "AWS_DEFAULT_REGION", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "CLOUDSDK_CONFIG", "GOOGLE_APPLICATION_CREDENTIALS", "AZURE_CONFIG_DIR", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET", "AZURE_TENANT_ID", "AZURE_FEDERATED_TOKEN_FILE"}
	var lines []string
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			lines = append(lines, key+"="+strconv.Quote(value))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}
func bridgeService(prefix string, o options, socket string) string {
	args := []string{quote(filepath.Join(prefix, "k9s-web-ui"))}
	for _, arg := range bridgeArguments(o, socket) {
		args = append(args, quote(arg))
	}
	return "[Unit]\nDescription=K9s Web UI host kubectl helper\nAfter=network-online.target\nWants=network-online.target\n\n[Service]\nType=simple\nUser=root\nWorkingDirectory=" + prefix + "\nEnvironmentFile=" + quote(filepath.Join(prefix, "etc", "host.env")) + "\nExecStart=" + strings.Join(args, " ") + "\nRestart=on-failure\nRestartSec=3\nNoNewPrivileges=true\n\n[Install]\nWantedBy=multi-user.target\n"
}

func containerRunArgs(image, stateDir, socketDir, listen, user string, noAuth bool) []string {
	args := []string{"run", "-d", "--name", managedContainer, "--restart", "unless-stopped", "--network", "host", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--user", "0:0", "--mount", "type=bind,src=" + socketDir + ",dst=/host,readonly", "--mount", "type=bind,src=" + stateDir + ",dst=/state"}
	if strings.HasSuffix(listen, ":80") {
		args = append(args, "--cap-add", "NET_BIND_SERVICE")
	}
	args = append(args, image, "serve", "--host-bridge", "/host/bridge.sock", "--listen", listen, "--password-file", "/state/dashboard-password", "--username", user)
	if noAuth {
		args = append(args, "--no-auth")
	}
	return args
}

func installContainer(args []string) (err error) {
	if os.Geteuid() != 0 {
		return errors.New("container-install must run on the Docker host as root")
	}
	if remote := os.Getenv("DOCKER_HOST"); remote != "" && !strings.HasPrefix(remote, "unix://") {
		return errors.New("a local Linux Docker daemon is required")
	}
	if context := os.Getenv("DOCKER_CONTEXT"); context != "" && context != "default" {
		return errors.New("use the local default Docker context")
	}
	o := defaults()
	o.Kubectl = "kubectl"
	flags := flag.NewFlagSet("container-install", flag.ContinueOnError)
	optionsFlags(flags, &o)
	image := flags.String("image", "k9s-web-ui:local", "Locally built Docker image")
	prefix := flags.String("prefix", "/opt/k9s-web-ui-container", "Installation directory (must not already exist)")
	apacheConfig := flags.String("apache-config", "", "Apache main configuration override")
	backendPort := flags.Int("backend-port", 19090, "Loopback port behind Apache")
	if err = flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || o.HostBridge != "" {
		return errors.New("unexpected container installer arguments")
	}
	if !filepath.IsAbs(*prefix) || strings.ContainsAny(*prefix, "\n\r\"%,") {
		return errors.New("prefix must be absolute without quotes, newlines, percent signs or commas")
	}
	if _, e := os.Stat(*prefix); !os.IsNotExist(e) {
		return errors.New("installation directory already exists; stop/remove the managed deployment before reinstalling")
	}
	if *backendPort < 1024 || *backendPort > 65535 {
		return errors.New("backend port must be 1024 through 65535")
	}
	if _, err = exec.LookPath("docker"); err != nil {
		return errors.New("Docker Engine and CLI are required on this Linux host")
	}
	if _, err = command("docker", "info"); err != nil {
		return err
	}
	if _, e := command("docker", "inspect", managedContainer); e == nil {
		return errors.New("a container named k9s-web-ui already exists; refusing to replace it")
	}
	if _, err = command("docker", "image", "inspect", *image); err != nil {
		return err
	}
	client, err := exec.LookPath(o.Kubectl)
	if err != nil {
		return fmt.Errorf("host kubectl must already be installed: %w", err)
	}
	o.Kubectl, err = filepath.Abs(client)
	if err != nil {
		return err
	}
	paths := filepath.SplitList(o.Kubeconfig)
	for i, p := range paths {
		if p != "" {
			paths[i], err = filepath.Abs(p)
			if err != nil {
				return err
			}
		}
	}
	o.Kubeconfig = strings.Join(paths, string(os.PathListSeparator))
	// Never replace the host client or require a currently valid login to start
	// the UI: authorization failures are displayed in its connection banner.
	if _, err = o.run("version", "--client", "-o", "json"); err != nil {
		return fmt.Errorf("host kubectl cannot execute: %w", err)
	}
	// Keep the bind-mounted directory stable across helper restarts and host boots.
	// A systemd RuntimeDirectory would be deleted/replaced under Docker's mount.
	state := containerState{Image: *image, Prefix: *prefix, Socket: filepath.Join(*prefix, "run", "bridge.sock"), ApacheCommand: apacheExecutable()}
	state.ApacheWasRunning = !portFree(":80")
	state.Listen, err = containerListen(state.ApacheCommand != "", !state.ApacheWasRunning, *backendPort)
	if err != nil {
		return err
	}
	if !portFree(state.Listen) {
		return fmt.Errorf("dashboard listen address %s is busy", state.Listen)
	}
	if state.ApacheCommand != "" {
		state.ApacheConfig, state.Changes, err = planContainerApache(state.ApacheCommand, *apacheConfig, *prefix, *backendPort)
		if err != nil {
			return fmt.Errorf("Apache integration: %w", err)
		}
	}
	_, sdErr := os.Stat("/run/systemd/system")
	_, ctlErr := exec.LookPath("systemctl")
	state.Systemd = sdErr == nil && ctlErr == nil
	unitPath := filepath.Join("/etc/systemd/system", bridgeUnit)
	if state.Systemd {
		if _, e := os.Stat(unitPath); !os.IsNotExist(e) {
			return errors.New("host helper service already exists")
		}
	}
	if checkHostBridge(state.Socket) == nil {
		return errors.New("a host kubectl helper is already running")
	}
	if err = os.MkdirAll(filepath.Join(*prefix, "etc"), 0700); err != nil {
		return err
	}
	createdContainer, startedBridge, applied, apacheStarted := false, false, 0, false
	defer func() {
		if err == nil {
			return
		}
		for i := applied - 1; i >= 0; i-- {
			c := state.Changes[i]
			if c.Existed {
				copyFile(c.Backup, c.Path, c.Mode)
			} else {
				os.Remove(c.Path)
			}
		}
		if applied > 0 && state.ApacheWasRunning {
			command(state.ApacheCommand, "-k", "graceful", "-f", state.ApacheConfig)
		}
		if apacheStarted && !state.ApacheWasRunning {
			command(state.ApacheCommand, "-k", "stop", "-f", state.ApacheConfig)
		}
		if createdContainer {
			command("docker", "rm", "-f", managedContainer)
		}
		if startedBridge && state.Systemd {
			command("systemctl", "disable", "--now", bridgeUnit)
			os.Remove(unitPath)
			command("systemctl", "daemon-reload")
		}
		if startedBridge && !state.Systemd && state.BridgePID > 0 {
			syscall.Kill(state.BridgePID, syscall.SIGTERM)
		}
		fmt.Fprintln(os.Stderr, "Container installation rolled back; diagnostic files retained at", *prefix)
	}()
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	deployed := filepath.Join(*prefix, "k9s-web-ui")
	if err = copyFile(exe, deployed, 0755); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(state.Socket), 0700); err != nil {
		return err
	}
	if state.Systemd {
		if err = os.WriteFile(filepath.Join(*prefix, "etc", "host.env"), []byte(bridgeEnvironment()), 0600); err != nil {
			return err
		}
		if err = os.WriteFile(unitPath, []byte(bridgeService(*prefix, o, state.Socket)), 0644); err != nil {
			return err
		}
		startedBridge = true
		if _, err = command("systemctl", "daemon-reload"); err != nil {
			return err
		}
		if _, err = command("systemctl", "enable", "--now", bridgeUnit); err != nil {
			return err
		}
	} else {
		logFile, e := os.OpenFile(filepath.Join(*prefix, "etc", "host-bridge.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if e != nil {
			return e
		}
		cmd := exec.Command(deployed, bridgeArguments(o, state.Socket)...)
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		err = cmd.Start()
		logFile.Close()
		if err != nil {
			return err
		}
		state.BridgePID = cmd.Process.Pid
		cmd.Process.Release()
		startedBridge = true
		if err = os.WriteFile(filepath.Join(*prefix, "etc", "host-bridge.pid"), []byte(strconv.Itoa(state.BridgePID)), 0600); err != nil {
			return err
		}
	}
	for i := 0; i < 30; i++ {
		err = checkHostBridge(state.Socket)
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		return err
	}
	o.PasswordFile = filepath.Join(*prefix, "etc", "dashboard-password")
	if !o.NoAuth {
		if _, err = password(o.PasswordFile); err != nil {
			return err
		}
	}
	// Separate dashboard state from host-only environment/configuration backups.
	stateDir := filepath.Join(*prefix, "dashboard")
	if err = os.MkdirAll(stateDir, 0700); err != nil {
		return err
	}
	if !o.NoAuth {
		if err = copyFile(o.PasswordFile, filepath.Join(stateDir, "dashboard-password"), 0600); err != nil {
			return err
		}
	}
	runArgs := containerRunArgs(*image, stateDir, filepath.Dir(state.Socket), state.Listen, o.Username, o.NoAuth)
	if o.Context != "" {
		runArgs = append(runArgs, "--context", o.Context)
	}
	if o.Namespace != "" {
		runArgs = append(runArgs, "--namespace", o.Namespace)
	}
	runArgs = append(runArgs, "--gpu-namespace", o.GPUNamespace, "--gpu-selector", o.GPUSelector)
	createdContainer = true
	if _, err = command("docker", runArgs...); err != nil {
		return err
	}
	o.Listen = state.Listen
	if err = waitReady(o); err != nil {
		return err
	}
	for i := range state.Changes {
		c := &state.Changes[i]
		if st, e := os.Stat(c.Path); e == nil {
			c.Existed = true
			c.Mode = st.Mode().Perm()
			c.Backup = filepath.Join(*prefix, "etc", fmt.Sprintf("apache-before-%d.conf", i))
			if err = copyFile(c.Path, c.Backup, 0600); err != nil {
				return err
			}
		} else if !os.IsNotExist(e) {
			return e
		} else {
			c.Mode = 0644
		}
		applied = i + 1
		if err = os.WriteFile(c.Path, []byte(c.Contents), c.Mode); err != nil {
			return err
		}
	}
	if state.ApacheCommand != "" {
		if _, err = command(state.ApacheCommand, "-t", "-f", state.ApacheConfig); err != nil {
			return err
		}
		action := "graceful"
		if !state.ApacheWasRunning {
			action = "start"
			apacheStarted = true
		}
		if _, err = command(state.ApacheCommand, "-k", action, "-f", state.ApacheConfig); err != nil {
			return err
		}
		o.Listen = "127.0.0.1:80"
		if err = waitReady(o); err != nil {
			return fmt.Errorf("Apache port 80 verification: %w", err)
		}
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	if err = os.WriteFile(filepath.Join(*prefix, "etc", "container.json"), data, 0600); err != nil {
		return err
	}
	fmt.Println("Container ready: http://<host-ip>/k9s/")
	fmt.Println("Host kubectl:", o.Kubectl, "(never replaced or downloaded)")
	if state.ApacheCommand != "" {
		fmt.Println("Reusing existing Apache/httpd on port 80")
	} else {
		fmt.Println("Container web server owns port 80")
	}
	if !o.NoAuth {
		fmt.Println("Login:", o.Username, "; password file:", filepath.Join(stateDir, "dashboard-password"))
	}
	fmt.Println("Container: docker logs / stop / start", managedContainer)
	if state.Systemd {
		fmt.Println("Host helper: systemctl status", bridgeUnit)
	} else {
		fmt.Println("Host helper PID:", state.BridgePID, "; configure boot startup on this non-systemd host")
	}
	return nil
}
