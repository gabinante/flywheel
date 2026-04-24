package delivery

import (
	"testing"

	"github.com/gabinante/flywheel/internal/project"
)

func TestParseConfig_Empty(t *testing.T) {
	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if !isZeroConfig(cfg) {
		t.Fatalf("expected zero config, got %#v", cfg)
	}
}

func TestParseConfig_Invalid(t *testing.T) {
	_, err := ParseConfig(map[string]string{ConfigKey: "{"})
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestStoreConfig_RoundTrip(t *testing.T) {
	cfg := Config{
		SCM: SCMConfig{Provider: "github"},
		Infrastructure: InfrastructureConfig{
			Provider: "flyio",
			FlyIO: &FlyIOConfig{
				Apps: []FlyIOAppConfig{
					{AppName: "api-prod", EnvironmentSlug: "prod", Service: "api"},
				},
			},
		},
	}

	pack, err := StoreConfig(project.ContextPack{}, cfg)
	if err != nil {
		t.Fatalf("StoreConfig returned error: %v", err)
	}
	if pack.Extra[ConfigKey] == "" {
		t.Fatal("expected config to be stored")
	}

	parsed, err := ParseConfig(pack.Extra)
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if parsed.Infrastructure.Provider != "flyio" {
		t.Fatalf("expected flyio provider, got %q", parsed.Infrastructure.Provider)
	}
	if parsed.Infrastructure.FlyIO == nil || len(parsed.Infrastructure.FlyIO.Apps) != 1 {
		t.Fatalf("expected app mapping, got %#v", parsed.Infrastructure.FlyIO)
	}
}

func TestStoreConfig_ZeroDeletes(t *testing.T) {
	pack := project.ContextPack{
		Extra: map[string]string{ConfigKey: `{"scm":{"provider":"github"}}`},
	}
	next, err := StoreConfig(pack, Config{})
	if err != nil {
		t.Fatalf("StoreConfig returned error: %v", err)
	}
	if _, ok := next.Extra[ConfigKey]; ok {
		t.Fatal("expected config key to be removed")
	}
}

func TestMaskToken(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"   ", ""},
		{"short", "***"},
		{"12345678", "***"},
		{"fo1_abcdefghijk", "fo1_***ijk"},
		{"fo1_xyzLONGTOKENVALUEabc", "fo1_***abc"},
	}
	for _, tt := range tests {
		got := MaskToken(tt.input)
		if got != tt.want {
			t.Errorf("MaskToken(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMaskedConfigResponse_NoCredentials(t *testing.T) {
	cfg := Config{SCM: SCMConfig{Provider: "github"}}
	resp := MaskedConfigResponse(cfg)
	if resp.Credentials.FlyIOConfigured {
		t.Fatal("expected flyio_configured=false when no credentials")
	}
	if resp.Credentials.FlyIOMasked != "" {
		t.Fatalf("expected empty masked value, got %q", resp.Credentials.FlyIOMasked)
	}
	if resp.SCM.Provider != "github" {
		t.Fatalf("expected SCM provider github, got %q", resp.SCM.Provider)
	}
}

func TestMaskedConfigResponse_WithCredentials(t *testing.T) {
	cfg := Config{
		Infrastructure: InfrastructureConfig{Provider: "flyio"},
		Credentials:    &Credentials{FlyIOAPIToken: "fo1_my_secret_token_xyz"},
	}
	resp := MaskedConfigResponse(cfg)
	if !resp.Credentials.FlyIOConfigured {
		t.Fatal("expected flyio_configured=true")
	}
	if resp.Credentials.FlyIOMasked == "" {
		t.Fatal("expected non-empty masked value")
	}
	// Must not contain the raw token
	if resp.Credentials.FlyIOMasked == "fo1_my_secret_token_xyz" {
		t.Fatal("masked value must not equal raw token")
	}
}

func TestResolveFlyIOToken_ProjectFirst(t *testing.T) {
	cfg := Config{
		Credentials: &Credentials{FlyIOAPIToken: "project-token"},
	}
	got := ResolveFlyIOToken(cfg, "env-fallback")
	if got != "project-token" {
		t.Fatalf("expected project-token, got %q", got)
	}
}

func TestResolveFlyIOToken_FallbackWhenEmpty(t *testing.T) {
	cfg := Config{} // no credentials
	got := ResolveFlyIOToken(cfg, "env-fallback")
	if got != "env-fallback" {
		t.Fatalf("expected env-fallback, got %q", got)
	}
}

func TestResolveFlyIOToken_FallbackWhenNilCredentials(t *testing.T) {
	cfg := Config{Credentials: nil}
	got := ResolveFlyIOToken(cfg, "env-fallback")
	if got != "env-fallback" {
		t.Fatalf("expected env-fallback, got %q", got)
	}
}

func TestResolveFlyIOToken_FallbackWhenWhitespace(t *testing.T) {
	cfg := Config{Credentials: &Credentials{FlyIOAPIToken: "   "}}
	got := ResolveFlyIOToken(cfg, "env-fallback")
	if got != "env-fallback" {
		t.Fatalf("expected env-fallback, got %q", got)
	}
}

func TestStoreConfig_WithCredentials_RoundTrip(t *testing.T) {
	cfg := Config{
		Infrastructure: InfrastructureConfig{Provider: "flyio"},
		Credentials:    &Credentials{FlyIOAPIToken: "fo1_secret_value"},
	}
	pack, err := StoreConfig(project.ContextPack{}, cfg)
	if err != nil {
		t.Fatalf("StoreConfig: %v", err)
	}
	parsed, err := ParseConfig(pack.Extra)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if parsed.Credentials == nil || parsed.Credentials.FlyIOAPIToken != "fo1_secret_value" {
		t.Fatalf("expected credentials to round-trip, got %v", parsed.Credentials)
	}
}

func TestIsZeroConfig_WithCredentials(t *testing.T) {
	cfg := Config{Credentials: &Credentials{FlyIOAPIToken: "token"}}
	if isZeroConfig(cfg) {
		t.Fatal("config with credentials should not be zero")
	}
}

func TestIsZeroConfig_EmptyCredentials(t *testing.T) {
	cfg := Config{Credentials: &Credentials{}}
	if !isZeroConfig(cfg) {
		t.Fatal("config with empty credentials should be zero")
	}
}
