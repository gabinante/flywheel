package mirror

import "testing"

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name    string
		extra   map[string]string
		wantNil bool
		wantErr bool
	}{
		{
			name:    "empty extra - no config",
			extra:   map[string]string{},
			wantNil: true,
		},
		{
			name:    "no mirror_config key",
			extra:   map[string]string{"other": "value"},
			wantNil: true,
		},
		{
			name:    "empty mirror_config value",
			extra:   map[string]string{"mirror_config": ""},
			wantNil: true,
		},
		{
			name:    "invalid JSON",
			extra:   map[string]string{"mirror_config": "not json"},
			wantErr: true,
		},
		{
			name: "valid config",
			extra: map[string]string{
				"mirror_config": `{"enabled":true,"provider":"linear","state_mapping":{"executing":"In Progress"},"linear":{"team_id":"t1","api_key_secret":"key"}}`,
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseConfig(tt.extra)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil && cfg != nil {
				t.Error("expected nil config, got non-nil")
			}
			if !tt.wantNil && cfg == nil {
				t.Error("expected non-nil config, got nil")
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "disabled - always valid",
			cfg:  Config{Enabled: false},
		},
		{
			name:    "enabled no provider",
			cfg:     Config{Enabled: true},
			wantErr: true,
		},
		{
			name:    "enabled no state mapping",
			cfg:     Config{Enabled: true, Provider: "linear"},
			wantErr: true,
		},
		{
			name:    "linear missing config",
			cfg:     Config{Enabled: true, Provider: "linear", StateMapping: map[string]string{"x": "y"}},
			wantErr: true,
		},
		{
			name: "linear missing team_id",
			cfg: Config{
				Enabled:      true,
				Provider:     "linear",
				StateMapping: map[string]string{"x": "y"},
				Linear:       &LinearConfig{APIKeySecret: "k"},
			},
			wantErr: true,
		},
		{
			name: "linear valid",
			cfg: Config{
				Enabled:      true,
				Provider:     "linear",
				StateMapping: map[string]string{"executing": "In Progress"},
				Linear:       &LinearConfig{TeamID: "t1", APIKeySecret: "k"},
			},
			wantErr: false,
		},
		{
			name: "jira missing config",
			cfg: Config{
				Enabled:      true,
				Provider:     "jira",
				StateMapping: map[string]string{"x": "y"},
			},
			wantErr: true,
		},
		{
			name: "jira valid",
			cfg: Config{
				Enabled:      true,
				Provider:     "jira",
				StateMapping: map[string]string{"executing": "In Progress"},
				Jira:         &JiraConfig{BaseURL: "https://x.atlassian.net", ProjectKey: "ENG", APITokenSecret: "t"},
			},
			wantErr: false,
		},
		{
			name: "unsupported provider",
			cfg: Config{
				Enabled:      true,
				Provider:     "notion",
				StateMapping: map[string]string{"x": "y"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestConfigMapState(t *testing.T) {
	cfg := &Config{
		StateMapping: map[string]string{
			"executing": "In Progress",
			"observing": "In Review",
			"closed":    "Done",
		},
	}

	tests := []struct {
		input string
		want  string
	}{
		{"executing", "In Progress"},
		{"observing", "In Review"},
		{"closed", "Done"},
		{"draft", ""},       // not mapped
		{"specced", ""},     // not mapped
		{"nonexistent", ""}, // not mapped
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cfg.MapState(tt.input)
			if got != tt.want {
				t.Errorf("MapState(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConfigTicketURL(t *testing.T) {
	cfg := &Config{ContextBaseURL: "https://warrant.example.com"}
	url := cfg.TicketURL("proj-42")
	expected := "https://warrant.example.com/#/tickets/proj-42"
	if url != expected {
		t.Errorf("TicketURL = %q, want %q", url, expected)
	}

	// Empty base URL.
	cfg2 := &Config{}
	if url := cfg2.TicketURL("proj-1"); url != "" {
		t.Errorf("expected empty URL, got %q", url)
	}
}
