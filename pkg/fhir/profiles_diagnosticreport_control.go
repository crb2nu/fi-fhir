//go:build fhirdrnoteonly

package fhir

// diagnosticReportProfiles, under the `fhirdrnoteonly` tag, restores the
// pre-Slice-5.1a accepted set: `-note` only.
//
// This file exists solely as the conformance table's negative control. It
// reverts the DiagnosticReport reconciliation without reverting anything else,
// so `make fhir-conformance-negative-control` can require that the table fails
// on exactly the DiagnosticReport row and passes everywhere else. A control that
// passes would mean the table is not round-tripping the mapper's own bytes and
// the proof above it is vacuous.
//
// Nothing but the control build should ever set this tag.
func diagnosticReportProfiles() map[string]bool {
	return map[string]bool{USCoreDiagnosticReportNoteProfile: true}
}
