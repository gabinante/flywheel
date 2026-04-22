package dispatch

import (
	"strings"
	"testing"

	"github.com/gabinante/flywheel/internal/plan"
)

func TestBuildSandboxConfig_NetworkDeny(t *testing.T) {
	// No network ops declared → network should be denied.
	policy := &plan.SandboxPolicy{
		TimeoutSeconds: 60,
	}
	cfg := BuildSandboxConfig(policy)
	found := false
	for i, arg := range cfg.DockerArgs {
		if arg == "--network" && i+1 < len(cfg.DockerArgs) && cfg.DockerArgs[i+1] == "none" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected --network none when no endpoints declared")
	}
}

func TestBuildSandboxConfig_NetworkAllow(t *testing.T) {
	policy := &plan.SandboxPolicy{
		AllowedEndpoints: []string{"api.github.com:443", "registry.npmjs.org:443"},
		TimeoutSeconds:   120,
		NetworkCallLimit: 20,
	}
	cfg := BuildSandboxConfig(policy)

	if cfg.EnvVars["FLYWHEEL_FIREWALL"] != "true" {
		t.Fatal("expected FLYWHEEL_FIREWALL=true when endpoints declared")
	}
	if !strings.Contains(cfg.EnvVars["FLYWHEEL_ALLOWED_HOSTS"], "api.github.com:443") {
		t.Fatal("expected api.github.com:443 in allowed hosts")
	}
	if cfg.EnvVars["FLYWHEEL_NETWORK_CALL_LIMIT"] != "20" {
		t.Fatalf("expected network call limit 20, got: %s", cfg.EnvVars["FLYWHEEL_NETWORK_CALL_LIMIT"])
	}
}

func TestBuildSandboxConfig_MountRestrictions(t *testing.T) {
	policy := &plan.SandboxPolicy{
		ReadOnlyMounts: []plan.MountSpec{
			{HostPath: "/app/config", ContainerPath: "/app/config", ReadOnly: true},
		},
		MountPaths: []plan.MountSpec{
			{HostPath: "/app/dist", ContainerPath: "/app/dist", ReadOnly: false},
		},
		TimeoutSeconds: 60,
	}
	cfg := BuildSandboxConfig(policy)

	hasRO := false
	hasRW := false
	for i, arg := range cfg.DockerArgs {
		if arg == "-v" && i+1 < len(cfg.DockerArgs) {
			if strings.HasSuffix(cfg.DockerArgs[i+1], ":ro") {
				hasRO = true
			}
			if strings.HasSuffix(cfg.DockerArgs[i+1], ":rw") {
				hasRW = true
			}
		}
	}
	if !hasRO {
		t.Fatal("expected read-only mount for /app/config")
	}
	if !hasRW {
		t.Fatal("expected read-write mount for /app/dist")
	}
}

func TestBuildSandboxConfig_SeccompProfile(t *testing.T) {
	policy := &plan.SandboxPolicy{
		SeccompProfile: "flywheel-strict",
		TimeoutSeconds: 60,
	}
	cfg := BuildSandboxConfig(policy)

	found := false
	for i, arg := range cfg.DockerArgs {
		if arg == "--security-opt" && i+1 < len(cfg.DockerArgs) {
			if strings.Contains(cfg.DockerArgs[i+1], "flywheel-strict") {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected seccomp profile flywheel-strict in docker args")
	}
}

func TestBuildSandboxConfig_DiskQuota(t *testing.T) {
	policy := &plan.SandboxPolicy{
		DiskQuotaBytes: 50 * 1024 * 1024, // 50MB
		TimeoutSeconds: 60,
	}
	cfg := BuildSandboxConfig(policy)

	found := false
	for i, arg := range cfg.DockerArgs {
		if arg == "--tmpfs" && i+1 < len(cfg.DockerArgs) {
			if strings.Contains(cfg.DockerArgs[i+1], "size=50m") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected tmpfs with size=50m, got args: %v", cfg.DockerArgs)
	}
}

func TestBuildSandboxConfig_SecretRestriction(t *testing.T) {
	policy := &plan.SandboxPolicy{
		SecretNames:    []string{"GITHUB_TOKEN", "NPM_TOKEN"},
		TimeoutSeconds: 60,
	}
	cfg := BuildSandboxConfig(policy)

	if cfg.EnvVars["FLYWHEEL_ALLOWED_SECRET_GITHUB_TOKEN"] != "true" {
		t.Fatal("expected GITHUB_TOKEN in allowed secrets")
	}
	if cfg.EnvVars["FLYWHEEL_ALLOWED_SECRET_NPM_TOKEN"] != "true" {
		t.Fatal("expected NPM_TOKEN in allowed secrets")
	}
}

func TestBuildSandboxConfig_ProcessRestriction(t *testing.T) {
	policy := &plan.SandboxPolicy{
		AllowedProcesses: []string{"npm", "git"},
		TimeoutSeconds:   60,
	}
	cfg := BuildSandboxConfig(policy)

	if cfg.EnvVars["FLYWHEEL_ALLOWED_PROCESSES"] != "npm,git" {
		t.Fatalf("expected 'npm,git' in allowed processes, got: %s", cfg.EnvVars["FLYWHEEL_ALLOWED_PROCESSES"])
	}
}

func TestManifestEnforcementScript_WithEndpoints(t *testing.T) {
	policy := &plan.SandboxPolicy{
		AllowedEndpoints: []string{"api.github.com:443", "registry.npmjs.org:443"},
		TimeoutSeconds:   120,
		NetworkCallLimit: 20,
	}
	script := ManifestEnforcementScript(policy)

	if !strings.Contains(script, "iptables -P OUTPUT DROP") {
		t.Fatal("expected default DROP policy in enforcement script")
	}
	if !strings.Contains(script, "api.github.com") {
		t.Fatal("expected api.github.com in iptables rules")
	}
	if !strings.Contains(script, "registry.npmjs.org") {
		t.Fatal("expected registry.npmjs.org in iptables rules")
	}
	if !strings.Contains(script, "FLYWHEEL_NETWORK_CALL_LIMIT=20") {
		t.Fatal("expected network call limit in script")
	}
}

func TestManifestEnforcementScript_NoEndpoints(t *testing.T) {
	policy := &plan.SandboxPolicy{
		TimeoutSeconds: 60,
	}
	script := ManifestEnforcementScript(policy)

	if strings.Contains(script, "iptables") {
		t.Fatal("expected no iptables rules when no endpoints declared")
	}
}

func TestSplitEndpoint(t *testing.T) {
	tests := []struct {
		input    string
		wantHost string
		wantPort string
	}{
		{"api.github.com:443", "api.github.com", "443"},
		{"*:443", "*", "443"},
		{"localhost:8080", "localhost", "8080"},
		{"noport", "noport", "443"},
	}
	for _, tt := range tests {
		host, port := splitEndpoint(tt.input)
		if host != tt.wantHost || port != tt.wantPort {
			t.Errorf("splitEndpoint(%q) = (%q, %q), want (%q, %q)", tt.input, host, port, tt.wantHost, tt.wantPort)
		}
	}
}
