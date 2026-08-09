//go:build !fhirdrnoteonly

package fhir

// diagnosticReportProfiles is the set of US Core profiles this checker accepts
// on a DiagnosticReport.
//
// US Core defines two, and the distinction is real rather than editorial:
//
//   - us-core-diagnosticreport-lab — Laboratory Results Reporting. Required
//     category `LAB`, `code` bound to LOINC, `result` referencing US Core
//     Laboratory Result Observations. This is what MapLabResult produces.
//   - us-core-diagnosticreport-note — Report and Note exchange, for clinical
//     notes and imaging/diagnostic reports. This is what MapDiagnosticReportNote
//     produces.
//
// Before Slice 5.1a only `-note` was accepted, while MapLabResult stamped `-lab`
// as a bare literal that was never declared as a constant — which is exactly how
// it fell out of this set. The shipped validator therefore rejected the shipped
// mapper's own output on the one input the CI fixtures never exercised.
//
// Which side to move was not a coin toss: a lab panel with category LAB and
// LOINC-coded results conforms to `-lab`, and re-stamping it `-note` to satisfy
// the checker would have made the resource wrong to satisfy a check that was
// wrong. The mapper was right; the accepted set was incomplete.
//
// The `fhirdrnoteonly` build tag restores the pre-5.1a set. It is the
// conformance table's negative control: with the tag on, the DiagnosticReport
// row — and only that row — must fail. See `make fhir-conformance-negative-control`.
func diagnosticReportProfiles() map[string]bool {
	return map[string]bool{
		USCoreDiagnosticReportLabProfile:  true,
		USCoreDiagnosticReportNoteProfile: true,
	}
}
