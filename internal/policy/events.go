package policy

// Event type constants for the policy calibration feedback loop.
const (
	EventPolicyCreated         = "policy.created"
	EventPolicyUpdated         = "policy.updated"
	EventPolicyDisabled        = "policy.disabled"
	EventPolicyDecisionMade    = "policy.decision_made"
	EventPolicyOutcomeRecorded = "policy.outcome_recorded"
	EventPolicyProposalCreated = "policy.proposal_created"
	EventPolicyCalibrationRun  = "policy.calibration_run"
)
