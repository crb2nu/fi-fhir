package workflow

import (
	"os"
	"path/filepath"
	"strings"
)

// EnvExecAllowlist names the deployment-owned list of executables the `exec`
// action may run: comma-separated absolute paths. It is read when an Engine is
// built and is empty by default, which makes `exec` refuse every command.
//
// Before 2026-09-27 the only allowlist was the action's own `allowlist` config
// key, i.e. part of the same workflow YAML that names the command, so whoever
// could submit a workflow could also authorise any binary in the image. The
// YAML key is still required and can only narrow: a command runs when it is in
// both lists. See docs/operations/SECURITY.md (SEC-2026-09-27-1).
const EnvExecAllowlist = "FI_FHIR_WORKFLOW_EXEC_ALLOWLIST"

// ExecAllowlistFromEnv returns the deployment exec allowlist.
func ExecAllowlistFromEnv() []string {
	return ParseExecAllowlist(os.Getenv(EnvExecAllowlist))
}

// ParseExecAllowlist parses a comma-separated list of executables. Entries
// that are not absolute, or that are not already in clean form (for example
// `/usr/bin/../bin/sh`), are dropped rather than normalised: a deployment
// allowlist names exact paths.
func ParseExecAllowlist(raw string) []string {
	var paths []string
	seen := make(map[string]struct{})
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" || !filepath.IsAbs(item) || filepath.Clean(item) != item {
			continue
		}
		if _, dup := seen[item]; dup {
			continue
		}
		seen[item] = struct{}{}
		paths = append(paths, item)
	}
	return paths
}

// execCommandAllowed reports whether command may run: it must be named by the
// deployment allowlist and by the action's own allowlist. The action's list
// can only narrow the deployment's, never widen it.
func execCommandAllowed(command string, deployment []string, actionAllowlist string) bool {
	inDeployment := false
	for _, allowed := range deployment {
		if allowed == command {
			inDeployment = true
			break
		}
	}
	if !inDeployment {
		return false
	}
	for _, item := range strings.Split(actionAllowlist, ",") {
		if strings.TrimSpace(item) == command {
			return true
		}
	}
	return false
}
