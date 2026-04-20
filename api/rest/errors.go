package rest

import (
	"net/http"

	apierrors "github.com/gabinante/flywheel/internal/errors"
)

// WriteStructuredError writes a structured error as JSON and sets the status code.
func WriteStructuredError(w http.ResponseWriter, e *apierrors.StructuredError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.HTTPStatus())
	_, _ = w.Write([]byte(e.JSON()))
}

// TicketError maps ticket/queue errors to a structured error for REST.
func TicketError(err error) *apierrors.StructuredError {
	return apierrors.MapError(err)
}
