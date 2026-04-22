package risk

import "strings"

// ShellClassifier classifies shell operations by reading the side-effect manifest,
// NOT by analyzing the script itself. The manifest declares what the script will do.
//
// Classification rules:
//   - Read-only operations (list, status, check, version) → safe
//   - File creation, directory creation → low
//   - File modification, service restart → reversible
//   - Package install/remove, user management → high
//   - Disk format, rm -rf, system-level changes → destructive
//
// The classifier inspects:
//   - Operation.Action: the manifest-declared side effect type
//   - Operation.Details["side_effects"]: list of declared side effects
//   - Operation.Details["modifies_system"]: bool flag from manifest
//   - Operation.Details["requires_sudo"]: bool flag from manifest
type ShellClassifier struct{}

// destructive shell side effects.
var shellDestructiveEffects = []string{
	"rm -rf",
	"format disk",
	"fdisk",
	"mkfs",
	"dd if=",
	"wipefs",
	"destroy",
	"drop database",
	"shred",
	"delete_all",
	"purge",
}

// high-risk shell side effects.
var shellHighEffects = []string{
	"install package",
	"remove package",
	"uninstall",
	"useradd",
	"userdel",
	"groupadd",
	"groupdel",
	"chmod",
	"chown",
	"systemctl enable",
	"systemctl disable",
	"iptables",
	"firewall",
	"crontab",
	"mount",
	"umount",
}

// reversible shell side effects.
var shellReversibleEffects = []string{
	"modify file",
	"edit file",
	"write file",
	"update config",
	"restart service",
	"reload service",
	"systemctl restart",
	"systemctl reload",
	"stop service",
	"start service",
	"mkdir",
	"rename",
	"move",
	"copy",
	"sed",
	"patch",
}

// low-risk shell side effects.
var shellLowEffects = []string{
	"create file",
	"create directory",
	"touch",
	"tee",
	"append",
	"log",
	"download",
	"fetch",
	"curl",
	"wget",
}

// safe (read-only) shell side effects.
var shellSafeEffects = []string{
	"read",
	"list",
	"ls",
	"cat",
	"head",
	"tail",
	"grep",
	"find",
	"stat",
	"status",
	"check",
	"version",
	"echo",
	"print",
	"whoami",
	"hostname",
	"date",
	"uptime",
	"df",
	"du",
	"ps",
	"top",
	"env",
	"pwd",
	"which",
	"test",
}

// Classify classifies a shell operation by its declared side-effect manifest.
func (c *ShellClassifier) Classify(op Operation) (RiskLevel, []RuleCitation) {
	// If the manifest declares sudo requirement, bump minimum to high.
	requiresSudo := false
	if sudo, ok := op.Details["requires_sudo"].(bool); ok && sudo {
		requiresSudo = true
	}

	// If the manifest declares system modification, bump minimum to reversible.
	modifiesSystem := false
	if mod, ok := op.Details["modifies_system"].(bool); ok && mod {
		modifiesSystem = true
	}

	// Collect all declared effects to classify.
	effects := collectEffects(op)

	// If no effects declared at all, unknown → maximum risk.
	if len(effects) == 0 {
		return RiskDestructive, []RuleCitation{{
			Rule:        "shell.no_manifest",
			Backend:     BackendShell,
			Level:       RiskDestructive,
			Description: "Shell operation has no side-effect manifest — defaults to maximum risk",
		}}
	}

	// Classify each effect and take the highest.
	maxLevel := RiskSafe
	var citations []RuleCitation

	for _, effect := range effects {
		level, citation := classifyShellEffect(effect)
		if level > maxLevel {
			maxLevel = level
		}
		citations = append(citations, citation)
	}

	// Apply sudo bump: minimum high.
	if requiresSudo && maxLevel < RiskHigh {
		maxLevel = RiskHigh
		citations = append(citations, RuleCitation{
			Rule:        "shell.requires_sudo",
			Backend:     BackendShell,
			Level:       RiskHigh,
			Description: "Shell operation requires sudo/root — minimum risk elevated to high",
		})
	}

	// Apply system modification bump: minimum reversible.
	if modifiesSystem && maxLevel < RiskReversible {
		maxLevel = RiskReversible
		citations = append(citations, RuleCitation{
			Rule:        "shell.modifies_system",
			Backend:     BackendShell,
			Level:       RiskReversible,
			Description: "Shell operation modifies system state — minimum risk elevated to reversible",
		})
	}

	return maxLevel, citations
}

// collectEffects gathers side effects from the operation's action and details.
func collectEffects(op Operation) []string {
	var effects []string

	// The action itself is a declared effect.
	if op.Action != "" {
		effects = append(effects, strings.ToLower(strings.TrimSpace(op.Action)))
	}

	// Additional effects from the manifest list.
	if sideEffects, ok := op.Details["side_effects"]; ok {
		switch se := sideEffects.(type) {
		case []any:
			for _, e := range se {
				if s, ok := e.(string); ok {
					effects = append(effects, strings.ToLower(strings.TrimSpace(s)))
				}
			}
		case []string:
			for _, s := range se {
				effects = append(effects, strings.ToLower(strings.TrimSpace(s)))
			}
		}
	}

	return effects
}

// classifyShellEffect classifies a single declared side effect.
func classifyShellEffect(effect string) (RiskLevel, RuleCitation) {
	for _, pattern := range shellDestructiveEffects {
		if strings.Contains(effect, pattern) {
			return RiskDestructive, RuleCitation{
				Rule:        "shell.destructive_effect",
				Backend:     BackendShell,
				Level:       RiskDestructive,
				Description: "Shell declares destructive side effect: '" + effect + "'",
			}
		}
	}

	for _, pattern := range shellHighEffects {
		if strings.Contains(effect, pattern) {
			return RiskHigh, RuleCitation{
				Rule:        "shell.high_effect",
				Backend:     BackendShell,
				Level:       RiskHigh,
				Description: "Shell declares high-risk side effect: '" + effect + "'",
			}
		}
	}

	for _, pattern := range shellReversibleEffects {
		if strings.Contains(effect, pattern) {
			return RiskReversible, RuleCitation{
				Rule:        "shell.reversible_effect",
				Backend:     BackendShell,
				Level:       RiskReversible,
				Description: "Shell declares reversible side effect: '" + effect + "'",
			}
		}
	}

	for _, pattern := range shellLowEffects {
		if strings.Contains(effect, pattern) {
			return RiskLow, RuleCitation{
				Rule:        "shell.low_effect",
				Backend:     BackendShell,
				Level:       RiskLow,
				Description: "Shell declares low-risk side effect: '" + effect + "'",
			}
		}
	}

	for _, pattern := range shellSafeEffects {
		if strings.Contains(effect, pattern) {
			return RiskSafe, RuleCitation{
				Rule:        "shell.safe_effect",
				Backend:     BackendShell,
				Level:       RiskSafe,
				Description: "Shell declares safe (read-only) side effect: '" + effect + "'",
			}
		}
	}

	// Unknown shell effect → maximum risk.
	return RiskDestructive, RuleCitation{
		Rule:        "shell.unknown_effect",
		Backend:     BackendShell,
		Level:       RiskDestructive,
		Description: "Shell declares unknown side effect: '" + effect + "' — defaults to maximum risk",
	}
}
