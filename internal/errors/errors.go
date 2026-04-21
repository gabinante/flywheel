package errors

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/gabinante/flywheel/internal/cost"
	"github.com/gabinante/flywheel/internal/pillar"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/queue"
	"github.com/gabinante/flywheel/internal/ticket"
)

// Code is a stable identifier for error types so agents and clients can branch on it.
type Code string

const (
	CodeLeaseExpired    Code = "lease_expired"
	CodeUnauthorized    Code = "unauthorized"
	CodeNotFound        Code = "not_found"
	CodeConflict        Code = "conflict"
	CodeInvalidInput    Code = "invalid_input"
	CodeForbidden       Code = "forbidden"
	CodeInternal        Code = "internal"
	CodeProjectClosed   Code = "project_closed"
	CodeNotImplemented  Code = "not_implemented"
	CodeBudgetExceeded  Code = "budget_exceeded"  // spend exceeds limit (hard stop enabled)
	CodeRateLimited     Code = "rate_limited"     // provider rate-limited, backing off
)

// StructuredError is returned in REST JSON and in MCP tool error messages (as JSON string).
// Agents can parse the error string as JSON to get code and retriable for retry logic.
type StructuredError struct {
	Message   string `json:"error"` // JSON key "error" for API compatibility
	Code      Code   `json:"code"`
	Retriable bool   `json:"retriable"`
}

// Error implements the error interface so *StructuredError can be returned as error.
func (e *StructuredError) Error() string {
	return e.Message
}

// New returns a StructuredError. Use code constants and set retriable when the client could retry (e.g. transient failure).
func New(code Code, message string, retriable bool) *StructuredError {
	return &StructuredError{Message: message, Code: code, Retriable: retriable}
}


// JSON returns the JSON encoding for use in REST body or MCP tool error message.
func (e *StructuredError) JSON() string {
	b, _ := json.Marshal(e)
	return string(b)
}

// HTTPStatus returns the HTTP status code for this error type.
func (e *StructuredError) HTTPStatus() int {
	switch e.Code {
	case CodeUnauthorized:
		return 401
	case CodeForbidden:
		return 403
	case CodeNotFound:
		return 404
	case CodeConflict:
		return 409
	case CodeInvalidInput, CodeProjectClosed, CodeBudgetExceeded:
		return 400
	case CodeRateLimited:
		return 429
	case CodeNotImplemented:
		return 501
	case CodeLeaseExpired, CodeInternal:
	default:
	}
	return 500
}

// MapError maps known service/domain errors to a structured error (for REST and MCP).
func MapError(err error) *StructuredError {
	if err == nil {
		return nil
	}
	var accFail *ticket.AcceptanceTestFailure
	if errors.As(err, &accFail) {
		msg := accFail.Error()
		if accFail.Stdout != "" {
			msg += "\n--- output ---\n" + accFail.Stdout
		}
		return New(CodeInvalidInput, msg, true)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return New(CodeNotFound, "not found", false)
	}
	if errors.Is(err, ticket.ErrVersionConflict) {
		return New(CodeConflict, err.Error(), false)
	}
	if errors.Is(err, queue.ErrNoTicketAvailable) {
		return New(CodeNotFound, err.Error(), true)
	}
	if errors.Is(err, project.ErrProjectNotFound) {
		return New(CodeNotFound, "project not found", false)
	}
	if errors.Is(err, project.ErrInvalidStatus) {
		return New(CodeInvalidInput, err.Error(), false)
	}
	if errors.Is(err, ticket.ErrAcceptanceCriteriaRequired) {
		return New(CodeInvalidInput, err.Error(), false)
	}
	if errors.Is(err, cost.ErrBudgetExceeded) {
		return New(CodeBudgetExceeded, err.Error(), false)
	}
	if errors.Is(err, cost.ErrRateLimited) {
		return New(CodeRateLimited, err.Error(), true)
	}
	if errors.Is(err, cost.ErrInvalidBudget) {
		return New(CodeInvalidInput, err.Error(), false)
	}
	// Pillar layer errors (Layer 15).
	if errors.Is(err, pillar.ErrEntryNotFound) || errors.Is(err, pillar.ErrClaimNotFound) || errors.Is(err, pillar.ErrEvaluationNotFound) {
		return New(CodeNotFound, err.Error(), false)
	}
	if errors.Is(err, pillar.ErrInvalidPillarType) || errors.Is(err, pillar.ErrInvalidClaimStatus) {
		return New(CodeInvalidInput, err.Error(), false)
	}
	if errors.Is(err, pillar.ErrVersionConflict) {
		return New(CodeConflict, err.Error(), true)
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "project:"):
		return New(CodeNotFound, msg, false)
	case strings.Contains(msg, "dependency not done"), strings.Contains(msg, "ticket already claimed"),
		strings.Contains(msg, "actor is not the leaseholder"):
		return New(CodeConflict, msg, false)
	case strings.Contains(msg, "outputs required"), strings.Contains(msg, "reason or question required"),
		strings.Contains(msg, "no valid transition"):
		return New(CodeInvalidInput, msg, false)
	case strings.Contains(msg, "invalid lease token"):
		return New(CodeLeaseExpired, "lease expired or invalid token", true)
	default:
		return New(CodeInternal, msg, false)
	}
}
