package workflow

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// EnvDebugActions names the deployment-owned list of action types the
// interactive workflow debugger may execute for real: comma-separated action
// type names. It is empty by default, so the debugger runs every action as a
// recording no-op stub — which is what a debugger should do with a workflow a
// caller pasted in. `exec` can never be enabled here.
//
// Before 2026-09-27 the debugger built a full production engine, so
// `startDebugSession` executed webhook, fhir, email, exec, file, database,
// queue, event_store and athena actions from caller-supplied YAML. See
// docs/operations/SECURITY.md (SEC-2026-09-27-1).
const EnvDebugActions = "FI_FHIR_WORKFLOW_DEBUG_ACTIONS"

// Span attributes the debugger adds to action spans. They are set only on
// engines built by NewDebugEngine, never on production spans.
const (
	// AttrActionStubbed is true when the debugger replaced the action with a
	// recording no-op.
	AttrActionStubbed = "action.stubbed"
	// AttrActionInputs is the action's config with templates resolved against
	// the (transformed) event, credential-named keys redacted, and bounded.
	AttrActionInputs = "action.inputs"
)

// debugNeverExecutable are action types the debugger never runs for real,
// whatever the deployment allowlist says.
var debugNeverExecutable = map[string]struct{}{"exec": {}}

const (
	debugInputMaxKeys       = 64
	debugInputMaxValueBytes = 2048
	debugRedacted           = "[redacted]"
)

// credentialConfigKey matches config keys whose value is, or carries, a
// credential. Their values never reach a debug trace.
var credentialConfigKey = regexp.MustCompile(`(?i)(password|passwd|secret|token|api[_-]?key|private[_-]?key|authorization|auth_header|credential|dsn|connection[_-]?string|cookie)`)

// ParseDebugActionAllowlist parses FI_FHIR_WORKFLOW_DEBUG_ACTIONS. It returns
// the action types the debugger may execute and, separately, the ones it
// refused because they can never be enabled (`exec`). Names are lower-cased
// and de-duplicated; the result is sorted.
func ParseDebugActionAllowlist(raw string) (allowed []string, refused []string) {
	seen := make(map[string]struct{})
	for _, item := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(item))
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		if _, never := debugNeverExecutable[name]; never {
			refused = append(refused, name)
			continue
		}
		allowed = append(allowed, name)
	}
	sort.Strings(allowed)
	sort.Strings(refused)
	return allowed, refused
}

// NewDebugEngine builds the engine the interactive debugger runs. Every
// registered action whose type is not in executable is replaced by a recording
// no-op stub; `exec` is always stubbed. Action spans carry the action's
// resolved inputs (AttrActionInputs) and whether it was stubbed
// (AttrActionStubbed), so a paused action step shows what the action would
// have done.
func NewDebugEngine(wf *Workflow, executable []string) (*Engine, error) {
	engine, err := NewEngine(wf)
	if err != nil {
		return nil, err
	}
	// Belt and braces: even if exec were somehow executable it would refuse.
	engine.SetExecAllowlist(nil)

	allowed := make(map[string]struct{}, len(executable))
	for _, name := range executable {
		name = strings.ToLower(strings.TrimSpace(name))
		if _, never := debugNeverExecutable[name]; never || name == "" {
			continue
		}
		allowed[name] = struct{}{}
	}

	engine.debugStubbed = make(map[string]bool)
	for _, name := range engine.RegisteredActionTypes() {
		if _, ok := allowed[name]; ok {
			engine.debugStubbed[name] = false
			continue
		}
		engine.debugStubbed[name] = true
		engine.RegisterAction(name, debugStubAction())
	}
	return engine, nil
}

// IsDebugEngine reports whether the engine was built by NewDebugEngine.
func (e *Engine) IsDebugEngine() bool { return e.debugStubbed != nil }

// DebugStubbedAction reports whether the debug engine replaced an action type
// with a no-op stub. It is false on production engines.
func (e *Engine) DebugStubbedAction(actionType string) bool {
	return e.debugStubbed != nil && e.debugStubbed[actionType]
}

// debugStubAction is the recording no-op every non-allowlisted action becomes.
// The record is the action span itself: the engine attaches the resolved
// inputs before the span starts, so the stub has nothing left to do but
// succeed without side effects.
func debugStubAction() ContextActionHandlerFunc {
	return func(context.Context, interface{}, map[string]string) error { return nil }
}

// debugActionAttributes are the extra span attributes a debug engine attaches
// to an action span.
func (e *Engine) debugActionAttributes(action Action, event interface{}) []SpanAttribute {
	if e.debugStubbed == nil {
		return nil
	}
	return []SpanAttribute{
		Attr(AttrActionStubbed, e.DebugStubbedAction(action.Type)),
		Attr(AttrActionInputs, resolveDebugActionInputs(action.Config, event)),
	}
}

// resolveDebugActionInputs renders each config value against the event,
// redacts credential-named keys and URL passwords, and bounds the result.
func resolveDebugActionInputs(config map[string]string, event interface{}) map[string]interface{} {
	keys := make([]string, 0, len(config))
	for key := range config {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	inputs := make(map[string]interface{}, len(keys))
	for i, key := range keys {
		if i >= debugInputMaxKeys {
			inputs["_truncated"] = fmt.Sprintf("%d more keys omitted", len(keys)-debugInputMaxKeys)
			break
		}
		if credentialConfigKey.MatchString(key) {
			inputs[key] = debugRedacted
			continue
		}
		value := redactURLPassword(renderTemplate(config[key], event))
		if len(value) > debugInputMaxValueBytes {
			value = truncateForError(value, debugInputMaxValueBytes)
		}
		inputs[key] = value
	}
	return inputs
}

// redactURLPassword masks the password of a value that parses as an absolute
// URL carrying user info; every other value is returned unchanged.
func redactURLPassword(value string) string {
	if !strings.Contains(value, "://") || !strings.Contains(value, "@") {
		return value
	}
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.User == nil {
		return value
	}
	if _, hasPassword := parsed.User.Password(); !hasPassword {
		return value
	}
	return parsed.Redacted()
}
