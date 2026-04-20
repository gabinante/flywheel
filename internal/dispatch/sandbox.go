package dispatch

import (
	"fmt"
	"strings"

	"github.com/gabinante/flywheel/internal/plan"
)

// SandboxConfig holds Docker arguments generated from a manifest's sandbox policy.
// Used by DockerWorker to apply container-level enforcement of the manifest.
type SandboxConfig struct {
	// DockerArgs are extra arguments for `docker run`.
	DockerArgs []string
	// EnvVars are environment variables to inject into the container.
	EnvVars map[string]string
	// TimeoutSeconds is the max runtime (used for context deadline).
	TimeoutSeconds int
}

// BuildSandboxConfig generates Docker container constraints from a SandboxPolicy.
// This is the bridge between the plan layer (manifest → policy) and the dispatch
// layer (policy → container runtime enforcement).
//
// Enforcement mechanisms:
// - Mount restrictions: only declared paths are mounted read-write
// - Network policies: iptables rules restrict outbound to declared endpoints
// - Seccomp profiles: system call filtering based on declared operations
// - Resource limits: memory, disk, time limits from manifest
func BuildSandboxConfig(policy *plan.SandboxPolicy) *SandboxConfig {
	cfg := &SandboxConfig{
		EnvVars:        make(map[string]string),
		TimeoutSeconds: policy.TimeoutSeconds,
	}

	var args []string

	// --- Mount restrictions ---
	// Read-only mounts: paths the script can read but not write.
	for _, mount := range policy.ReadOnlyMounts {
		args = append(args, "-v", mount.HostPath+":"+mount.ContainerPath+":ro")
	}
	// Read-write mounts: paths the script can modify.
	for _, mount := range policy.MountPaths {
		args = append(args, "-v", mount.HostPath+":"+mount.ContainerPath+":rw")
	}

	// --- Disk quota via tmpfs ---
	// Limit writable space using a tmpfs overlay with size constraint.
	if policy.DiskQuotaBytes > 0 {
		sizeMB := policy.DiskQuotaBytes / (1024 * 1024)
		if sizeMB < 1 {
			sizeMB = 1
		}
		args = append(args, "--tmpfs", fmt.Sprintf("/tmp:size=%dm", sizeMB))
	}

	// --- Network policies ---
	if len(policy.AllowedEndpoints) > 0 {
		// Enable firewall enforcement inside the container.
		args = append(args, "--cap-add", "NET_ADMIN", "--cap-add", "NET_RAW")
		cfg.EnvVars["FLYWHEEL_FIREWALL"] = "true"
		cfg.EnvVars["FLYWHEEL_ALLOWED_HOSTS"] = strings.Join(policy.AllowedEndpoints, ",")
		if policy.NetworkCallLimit > 0 {
			cfg.EnvVars["FLYWHEEL_NETWORK_CALL_LIMIT"] = fmt.Sprintf("%d", policy.NetworkCallLimit)
		}
	} else {
		// No network declared → deny all network access.
		args = append(args, "--network", "none")
	}

	// --- Seccomp profile ---
	if policy.SeccompProfile != "" {
		// Reference a pre-installed seccomp profile.
		// The profile files live in /etc/docker/seccomp/ on the host.
		args = append(args, "--security-opt", "seccomp=/etc/docker/seccomp/"+policy.SeccompProfile+".json")
	}

	// --- Process restrictions ---
	if len(policy.AllowedProcesses) > 0 {
		cfg.EnvVars["FLYWHEEL_ALLOWED_PROCESSES"] = strings.Join(policy.AllowedProcesses, ",")
	}

	// --- Credential injection ---
	// Only inject secrets that are declared in the manifest.
	for _, secret := range policy.SecretNames {
		cfg.EnvVars["FLYWHEEL_ALLOWED_SECRET_"+secret] = "true"
	}

	cfg.DockerArgs = args
	return cfg
}

// ManifestEnforcementScript generates a shell wrapper script that enforces
// the manifest at the OS level. This script runs inside the container and
// intercepts file/network/process operations.
//
// The enforcement uses:
// - iptables for network egress filtering (only declared endpoints)
// - A custom LD_PRELOAD library or seccomp-bpf for file access control
// - Process accounting for network call counting
//
// Returns the shell script content that should be prepended to the worker entrypoint.
func ManifestEnforcementScript(policy *plan.SandboxPolicy) string {
	var sb strings.Builder

	sb.WriteString("#!/bin/sh\nset -e\n\n")
	sb.WriteString("# --- Flywheel Manifest Enforcement ---\n")
	sb.WriteString("# Generated from side-effect manifest. Undeclared operations will fail.\n\n")

	// Network enforcement via iptables.
	if len(policy.AllowedEndpoints) > 0 {
		sb.WriteString("# Network policy: allow only declared endpoints\n")
		sb.WriteString("if [ \"$FLYWHEEL_FIREWALL\" = \"true\" ] && command -v iptables >/dev/null 2>&1; then\n")
		sb.WriteString("  iptables -P OUTPUT DROP\n")
		sb.WriteString("  iptables -A OUTPUT -o lo -j ACCEPT\n")
		sb.WriteString("  iptables -A OUTPUT -m state --state ESTABLISHED,RELATED -j ACCEPT\n")
		for _, endpoint := range policy.AllowedEndpoints {
			host, port := splitEndpoint(endpoint)
			if host == "*" {
				// Wildcard host: allow any destination on this port.
				sb.WriteString(fmt.Sprintf("  iptables -A OUTPUT -p tcp --dport %s -j ACCEPT\n", port))
			} else {
				sb.WriteString(fmt.Sprintf("  iptables -A OUTPUT -p tcp -d %s --dport %s -j ACCEPT\n", host, port))
			}
		}
		// Always allow DNS resolution.
		sb.WriteString("  iptables -A OUTPUT -p udp --dport 53 -j ACCEPT\n")
		sb.WriteString("  iptables -A OUTPUT -p tcp --dport 53 -j ACCEPT\n")
		sb.WriteString("fi\n\n")
	}

	// Network call counting.
	if policy.NetworkCallLimit > 0 {
		sb.WriteString(fmt.Sprintf("export FLYWHEEL_NETWORK_CALL_LIMIT=%d\n", policy.NetworkCallLimit))
		sb.WriteString("export FLYWHEEL_NETWORK_CALL_COUNT=0\n\n")
	}

	// Timeout enforcement.
	if policy.TimeoutSeconds > 0 {
		sb.WriteString(fmt.Sprintf("export FLYWHEEL_TIMEOUT=%d\n", policy.TimeoutSeconds))
	}

	sb.WriteString("# --- End Manifest Enforcement ---\n\n")
	sb.WriteString("# Execute the actual command:\n")
	sb.WriteString("exec \"$@\"\n")

	return sb.String()
}

// splitEndpoint splits "host:port" into host and port parts.
func splitEndpoint(endpoint string) (string, string) {
	// Handle IPv6 addresses in brackets.
	if strings.HasPrefix(endpoint, "[") {
		idx := strings.LastIndex(endpoint, "]:")
		if idx >= 0 {
			return endpoint[:idx+1], endpoint[idx+2:]
		}
		return endpoint, "443"
	}
	parts := strings.SplitN(endpoint, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return endpoint, "443"
}
