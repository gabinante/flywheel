package plan

import (
	"testing"
)

func TestCheckFileAccess_DeclaredWrite_Allowed(t *testing.T) {
	manifest := &SideEffectManifest{
		FileOps: []FileOp{
			{Action: "write", Path: "/app/dist/**"},
		},
	}
	violation := CheckFileAccess(manifest, "/app/dist/bundle.js", "write")
	if violation != nil {
		t.Fatalf("expected no violation for declared write, got: %v", violation)
	}
}

func TestCheckFileAccess_UndeclaredWrite_Blocked(t *testing.T) {
	manifest := &SideEffectManifest{
		FileOps: []FileOp{
			{Action: "write", Path: "/app/dist/**"},
		},
	}
	violation := CheckFileAccess(manifest, "/etc/passwd", "write")
	if violation == nil {
		t.Fatal("expected violation for undeclared file write to /etc/passwd")
	}
	if violation.Operation != "file_write" {
		t.Fatalf("expected operation 'file_write', got: %s", violation.Operation)
	}
	if violation.Target != "/etc/passwd" {
		t.Fatalf("expected target '/etc/passwd', got: %s", violation.Target)
	}
}

func TestCheckFileAccess_ReadOnlyCannotWrite(t *testing.T) {
	manifest := &SideEffectManifest{
		FileOps: []FileOp{
			{Action: "read", Path: "/app/config/**"},
		},
	}
	violation := CheckFileAccess(manifest, "/app/config/settings.json", "write")
	if violation == nil {
		t.Fatal("expected violation: read-only manifest should not allow writes")
	}
}

func TestCheckFileAccess_ExactPathMatch(t *testing.T) {
	manifest := &SideEffectManifest{
		FileOps: []FileOp{
			{Action: "write", Path: "/tmp/output.log"},
		},
	}
	// Exact match should work
	violation := CheckFileAccess(manifest, "/tmp/output.log", "write")
	if violation != nil {
		t.Fatalf("expected exact path match to succeed, got: %v", violation)
	}
	// Different file should fail
	violation = CheckFileAccess(manifest, "/tmp/other.log", "write")
	if violation == nil {
		t.Fatal("expected violation for non-matching path")
	}
}

func TestCheckFileAccess_GlobPattern(t *testing.T) {
	manifest := &SideEffectManifest{
		FileOps: []FileOp{
			{Action: "write", Path: "/tmp/*.log"},
		},
	}
	violation := CheckFileAccess(manifest, "/tmp/app.log", "write")
	if violation != nil {
		t.Fatalf("expected glob match to succeed, got: %v", violation)
	}
}

func TestCheckFileAccess_DeleteNotDeclared(t *testing.T) {
	manifest := &SideEffectManifest{
		FileOps: []FileOp{
			{Action: "write", Path: "/app/**"},
		},
	}
	violation := CheckFileAccess(manifest, "/app/tempfile", "delete")
	if violation == nil {
		t.Fatal("expected violation: write permission should not grant delete")
	}
}

func TestCheckNetworkAccess_DeclaredEndpoint_Allowed(t *testing.T) {
	manifest := &SideEffectManifest{
		NetworkOps: []NetworkOp{
			{Endpoint: "api.github.com:443", Method: "GET", Idempotent: true},
		},
	}
	violation := CheckNetworkAccess(manifest, "api.github.com:443", "GET")
	if violation != nil {
		t.Fatalf("expected no violation for declared network access, got: %v", violation)
	}
}

func TestCheckNetworkAccess_UndeclaredEndpoint_Blocked(t *testing.T) {
	manifest := &SideEffectManifest{
		NetworkOps: []NetworkOp{
			{Endpoint: "api.github.com:443", Method: "GET", Idempotent: true},
		},
	}
	violation := CheckNetworkAccess(manifest, "evil.com:443", "POST")
	if violation == nil {
		t.Fatal("expected violation for undeclared network call to evil.com")
	}
	if violation.Operation != "network" {
		t.Fatalf("expected operation 'network', got: %s", violation.Operation)
	}
}

func TestCheckNetworkAccess_WrongMethod_Blocked(t *testing.T) {
	manifest := &SideEffectManifest{
		NetworkOps: []NetworkOp{
			{Endpoint: "api.github.com:443", Method: "GET", Idempotent: true},
		},
	}
	violation := CheckNetworkAccess(manifest, "api.github.com:443", "POST")
	if violation == nil {
		t.Fatal("expected violation: GET-only declaration should block POST")
	}
}

func TestCheckNetworkAccess_WildcardEndpoint(t *testing.T) {
	manifest := &SideEffectManifest{
		NetworkOps: []NetworkOp{
			{Endpoint: "*:443", Method: "GET", Idempotent: true},
		},
	}
	violation := CheckNetworkAccess(manifest, "anything.com:443", "GET")
	if violation != nil {
		t.Fatalf("expected wildcard endpoint to match, got: %v", violation)
	}
}

func TestCheckNetworkAccess_EmptyManifest_Blocked(t *testing.T) {
	manifest := &SideEffectManifest{}
	violation := CheckNetworkAccess(manifest, "api.github.com:443", "GET")
	if violation == nil {
		t.Fatal("expected violation: empty manifest should block all network access")
	}
}

func TestCheckProcessSpawn_DeclaredBinary_Allowed(t *testing.T) {
	manifest := &SideEffectManifest{
		ProcessOps: []ProcessOp{
			{Binary: "npm", Args: []string{"install", "run", "build"}},
		},
	}
	violation := CheckProcessSpawn(manifest, "npm", []string{"install"})
	if violation != nil {
		t.Fatalf("expected no violation for declared process, got: %v", violation)
	}
}

func TestCheckProcessSpawn_UndeclaredBinary_Blocked(t *testing.T) {
	manifest := &SideEffectManifest{
		ProcessOps: []ProcessOp{
			{Binary: "npm"},
		},
	}
	violation := CheckProcessSpawn(manifest, "rm", []string{"-rf", "/"})
	if violation == nil {
		t.Fatal("expected violation for undeclared process 'rm'")
	}
}

func TestCheckProcessSpawn_BaseNameMatch(t *testing.T) {
	manifest := &SideEffectManifest{
		ProcessOps: []ProcessOp{
			{Binary: "git"},
		},
	}
	violation := CheckProcessSpawn(manifest, "/usr/bin/git", []string{"status"})
	if violation != nil {
		t.Fatalf("expected base name match, got: %v", violation)
	}
}

func TestCheckCredentialAccess_Declared_Allowed(t *testing.T) {
	manifest := &SideEffectManifest{
		CredentialOps: []CredentialOp{
			{Name: "GITHUB_TOKEN", Purpose: "Push to repository"},
		},
	}
	violation := CheckCredentialAccess(manifest, "GITHUB_TOKEN")
	if violation != nil {
		t.Fatalf("expected no violation for declared credential, got: %v", violation)
	}
}

func TestCheckCredentialAccess_Undeclared_Blocked(t *testing.T) {
	manifest := &SideEffectManifest{
		CredentialOps: []CredentialOp{
			{Name: "GITHUB_TOKEN"},
		},
	}
	violation := CheckCredentialAccess(manifest, "AWS_SECRET_KEY")
	if violation == nil {
		t.Fatal("expected violation for undeclared credential 'AWS_SECRET_KEY'")
	}
}

func TestGenerateSandboxPolicy_FullManifest(t *testing.T) {
	manifest := &SideEffectManifest{
		FileOps: []FileOp{
			{Action: "read", Path: "/app/config/**"},
			{Action: "write", Path: "/app/dist/**"},
			{Action: "delete", Path: "/tmp/*.log"},
		},
		NetworkOps: []NetworkOp{
			{Endpoint: "api.github.com:443", Method: "GET", Idempotent: true},
			{Endpoint: "registry.npmjs.org:443", Method: "GET", Idempotent: true},
		},
		ProcessOps: []ProcessOp{
			{Binary: "npm", Args: []string{"install", "run"}},
			{Binary: "git", Args: []string{"status", "log"}},
		},
		CredentialOps: []CredentialOp{
			{Name: "NPM_TOKEN", Purpose: "Install private packages"},
		},
		ResourceLimits: ResourceLimits{
			MaxRuntimeSeconds: 120,
			MaxDiskWriteBytes: 50 * 1024 * 1024,
			MaxNetworkCalls:   20,
		},
	}

	policy := GenerateSandboxPolicy(manifest)

	if len(policy.ReadOnlyMounts) != 1 {
		t.Fatalf("expected 1 read-only mount, got: %d", len(policy.ReadOnlyMounts))
	}
	if len(policy.MountPaths) != 2 { // write + delete
		t.Fatalf("expected 2 writable mounts, got: %d", len(policy.MountPaths))
	}
	if len(policy.AllowedEndpoints) != 2 {
		t.Fatalf("expected 2 allowed endpoints, got: %d", len(policy.AllowedEndpoints))
	}
	if len(policy.AllowedProcesses) != 2 {
		t.Fatalf("expected 2 allowed processes, got: %d", len(policy.AllowedProcesses))
	}
	if len(policy.SecretNames) != 1 {
		t.Fatalf("expected 1 secret name, got: %d", len(policy.SecretNames))
	}
	if policy.TimeoutSeconds != 120 {
		t.Fatalf("expected timeout 120s, got: %d", policy.TimeoutSeconds)
	}
	if policy.DiskQuotaBytes != 50*1024*1024 {
		t.Fatalf("expected disk quota 50MB, got: %d", policy.DiskQuotaBytes)
	}
	if policy.NetworkCallLimit != 20 {
		t.Fatalf("expected network call limit 20, got: %d", policy.NetworkCallLimit)
	}
}

func TestGenerateSandboxPolicy_Defaults(t *testing.T) {
	manifest := &SideEffectManifest{
		FileOps: []FileOp{
			{Action: "write", Path: "/tmp/out"},
		},
	}
	policy := GenerateSandboxPolicy(manifest)

	if policy.TimeoutSeconds != DefaultTimeoutSeconds {
		t.Fatalf("expected default timeout %d, got: %d", DefaultTimeoutSeconds, policy.TimeoutSeconds)
	}
	if policy.DiskQuotaBytes != DefaultDiskQuotaBytes {
		t.Fatalf("expected default disk quota, got: %d", policy.DiskQuotaBytes)
	}
	if policy.NetworkCallLimit != DefaultNetworkCallLimit {
		t.Fatalf("expected default network call limit, got: %d", policy.NetworkCallLimit)
	}
}

func TestGenerateSandboxPolicy_SeccompProfile(t *testing.T) {
	tests := []struct {
		name     string
		manifest *SideEffectManifest
		expected string
	}{
		{
			"strict - no network, no process",
			&SideEffectManifest{
				FileOps: []FileOp{{Action: "write", Path: "/tmp/out"}},
			},
			"flywheel-strict",
		},
		{
			"network - has network, no process",
			&SideEffectManifest{
				NetworkOps: []NetworkOp{{Endpoint: "api.example.com:443", Method: "GET"}},
			},
			"flywheel-network",
		},
		{
			"default - has process ops",
			&SideEffectManifest{
				ProcessOps: []ProcessOp{{Binary: "npm"}},
			},
			"flywheel-default",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := GenerateSandboxPolicy(tt.manifest)
			if policy.SeccompProfile != tt.expected {
				t.Errorf("expected seccomp profile %q, got %q", tt.expected, policy.SeccompProfile)
			}
		})
	}
}

func TestMatchGlobPath(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"/app/dist/**", "/app/dist/bundle.js", true},
		{"/app/dist/**", "/app/dist/sub/file.js", true},
		{"/app/dist/**", "/etc/passwd", false},
		{"/tmp/*.log", "/tmp/app.log", true},
		{"/tmp/*.log", "/tmp/sub/app.log", false},
		{"/app/config/", "/app/config/settings.json", true},
		{"/exact/file.txt", "/exact/file.txt", true},
		{"/exact/file.txt", "/exact/other.txt", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			got := matchGlobPath(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("matchGlobPath(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchEndpoint(t *testing.T) {
	tests := []struct {
		declared string
		target   string
		want     bool
	}{
		{"api.github.com:443", "api.github.com:443", true},
		{"api.github.com:443", "evil.com:443", false},
		{"*:443", "anything.com:443", true},
		{"*:443", "anything.com:80", false},
	}
	for _, tt := range tests {
		t.Run(tt.declared+"_"+tt.target, func(t *testing.T) {
			got := matchEndpoint(tt.declared, tt.target)
			if got != tt.want {
				t.Errorf("matchEndpoint(%q, %q) = %v, want %v", tt.declared, tt.target, got, tt.want)
			}
		})
	}
}
