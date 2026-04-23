package environment

import "errors"

var (
	ErrNameRequired           = errors.New("environment name is required")
	ErrSlugRequired           = errors.New("environment slug is required")
	ErrInvalidInfrastructure  = errors.New("invalid infrastructure: must be dev, staging, or prod")
	ErrInvalidDataTenancy     = errors.New("invalid data tenancy: must be synthetic, anonymized, or real")
	ErrInvalidIntegrationMode = errors.New("invalid integration mode: must be sandbox, test, or live")
	ErrNotFound               = errors.New("environment not found")
	ErrMinimumEnvironments    = errors.New("project must have at least two environments")
	ErrDuplicateSlug          = errors.New("environment slug already exists in project")
	ErrCannotDeleteDefault    = errors.New("cannot delete the default environment")
	ErrLastEnvironment        = errors.New("cannot delete: project must retain at least two environments")
)
