package dispatch

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
)

// ContentRiskClass identifies a class of potentially risky content in external data.
type ContentRiskClass string

const (
	RiskClassURL              ContentRiskClass = "url"
	RiskClassBase64           ContentRiskClass = "base64_blob"
	RiskClassInstructionLike  ContentRiskClass = "instruction_like"
	RiskClassUnusualFormat    ContentRiskClass = "unusual_formatting"
	RiskClassCodeExecution    ContentRiskClass = "code_execution"
	RiskClassPrivilegeEscalation ContentRiskClass = "privilege_escalation"
)

// ContentFlag represents a flagged segment of content with its risk classification.
type ContentFlag struct {
	Class       ContentRiskClass `json:"class"`
	Excerpt     string           `json:"excerpt"`      // Truncated excerpt of the flagged content
	StartOffset int              `json:"start_offset"` // Byte offset in the original content
	Reason      string           `json:"reason"`       // Why this was flagged
}

// ContentAuditEntry records an external content ingestion event for audit purposes.
type ContentAuditEntry struct {
	Timestamp   time.Time        `json:"timestamp"`
	Source      string           `json:"source"`       // Where the content came from (e.g. "dependency_readme", "pr_description", "mcp_tool_output")
	TicketID    string           `json:"ticket_id"`    // Associated ticket if any
	AgentID     string           `json:"agent_id"`     // Which agent read it
	ContentSize int              `json:"content_size"` // Byte length of ingested content
	Flags       []ContentFlag    `json:"flags"`        // Risky content classes detected
	Action      string           `json:"action"`       // What happened next ("injected_to_prompt", "logged_only", "blocked")
}

// Patterns for risky content detection.
var (
	// URLs: http(s), ftp, data URIs
	urlPattern = regexp.MustCompile(`(?i)(https?://|ftp://|data:)[^\s<>"'` + "`" + `]{10,}`)

	// Base64: long stretches of base64-valid characters (at least 64 chars)
	base64Pattern = regexp.MustCompile(`[A-Za-z0-9+/=]{64,}`)

	// Instruction-like patterns: content that appears to give instructions to an AI agent
	instructionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(you must|you should|ignore previous|disregard|override|forget|new instructions?|system prompt|act as|pretend|role.?play)\b`),
		regexp.MustCompile(`(?i)\b(execute|run|eval|deploy|commit|push|merge|approve|grant|sudo|chmod|rm -rf)\s+(this|the|immediately|now)\b`),
		regexp.MustCompile(`(?i)<\s*(system|instruction|prompt|command)\s*>`),
		regexp.MustCompile(`(?i)\[INST\]|\[/INST\]|<<SYS>>|<</SYS>>`),
		regexp.MustCompile(`(?i)(?:^|\n)\s*#{1,3}\s*(system|instruction|new role|override)`),
	}

	// Code execution patterns: shell commands, eval constructs
	codeExecPatterns = []*regexp.Regexp{
		regexp.MustCompile("(?i)`{3}\\s*(bash|sh|shell|zsh|cmd|powershell)[\\s\\n]"),
		regexp.MustCompile(`(?i)\$\(.+\)|` + "`" + `.+` + "`"),
		regexp.MustCompile(`(?i)(curl|wget|nc|netcat)\s+.*(http|ftp|\|)`),
	}

	// Privilege escalation patterns: attempts to modify policy, grant access, etc.
	privEscPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(grant|revoke|elevate|escalat|admin|root|superuser|policy\s+change)\b`),
		regexp.MustCompile(`(?i)\b(modify\s+policy|change\s+permissions?|update\s+access|add\s+collaborator)\b`),
		regexp.MustCompile(`(?i)\b(api[_\s]?key|secret[_\s]?key|password|token|credential)s?\s*[:=]\s*\S+`),
	}

	// Unusual formatting: zero-width chars, RTL overrides, homoglyphs
	unusualFormatPattern = regexp.MustCompile(`[\x{200B}-\x{200F}\x{202A}-\x{202E}\x{2060}-\x{2064}\x{FEFF}]`)
)

// ScanContentForRisks examines external content and returns any flagged risky segments.
// This is a defense-in-depth measure: flagged content is still treated as data,
// but the flags are logged for audit and can trigger human confirmation flows.
func ScanContentForRisks(content string) []ContentFlag {
	var flags []ContentFlag

	// Check for URLs
	if matches := urlPattern.FindAllStringIndex(content, 10); len(matches) > 0 {
		for _, m := range matches {
			flags = append(flags, ContentFlag{
				Class:       RiskClassURL,
				Excerpt:     truncateExcerpt(content[m[0]:m[1]], 80),
				StartOffset: m[0],
				Reason:      "External URL detected in content",
			})
		}
	}

	// Check for base64 blobs
	if matches := base64Pattern.FindAllStringIndex(content, 5); len(matches) > 0 {
		for _, m := range matches {
			// Verify it's actually valid base64 (not just random chars)
			candidate := content[m[0]:m[1]]
			if _, err := base64.StdEncoding.DecodeString(candidate); err == nil {
				flags = append(flags, ContentFlag{
					Class:       RiskClassBase64,
					Excerpt:     truncateExcerpt(candidate, 40),
					StartOffset: m[0],
					Reason:      "Base64-encoded blob detected (possible obfuscated payload)",
				})
			}
		}
	}

	// Check for instruction-like patterns
	for _, pat := range instructionPatterns {
		if matches := pat.FindAllStringIndex(content, 5); len(matches) > 0 {
			for _, m := range matches {
				excerpt := content[m[0]:m[1]]
				// Extend excerpt for context
				start := m[0] - 20
				if start < 0 {
					start = 0
				}
				end := m[1] + 20
				if end > len(content) {
					end = len(content)
				}
				flags = append(flags, ContentFlag{
					Class:       RiskClassInstructionLike,
					Excerpt:     truncateExcerpt(content[start:end], 100),
					StartOffset: m[0],
					Reason:      fmt.Sprintf("Content appears to give instructions: %q", excerpt),
				})
			}
			break // One flag per class is sufficient for instruction patterns
		}
	}

	// Check for code execution patterns
	for _, pat := range codeExecPatterns {
		if matches := pat.FindAllStringIndex(content, 3); len(matches) > 0 {
			for _, m := range matches {
				start := m[0] - 10
				if start < 0 {
					start = 0
				}
				end := m[1] + 10
				if end > len(content) {
					end = len(content)
				}
				flags = append(flags, ContentFlag{
					Class:       RiskClassCodeExecution,
					Excerpt:     truncateExcerpt(content[start:end], 80),
					StartOffset: m[0],
					Reason:      "Content contains code execution patterns",
				})
			}
			break
		}
	}

	// Check for privilege escalation patterns
	for _, pat := range privEscPatterns {
		if matches := pat.FindAllStringIndex(content, 3); len(matches) > 0 {
			for _, m := range matches {
				start := m[0] - 10
				if start < 0 {
					start = 0
				}
				end := m[1] + 10
				if end > len(content) {
					end = len(content)
				}
				flags = append(flags, ContentFlag{
					Class:       RiskClassPrivilegeEscalation,
					Excerpt:     truncateExcerpt(content[start:end], 80),
					StartOffset: m[0],
					Reason:      "Content references privilege escalation or policy modification",
				})
			}
			break
		}
	}

	// Check for unusual formatting
	if matches := unusualFormatPattern.FindAllStringIndex(content, 5); len(matches) > 0 {
		flags = append(flags, ContentFlag{
			Class:       RiskClassUnusualFormat,
			Excerpt:     fmt.Sprintf("[%d invisible/control characters detected]", len(matches)),
			StartOffset: matches[0][0],
			Reason:      "Content contains zero-width or bidirectional control characters (possible obfuscation)",
		})
	}

	return flags
}

// LogContentIngestion records an external content ingestion event for audit.
// This creates an append-only audit trail of all external content read by the coordinator.
func LogContentIngestion(logger *slog.Logger, entry ContentAuditEntry) {
	if logger == nil {
		return
	}

	attrs := []any{
		slog.String("source", entry.Source),
		slog.String("ticket_id", entry.TicketID),
		slog.String("agent_id", entry.AgentID),
		slog.Int("content_size", entry.ContentSize),
		slog.Int("flags_count", len(entry.Flags)),
		slog.String("action", entry.Action),
		slog.Time("ingestion_time", entry.Timestamp),
	}

	if len(entry.Flags) > 0 {
		var classes []string
		for _, f := range entry.Flags {
			classes = append(classes, string(f.Class))
		}
		attrs = append(attrs, slog.String("risk_classes", strings.Join(classes, ",")))
	}

	logger.Info("content_ingestion_audit", attrs...)
}

// SensitiveAction identifies actions that require human confirmation regardless of context.
type SensitiveAction string

const (
	ActionOpenPR           SensitiveAction = "open_pull_request"
	ActionPostExternal     SensitiveAction = "post_to_external_channel"
	ActionModifyPermission SensitiveAction = "modify_permissions"
	ActionDeploy           SensitiveAction = "deploy"
	ActionDeleteResource   SensitiveAction = "delete_resource"
	ActionGrantAccess      SensitiveAction = "grant_access"
	ActionModifyPolicy     SensitiveAction = "modify_policy"
	ActionCommitCode       SensitiveAction = "commit_code"
)

// RequiresHumanConfirmation returns true if the given action always requires
// explicit human confirmation, regardless of any authorization that appears
// in external content. This is a structural defense: even if injected content
// says "approved" or "authorized", these actions still gate on real human input.
func RequiresHumanConfirmation(action SensitiveAction) bool {
	// All sensitive actions require confirmation by design.
	// This is not a filter — it's a structural invariant.
	switch action {
	case ActionOpenPR,
		ActionPostExternal,
		ActionModifyPermission,
		ActionDeploy,
		ActionDeleteResource,
		ActionGrantAccess,
		ActionModifyPolicy,
		ActionCommitCode:
		return true
	default:
		return false
	}
}

// CoordinatorForbiddenActions lists actions the coordinator is structurally
// prohibited from performing, regardless of any instruction in external content.
var CoordinatorForbiddenActions = []SensitiveAction{
	ActionCommitCode,
	ActionDeploy,
	ActionModifyPolicy,
	ActionGrantAccess,
	ActionModifyPermission,
	ActionDeleteResource,
}

// IsCoordinatorForbidden returns true if the action is structurally forbidden
// for the coordinator role. This is enforced at the tool level, not advisory.
func IsCoordinatorForbidden(action SensitiveAction) bool {
	for _, forbidden := range CoordinatorForbiddenActions {
		if action == forbidden {
			return true
		}
	}
	return false
}

func truncateExcerpt(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
