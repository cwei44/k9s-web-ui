package main

import (
	"strings"
	"testing"
)

func TestContainerPortSelection(t *testing.T) {
	for _, c := range []struct {
		apache, free bool
		want         string
		fail         bool
	}{
		{true, false, "127.0.0.1:19090", false}, {true, true, "127.0.0.1:19090", false},
		{false, true, "0.0.0.0:80", false}, {false, false, "", true},
	} {
		got, err := containerListen(c.apache, c.free, 19090)
		if got != c.want || (err != nil) != c.fail {
			t.Errorf("%+v: %q %v", c, got, err)
		}
	}
}

func TestContainerMigrationPreservesApacheAccessRules(t *testing.T) {
	original := `# Managed by setup-k9s-httpd.sh. Existing k9s-ui backend, Apache port 80.
IncludeOptional "/etc/httpd/k9s-gpu-routes.conf"
ProxyPass "/k9s/" "http://127.0.0.1:9090/k9s/"
ProxyPassReverse "/k9s/" "http://127.0.0.1:9090/k9s/"
<Location "/k9s/">
Require ip 127.0.0.1 ::1 192.0.2.10
</Location>
`
	got, err := migrateManagedApache(original, 19090)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "gpu-routes") || strings.Contains(got, ":9090/") || strings.Count(got, ":19090/") != 2 || !strings.Contains(got, "Require ip 127.0.0.1 ::1 192.0.2.10") {
		t.Fatal(got)
	}
	if _, err := migrateManagedApache("# my unrelated config\n"+original, 19090); err == nil {
		t.Fatal("unmanaged configuration accepted")
	}
}

func TestBridgeServiceSupportsPhotonSystemd(t *testing.T) {
	unit := bridgeService("/opt/k9s-web-ui-container", defaults(), "/run/k9s-web-ui-host/bridge.sock")
	if !strings.Contains(unit, "\nWorkingDirectory=/opt/k9s-web-ui-container\n") {
		t.Fatal("WorkingDirectory must be an unquoted absolute path on Photon systemd")
	}
	if strings.Contains(unit, "RuntimeDirectory=") {
		t.Fatal("systemd must not replace the directory Docker bind-mounts on helper restart")
	}
}

func TestContainerOnlyMountsPrivateBridgeAndDashboardState(t *testing.T) {
	for _, listen := range []string{"127.0.0.1:19090", "0.0.0.0:80"} {
		args := strings.Join(containerRunArgs("test-image", "/private/dashboard", "/run/private-helper", listen, "admin", false), " ")
		for _, required := range []string{"--network host", "--read-only", "--cap-drop ALL", "no-new-privileges", "src=/run/private-helper,dst=/host,readonly", "src=/private/dashboard,dst=/state", "--host-bridge /host/bridge.sock"} {
			if !strings.Contains(args, required) {
				t.Errorf("missing %s: %s", required, args)
			}
		}
		for _, forbidden := range []string{"docker.sock", "kubeconfig", "--privileged", "--pid", "src=/,"} {
			if strings.Contains(args, forbidden) {
				t.Errorf("unexpected host access %s", forbidden)
			}
		}
		if strings.Contains(args, "NET_BIND_SERVICE") != strings.HasSuffix(listen, ":80") {
			t.Fatal("wrong bind capability")
		}
	}
}
