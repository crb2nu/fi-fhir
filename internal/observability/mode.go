// Package observability owns the serve process's truthful health, readiness,
// and Prometheus surfaces.
//
// The package exists because none of those surfaces existed before Slice 4.3:
// `/health` was a string literal, `/ready` was mounted nowhere, and `/metrics`
// was described by a complete deployment façade — pod annotations, a named
// container port, two Services, a scrape job, a Grafana dashboard, and 32 alert
// rules — with nothing listening behind it. See `.loom/40-decisions.md`
// (2026-08-08) for the registry decision and its rejected alternatives.
//
// Two rules hold everywhere in this package:
//
//   - Label values come from compile-time constant sets. A metric label is a
//     cardinality contract and an egress surface at the same time; a correlation
//     ID, receipt ID, tenant string, URL, or any message-derived value is never
//     a label value here.
//   - An absent dependency reports "not configured", never "healthy". A
//     truthful absence is the whole point of the slice.
package observability

import (
	"os"
	"strings"
)

// ModeEnvVar names the environment variable that selects observability
// behaviour.
const ModeEnvVar = "FI_FHIR_OBSERVABILITY_MODE"

// Mode selects between the shipped observability behaviour and the pre-slice
// behaviour.
type Mode string

const (
	// ModeCurrent is the shipped behaviour and the default.
	ModeCurrent Mode = "current"

	// ModeLegacy restores the pre-Slice-4.3 behaviour at every seam this slice
	// touches: a literal `/health`, no `/ready`, no metrics listener, a
	// process-local session hub, process-local notifier de-duplication, and a
	// required batch worker ID with no derived default.
	//
	// It exists for exactly one reason: the kill-test
	// TestServeObservability_TwoReplicasUnderDocumentedConfiguration runs its
	// assertions against this mode as a negative control, and CI asserts that
	// the control *fails*. A proof that cannot fail is not a proof
	// (`.loom/31-sprint3-execution-specs.md` corrections 6 and 28).
	//
	// It is not a supported production configuration. `serve` prints a warning
	// when it is set, `.env.example` and `docs/operations/README.md` say so, and
	// `scripts/check-runtime-config.sh` refuses it.
	ModeLegacy Mode = "legacy"
)

// ModeFromEnv reads the mode from the process environment. Anything other than
// the exact string "legacy" is the current mode, so a typo degrades to correct
// behaviour rather than to the negative control.
func ModeFromEnv() Mode {
	return ParseMode(os.Getenv(ModeEnvVar))
}

// ParseMode maps a configured value onto a Mode.
func ParseMode(value string) Mode {
	if strings.EqualFold(strings.TrimSpace(value), string(ModeLegacy)) {
		return ModeLegacy
	}
	return ModeCurrent
}

// Legacy reports whether the pre-slice behaviour is selected.
func (m Mode) Legacy() bool { return m == ModeLegacy }

// String renders the mode for logs and diagnostics.
func (m Mode) String() string {
	if m.Legacy() {
		return string(ModeLegacy)
	}
	return string(ModeCurrent)
}
