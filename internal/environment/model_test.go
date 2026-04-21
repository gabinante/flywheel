package environment

import (
	"testing"
)

func TestEnvironment_Validate(t *testing.T) {
	tests := []struct {
		name    string
		env     Environment
		wantErr error
	}{
		{
			name: "valid environment",
			env: Environment{
				Name:            "Development",
				Slug:            "dev",
				Infrastructure:  InfraDev,
				DataTenancy:     DataSynthetic,
				IntegrationMode: IntegrationSandbox,
			},
			wantErr: nil,
		},
		{
			name: "valid staging environment",
			env: Environment{
				Name:            "Staging",
				Slug:            "staging",
				Infrastructure:  InfraStaging,
				DataTenancy:     DataAnonymized,
				IntegrationMode: IntegrationTest,
			},
			wantErr: nil,
		},
		{
			name: "valid production environment",
			env: Environment{
				Name:            "Production",
				Slug:            "prod",
				Infrastructure:  InfraProd,
				DataTenancy:     DataReal,
				IntegrationMode: IntegrationLive,
			},
			wantErr: nil,
		},
		{
			name: "missing name",
			env: Environment{
				Slug:            "dev",
				Infrastructure:  InfraDev,
				DataTenancy:     DataSynthetic,
				IntegrationMode: IntegrationSandbox,
			},
			wantErr: ErrNameRequired,
		},
		{
			name: "missing slug",
			env: Environment{
				Name:            "Dev",
				Infrastructure:  InfraDev,
				DataTenancy:     DataSynthetic,
				IntegrationMode: IntegrationSandbox,
			},
			wantErr: ErrSlugRequired,
		},
		{
			name: "invalid infrastructure",
			env: Environment{
				Name:            "Bad",
				Slug:            "bad",
				Infrastructure:  "invalid",
				DataTenancy:     DataSynthetic,
				IntegrationMode: IntegrationSandbox,
			},
			wantErr: ErrInvalidInfrastructure,
		},
		{
			name: "invalid data tenancy",
			env: Environment{
				Name:            "Bad",
				Slug:            "bad",
				Infrastructure:  InfraDev,
				DataTenancy:     "invalid",
				IntegrationMode: IntegrationSandbox,
			},
			wantErr: ErrInvalidDataTenancy,
		},
		{
			name: "invalid integration mode",
			env: Environment{
				Name:            "Bad",
				Slug:            "bad",
				Infrastructure:  InfraDev,
				DataTenancy:     DataSynthetic,
				IntegrationMode: "invalid",
			},
			wantErr: ErrInvalidIntegrationMode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.env.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestQualifiedState(t *testing.T) {
	env := &Environment{Slug: "dev"}
	got := env.QualifiedState("executing")
	if got != "executing-dev" {
		t.Errorf("QualifiedState() = %q, want %q", got, "executing-dev")
	}

	env2 := &Environment{Slug: "staging"}
	got2 := env2.QualifiedState("validated")
	if got2 != "validated-staging" {
		t.Errorf("QualifiedState() = %q, want %q", got2, "validated-staging")
	}
}

func TestIsValidInfrastructure(t *testing.T) {
	tests := []struct {
		input Infrastructure
		want  bool
	}{
		{InfraDev, true},
		{InfraStaging, true},
		{InfraProd, true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsValidInfrastructure(tt.input); got != tt.want {
			t.Errorf("IsValidInfrastructure(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsValidDataTenancy(t *testing.T) {
	tests := []struct {
		input DataTenancy
		want  bool
	}{
		{DataSynthetic, true},
		{DataAnonymized, true},
		{DataReal, true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsValidDataTenancy(tt.input); got != tt.want {
			t.Errorf("IsValidDataTenancy(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsValidIntegrationMode(t *testing.T) {
	tests := []struct {
		input IntegrationMode
		want  bool
	}{
		{IntegrationSandbox, true},
		{IntegrationTest, true},
		{IntegrationLive, true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsValidIntegrationMode(tt.input); got != tt.want {
			t.Errorf("IsValidIntegrationMode(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
