package environment

import "time"

// Infrastructure represents the infrastructure tier dimension.
type Infrastructure string

const (
	InfraDev     Infrastructure = "dev"
	InfraStaging Infrastructure = "staging"
	InfraProd    Infrastructure = "prod"
)

// AllInfrastructures returns all valid infrastructure values.
func AllInfrastructures() []Infrastructure {
	return []Infrastructure{InfraDev, InfraStaging, InfraProd}
}

// IsValidInfrastructure checks whether the value is a valid infrastructure tier.
func IsValidInfrastructure(i Infrastructure) bool {
	for _, v := range AllInfrastructures() {
		if i == v {
			return true
		}
	}
	return false
}

// DataTenancy represents the data tenancy dimension.
type DataTenancy string

const (
	DataSynthetic  DataTenancy = "synthetic"
	DataAnonymized DataTenancy = "anonymized"
	DataReal       DataTenancy = "real"
)

// AllDataTenancies returns all valid data tenancy values.
func AllDataTenancies() []DataTenancy {
	return []DataTenancy{DataSynthetic, DataAnonymized, DataReal}
}

// IsValidDataTenancy checks whether the value is a valid data tenancy.
func IsValidDataTenancy(d DataTenancy) bool {
	for _, v := range AllDataTenancies() {
		if d == v {
			return true
		}
	}
	return false
}

// IntegrationMode represents the integration mode dimension.
type IntegrationMode string

const (
	IntegrationSandbox IntegrationMode = "sandbox"
	IntegrationTest    IntegrationMode = "test"
	IntegrationLive    IntegrationMode = "live"
)

// AllIntegrationModes returns all valid integration mode values.
func AllIntegrationModes() []IntegrationMode {
	return []IntegrationMode{IntegrationSandbox, IntegrationTest, IntegrationLive}
}

// IsValidIntegrationMode checks whether the value is a valid integration mode.
func IsValidIntegrationMode(m IntegrationMode) bool {
	for _, v := range AllIntegrationModes() {
		if m == v {
			return true
		}
	}
	return false
}

// Environment is the compound tuple representing a deployment environment.
// Per spec v0.2 section 4.1, it has three dimensions:
// infrastructure (dev/staging/prod), data tenancy (synthetic/anonymized/real),
// and integration mode (sandbox/test/live).
type Environment struct {
	ID              string          `json:"id"`
	ProjectID       string          `json:"project_id"`
	Name            string          `json:"name"`
	Slug            string          `json:"slug"`
	Infrastructure  Infrastructure  `json:"infrastructure"`
	DataTenancy     DataTenancy     `json:"data_tenancy"`
	IntegrationMode IntegrationMode `json:"integration_mode"`
	IsDefault       bool            `json:"is_default"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// QualifiedState returns a ticket state qualified by this environment's slug.
// Example: "executing-dev" or "validated-staging".
func (e *Environment) QualifiedState(state string) string {
	return state + "-" + e.Slug
}

// Validate checks that all three dimensions are valid.
func (e *Environment) Validate() error {
	if e.Name == "" {
		return ErrNameRequired
	}
	if e.Slug == "" {
		return ErrSlugRequired
	}
	if !IsValidInfrastructure(e.Infrastructure) {
		return ErrInvalidInfrastructure
	}
	if !IsValidDataTenancy(e.DataTenancy) {
		return ErrInvalidDataTenancy
	}
	if !IsValidIntegrationMode(e.IntegrationMode) {
		return ErrInvalidIntegrationMode
	}
	return nil
}
