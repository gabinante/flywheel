package plan

import (
	"testing"
)

func TestClassifyManifest_ReadOnly(t *testing.T) {
	manifest := &SideEffectManifest{
		FileOps: []FileOp{
			{Action: "read", Path: "/app/config/settings.json"},
		},
	}
	got := ClassifyManifest(manifest)
	if got != ClassificationReadOnly {
		t.Fatalf("expected read_only, got: %s", got)
	}
}

func TestClassifyManifest_Nil(t *testing.T) {
	got := ClassifyManifest(nil)
	if got != ClassificationReadOnly {
		t.Fatalf("expected read_only for nil manifest, got: %s", got)
	}
}

func TestClassifyManifest_NarrowWrite_Mutating(t *testing.T) {
	manifest := &SideEffectManifest{
		FileOps: []FileOp{
			{Action: "write", Path: "/app/dist/bundle.js"},
		},
	}
	got := ClassifyManifest(manifest)
	if got != ClassificationMutating {
		t.Fatalf("expected mutating for narrow write, got: %s", got)
	}
}

func TestClassifyManifest_WildcardWrite_Destructive(t *testing.T) {
	manifest := &SideEffectManifest{
		FileOps: []FileOp{
			{Action: "write", Path: "/app/**"},
		},
	}
	got := ClassifyManifest(manifest)
	if got != ClassificationDestructive {
		t.Fatalf("expected destructive for wildcard write (**), got: %s", got)
	}
}

func TestClassifyManifest_SingleWildcardWrite_Mutating(t *testing.T) {
	manifest := &SideEffectManifest{
		FileOps: []FileOp{
			{Action: "write", Path: "/app/dist/*.js"},
		},
	}
	got := ClassifyManifest(manifest)
	if got != ClassificationMutating {
		t.Fatalf("expected mutating for single wildcard write, got: %s", got)
	}
}

func TestClassifyManifest_Delete_Mutating(t *testing.T) {
	manifest := &SideEffectManifest{
		FileOps: []FileOp{
			{Action: "delete", Path: "/tmp/specific-file.log"},
		},
	}
	got := ClassifyManifest(manifest)
	if got != ClassificationMutating {
		t.Fatalf("expected mutating for narrow delete, got: %s", got)
	}
}

func TestClassifyManifest_WildcardDelete_Destructive(t *testing.T) {
	manifest := &SideEffectManifest{
		FileOps: []FileOp{
			{Action: "delete", Path: "/app/cache/*"},
		},
	}
	got := ClassifyManifest(manifest)
	if got != ClassificationDestructive {
		t.Fatalf("expected destructive for wildcard delete, got: %s", got)
	}
}

func TestClassifyManifest_IdempotentNetwork_ReadOnly(t *testing.T) {
	manifest := &SideEffectManifest{
		NetworkOps: []NetworkOp{
			{Endpoint: "api.github.com:443", Method: "GET", Idempotent: true},
		},
	}
	got := ClassifyManifest(manifest)
	if got != ClassificationReadOnly {
		t.Fatalf("expected read_only for idempotent GET, got: %s", got)
	}
}

func TestClassifyManifest_NonIdempotentPost_Mutating(t *testing.T) {
	manifest := &SideEffectManifest{
		NetworkOps: []NetworkOp{
			{Endpoint: "api.github.com:443", Method: "POST", Idempotent: false},
		},
	}
	got := ClassifyManifest(manifest)
	if got != ClassificationMutating {
		t.Fatalf("expected mutating for non-idempotent POST, got: %s", got)
	}
}

func TestClassifyManifest_WildcardEndpoint_Destructive(t *testing.T) {
	manifest := &SideEffectManifest{
		NetworkOps: []NetworkOp{
			{Endpoint: "*:443", Method: "POST", Idempotent: false},
		},
	}
	got := ClassifyManifest(manifest)
	if got != ClassificationDestructive {
		t.Fatalf("expected destructive for wildcard endpoint, got: %s", got)
	}
}

func TestClassifyManifest_WildcardProcess_Destructive(t *testing.T) {
	manifest := &SideEffectManifest{
		ProcessOps: []ProcessOp{
			{Binary: "*"},
		},
	}
	got := ClassifyManifest(manifest)
	if got != ClassificationDestructive {
		t.Fatalf("expected destructive for wildcard process binary, got: %s", got)
	}
}

func TestClassifyManifest_NamedProcess_Idempotent(t *testing.T) {
	manifest := &SideEffectManifest{
		ProcessOps: []ProcessOp{
			{Binary: "npm", Args: []string{"install"}},
		},
	}
	got := ClassifyManifest(manifest)
	if got != ClassificationIdempotent {
		t.Fatalf("expected idempotent for named process, got: %s", got)
	}
}

func TestClassifyManifest_Credentials_AtLeastMutating(t *testing.T) {
	manifest := &SideEffectManifest{
		CredentialOps: []CredentialOp{
			{Name: "GITHUB_TOKEN", Purpose: "Read repos"},
		},
	}
	got := ClassifyManifest(manifest)
	if got != ClassificationMutating {
		t.Fatalf("expected at least mutating for credential access, got: %s", got)
	}
}

func TestClassifyManifest_CombinedNarrow_Mutating(t *testing.T) {
	// Narrow ops should classify by actual effect.
	manifest := &SideEffectManifest{
		FileOps: []FileOp{
			{Action: "read", Path: "/app/package.json"},
			{Action: "write", Path: "/app/dist/bundle.js"},
		},
		NetworkOps: []NetworkOp{
			{Endpoint: "registry.npmjs.org:443", Method: "GET", Idempotent: true},
		},
		ProcessOps: []ProcessOp{
			{Binary: "npm", Args: []string{"install"}},
		},
		ResourceLimits: ResourceLimits{
			MaxRuntimeSeconds: 60,
			MaxNetworkCalls:   10,
		},
	}
	got := ClassifyManifest(manifest)
	if got != ClassificationMutating {
		t.Fatalf("expected mutating for combined narrow ops, got: %s", got)
	}
}

func TestClassifyManifest_AnyWildcard_WidensToDestructive(t *testing.T) {
	// Even one wildcard op in an otherwise narrow manifest widens classification.
	manifest := &SideEffectManifest{
		FileOps: []FileOp{
			{Action: "read", Path: "/app/package.json"},
			{Action: "write", Path: "/**"}, // This one wildcard widens everything
		},
		NetworkOps: []NetworkOp{
			{Endpoint: "registry.npmjs.org:443", Method: "GET", Idempotent: true},
		},
	}
	got := ClassifyManifest(manifest)
	if got != ClassificationDestructive {
		t.Fatalf("expected destructive when any op has broad wildcard, got: %s", got)
	}
}

func TestClassifyManifest_IdempotentPost_Idempotent(t *testing.T) {
	manifest := &SideEffectManifest{
		NetworkOps: []NetworkOp{
			{Endpoint: "api.example.com:443", Method: "POST", Idempotent: true},
		},
	}
	got := ClassifyManifest(manifest)
	if got != ClassificationIdempotent {
		t.Fatalf("expected idempotent for idempotent POST, got: %s", got)
	}
}

func TestWidenClassification(t *testing.T) {
	tests := []struct {
		current  Classification
		candidate Classification
		expected Classification
	}{
		{ClassificationReadOnly, ClassificationReadOnly, ClassificationReadOnly},
		{ClassificationReadOnly, ClassificationIdempotent, ClassificationIdempotent},
		{ClassificationReadOnly, ClassificationDestructive, ClassificationDestructive},
		{ClassificationMutating, ClassificationIdempotent, ClassificationMutating},
		{ClassificationDestructive, ClassificationReadOnly, ClassificationDestructive},
	}
	for _, tt := range tests {
		got := widenClassification(tt.current, tt.candidate)
		if got != tt.expected {
			t.Errorf("widenClassification(%s, %s) = %s, want %s", tt.current, tt.candidate, got, tt.expected)
		}
	}
}

func TestCountWildcardLevel(t *testing.T) {
	tests := []struct {
		pattern string
		want    int
	}{
		{"/app/dist/bundle.js", 0},
		{"/app/dist/*.js", 1},
		{"/app/**", 2},
		{"/app/*/*.js", 2}, // Multiple single wildcards = level 2
		{"/exact/path", 0},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got := countWildcardLevel(tt.pattern)
			if got != tt.want {
				t.Errorf("countWildcardLevel(%q) = %d, want %d", tt.pattern, got, tt.want)
			}
		})
	}
}
