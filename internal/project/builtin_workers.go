package project

// Built-in definitions are selectable workers, but are not members of the legacy
// implicit pool of custom workers. Adding them must not change existing routing.
const (
	BuiltinDispatch     = "builtin-dispatch"
	BuiltinOrchestrator = "builtin-orchestrator"
	BuiltinReview       = "builtin-review"
	BuiltinFeedback     = "builtin-feedback"
)

func IsBuiltinWorker(id string) bool {
	switch id {
	case BuiltinDispatch, BuiltinOrchestrator, BuiltinReview, BuiltinFeedback:
		return true
	}
	return false
}
