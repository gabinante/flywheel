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
