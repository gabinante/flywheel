package plan

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SandboxPolicy generates container runtime enforcement configuration
// from a SideEffectManifest. The policy is used by DockerWorker to
// restrict what the shell script can actually do at runtime.
// If the script attempts operations not in the manifest, execution fails.
type SandboxPolicy struct {
	// MountPaths are the paths to mount read-write in the container.
	// Derived from file_ops with action "write" or "delete".
	MountPaths []MountSpec `json:"mount_paths"`

	// ReadOnlyMounts are paths mounted read-only (from file_ops with action "read").
	ReadOnlyMounts []MountSpec `json:"read_only_mounts"`

	// AllowedEndpoints are network endpoints the container may connect to.
	AllowedEndpoints []string `json:"allowed_endpoints"`

	// AllowedProcesses are binaries the container may execute.
	AllowedProcesses []string `json:"allowed_processes"`

	// SecretNames are credential names injected as environment variables.
	SecretNames []string `json:"secret_names"`

	// TimeoutSeconds is the max runtime enforced by container runtime.
	TimeoutSeconds int `json:"timeout_seconds"`

	// DiskQuotaBytes is the max writable bytes (enforced via tmpfs or quota).
	DiskQuotaBytes int64 `json:"disk_quota_bytes,omitempty"`

	// NetworkCallLimit is max outbound connections (enforced via iptables counting).
	NetworkCallLimit int `json:"network_call_limit,omitempty"`

	// SeccompProfile restricts system calls based on manifest declarations.
	SeccompProfile string `json:"seccomp_profile,omitempty"`
}

// MountSpec describes a container mount point.
type MountSpec struct {
	HostPath      string `json:"host_path"`
	ContainerPath string `json:"container_path"`
	ReadOnly      bool   `json:"read_only"`
}

// DefaultTimeoutSeconds is the default max runtime if not specified in manifest.
const DefaultTimeoutSeconds = 300

// DefaultDiskQuotaBytes is 100MB default disk quota.
const DefaultDiskQuotaBytes int64 = 100 * 1024 * 1024

// DefaultNetworkCallLimit is 50 outbound connections by default.
const DefaultNetworkCallLimit = 50

// GenerateSandboxPolicy derives a SandboxPolicy from the manifest.
// This is called at plan submission time; the policy is stored alongside the plan
// and used by the container runtime at execution time.
func GenerateSandboxPolicy(manifest *SideEffectManifest) *SandboxPolicy {
	policy := &SandboxPolicy{}

	// Derive mount specifications from file ops.
	for _, op := range manifest.FileOps {
		mount := MountSpec{
			HostPath:      op.Path,
			ContainerPath: op.Path,
			ReadOnly:      op.Action == "read",
		}
		if op.Action == "read" {
			policy.ReadOnlyMounts = append(policy.ReadOnlyMounts, mount)
		} else {
			policy.MountPaths = append(policy.MountPaths, mount)
		}
	}

	// Derive network allowlist from network ops.
	seen := make(map[string]bool)
	for _, op := range manifest.NetworkOps {
		if !seen[op.Endpoint] {
			policy.AllowedEndpoints = append(policy.AllowedEndpoints, op.Endpoint)
			seen[op.Endpoint] = true
		}
	}

	// Derive allowed processes from process ops.
	for _, op := range manifest.ProcessOps {
		policy.AllowedProcesses = append(policy.AllowedProcesses, op.Binary)
	}

	// Derive secret injection list from credential ops.
	for _, op := range manifest.CredentialOps {
		policy.SecretNames = append(policy.SecretNames, op.Name)
	}

	// Apply resource limits with defaults.
	policy.TimeoutSeconds = manifest.ResourceLimits.MaxRuntimeSeconds
	if policy.TimeoutSeconds <= 0 {
		policy.TimeoutSeconds = DefaultTimeoutSeconds
	}

	policy.DiskQuotaBytes = manifest.ResourceLimits.MaxDiskWriteBytes
	if policy.DiskQuotaBytes <= 0 {
		policy.DiskQuotaBytes = DefaultDiskQuotaBytes
	}

	policy.NetworkCallLimit = manifest.ResourceLimits.MaxNetworkCalls
	if policy.NetworkCallLimit <= 0 {
		policy.NetworkCallLimit = DefaultNetworkCallLimit
	}

	// Select seccomp profile based on what's declared.
	policy.SeccompProfile = selectSeccompProfile(manifest)

	return policy
}

// selectSeccompProfile chooses the appropriate seccomp profile.
// More restrictive manifests get tighter profiles.
func selectSeccompProfile(manifest *SideEffectManifest) string {
	hasNetwork := len(manifest.NetworkOps) > 0
	hasProcessSpawn := len(manifest.ProcessOps) > 0

	switch {
	case !hasNetwork && !hasProcessSpawn:
		return "flywheel-strict" // No network, no spawning
	case hasNetwork && !hasProcessSpawn:
		return "flywheel-network" // Network allowed, no arbitrary spawning
	default:
		return "flywheel-default" // Full access within declared bounds
	}
}

// SandboxViolation represents an attempt to perform an undeclared operation.
type SandboxViolation struct {
	Operation   string `json:"operation"`   // "file_write", "file_read", "network", "process", "credential"
	Target      string `json:"target"`      // What was attempted (path, endpoint, binary)
	Manifest    string `json:"manifest"`    // What the manifest declares for context
	Description string `json:"description"` // Human-readable error
}

func (v *SandboxViolation) Error() string {
	return fmt.Sprintf("sandbox violation [%s]: %s (target: %s)", v.Operation, v.Description, v.Target)
}

// CheckFileAccess verifies that a file path is allowed by the manifest.
// Returns a SandboxViolation if the access is not declared.
func CheckFileAccess(manifest *SideEffectManifest, path string, action string) *SandboxViolation {
	for _, op := range manifest.FileOps {
		if op.Action != action {
			continue
		}
		matched, _ := filepath.Match(op.Path, path)
		if matched {
			return nil
		}
		// Also check if the path is under a directory pattern.
		if matchGlobPath(op.Path, path) {
			return nil
		}
	}
	return &SandboxViolation{
		Operation:   "file_" + action,
		Target:      path,
		Manifest:    formatFileOps(manifest.FileOps, action),
		Description: fmt.Sprintf("undeclared file %s: path %q is not in manifest", action, path),
	}
}

// CheckNetworkAccess verifies that a network endpoint is allowed by the manifest.
func CheckNetworkAccess(manifest *SideEffectManifest, endpoint string, method string) *SandboxViolation {
	for _, op := range manifest.NetworkOps {
		if matchEndpoint(op.Endpoint, endpoint) && (op.Method == method || op.Method == "TCP" || op.Method == "UDP") {
			return nil
		}
	}
	return &SandboxViolation{
		Operation:   "network",
		Target:      fmt.Sprintf("%s %s", method, endpoint),
		Manifest:    formatNetworkOps(manifest.NetworkOps),
		Description: fmt.Sprintf("undeclared network access: %s %s is not in manifest", method, endpoint),
	}
}

// CheckProcessSpawn verifies that a process spawn is allowed by the manifest.
func CheckProcessSpawn(manifest *SideEffectManifest, binary string, args []string) *SandboxViolation {
	for _, op := range manifest.ProcessOps {
		if matchBinary(op.Binary, binary) {
			// If args declared, check them too.
			if len(op.Args) > 0 && !matchArgs(op.Args, args) {
				continue
			}
			return nil
		}
	}
	return &SandboxViolation{
		Operation:   "process",
		Target:      binary + " " + strings.Join(args, " "),
		Manifest:    formatProcessOps(manifest.ProcessOps),
		Description: fmt.Sprintf("undeclared process spawn: %q is not in manifest", binary),
	}
}

// CheckCredentialAccess verifies that a credential access is allowed by the manifest.
func CheckCredentialAccess(manifest *SideEffectManifest, name string) *SandboxViolation {
	for _, op := range manifest.CredentialOps {
		if op.Name == name {
			return nil
		}
	}
	return &SandboxViolation{
		Operation:   "credential",
		Target:      name,
		Manifest:    formatCredentialOps(manifest.CredentialOps),
		Description: fmt.Sprintf("undeclared credential access: %q is not in manifest", name),
	}
}

// --- Glob matching helpers ---

// matchGlobPath checks if a path matches a glob pattern, handling ** for directories.
func matchGlobPath(pattern, path string) bool {
	// Handle ** glob (matches any number of directories)
	if strings.Contains(pattern, "**") {
		// Convert ** to a prefix match: /app/dist/** matches /app/dist/anything
		prefix := strings.TrimSuffix(pattern, "**")
		prefix = strings.TrimSuffix(prefix, "/")
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	// Handle trailing glob: /tmp/*.log matches /tmp/foo.log
	matched, _ := filepath.Match(pattern, path)
	if matched {
		return true
	}

	// Handle directory containment: /app/dist/ matches /app/dist/bundle.js
	cleanPattern := strings.TrimSuffix(pattern, "/")
	if strings.HasPrefix(path, cleanPattern+"/") || path == cleanPattern {
		return true
	}

	return false
}

// matchEndpoint checks if a target endpoint matches a declared endpoint pattern.
func matchEndpoint(declared, target string) bool {
	if declared == target {
		return true
	}
	// Wildcard: "*:443" matches any host on port 443
	if strings.HasPrefix(declared, "*") {
		suffix := declared[1:]
		return strings.HasSuffix(target, suffix)
	}
	// Prefix match for domain patterns: "*.github.com:443"
	if strings.Contains(declared, "*") {
		matched, _ := filepath.Match(declared, target)
		return matched
	}
	return false
}

// matchBinary checks if a target binary matches a declared binary pattern.
func matchBinary(declared, target string) bool {
	if declared == target {
		return true
	}
	// Base name match: "npm" matches "/usr/bin/npm"
	if filepath.Base(target) == declared {
		return true
	}
	if filepath.Base(declared) == filepath.Base(target) {
		return true
	}
	return false
}

// matchArgs checks if actual arguments match declared argument patterns.
func matchArgs(declared []string, actual []string) bool {
	if len(declared) == 0 {
		return true // No argument restrictions
	}
	// Each declared arg is a pattern; all actual args must match at least one pattern.
	for _, arg := range actual {
		matched := false
		for _, pattern := range declared {
			if pattern == "*" || pattern == arg {
				matched = true
				break
			}
			if m, _ := filepath.Match(pattern, arg); m {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// --- Format helpers for error messages ---

func formatFileOps(ops []FileOp, action string) string {
	var paths []string
	for _, op := range ops {
		if op.Action == action {
			paths = append(paths, op.Path)
		}
	}
	if len(paths) == 0 {
		return "none declared"
	}
	return strings.Join(paths, ", ")
}

func formatNetworkOps(ops []NetworkOp) string {
	var endpoints []string
	for _, op := range ops {
		endpoints = append(endpoints, fmt.Sprintf("%s %s", op.Method, op.Endpoint))
	}
	if len(endpoints) == 0 {
		return "none declared"
	}
	return strings.Join(endpoints, ", ")
}

func formatProcessOps(ops []ProcessOp) string {
	var binaries []string
	for _, op := range ops {
		binaries = append(binaries, op.Binary)
	}
	if len(binaries) == 0 {
		return "none declared"
	}
	return strings.Join(binaries, ", ")
}

func formatCredentialOps(ops []CredentialOp) string {
	var names []string
	for _, op := range ops {
		names = append(names, op.Name)
	}
	if len(names) == 0 {
		return "none declared"
	}
	return strings.Join(names, ", ")
}
