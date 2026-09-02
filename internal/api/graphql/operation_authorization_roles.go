package graphql

import (
	"fmt"

	gqlgengraphql "github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
)

// Transport-gate role mapping (Sprint 4, Lane S4-E).
//
// Before this slice the gate was binary: any caller holding graphql:operator
// was allowed every root field, and the fine-grained roles shipped by Slices
// 2.3, 4.2a, and S3-C1 were enforced only one layer deeper, inside
// operator.Service.authorize and the session services. This file makes the
// transport gate itself enumerate the surface: every root field names the roles
// it requires, and a field with no entry is refused.
//
// This is defence in depth, not a relocation. Nothing here replaces a
// service-layer check; a request that clears this gate is still authorized
// again by the service that answers it.
//
// # Requirement semantics
//
// A field's value is an AND-set: the caller must hold every listed role. That
// mirrors operator.Service.authorize, which requires every role passed to it
// (internal/integration/operator/service.go:88-92), so the gate cannot be more
// permissive than the service behind it.
//
// # The compatibility grant
//
// graphql:operator is retained as a named compatibility grant that expands to
// the full set (see compatibilityGrantExpandsToEveryRootField). A token holding
// it behaves exactly as it did before this slice. Every documented operator
// deployment mints exactly that role
// (.loom/32-sprint4-execution-specs.md:100), so removing it would take the
// control plane away from every existing install.
//
// # Why 105 fields are still in the compatibility bucket
//
// .loom/32-sprint4-execution-specs.md:469 named the lane's riskiest assumption
// — that the shipped fine-grained roles could already express every operation —
// and set the re-scope trigger. Enumeration disconfirmed it: only the sixteen
// Slice 4.2a control-plane fields and ten clinical-read fields have a shipped
// role. The legacy catalog keeps an explicit
// graphql:operator entry and a TODO naming its follow-up slice. The entries are
// explicit rather than implicit precisely so the remaining surface is
// enumerable and the follow-ups are greppable.
//
// Two roles are deliberately absent from this map:
//
//   - integration.phi.export gates the includeRawPayload *argument* of
//     exportIntegrationBundle, not the field
//     (internal/integration/session/types.go:247). It is not a field-level role
//     and mapping it here would widen access.
//   - integration:submit / integration:mllp / integration:batch are minted by
//     the ingress, MLLP, and batch transports for their own principals
//     (internal/integration/ingress/auth.go:144), never carried by a GraphQL
//     token. Mapping the submit mutations to them would hand ingress
//     principals a GraphQL surface they do not have today.

// legacyCompatibility marks a root field that no shipped fine-grained role
// describes yet. It is the explicit compatibility bucket: reachable only
// through the graphql:operator grant, and named per field so the narrowing
// follow-ups can be enumerated rather than guessed.
var legacyCompatibility = []string{GraphQLOperatorRole}

// clinicalRead is the least-privilege role for the legacy event, patient, and
// projection read surface.
var clinicalRead = []string{clinicalReadRole}

// operatorRead is the Slice 4.2a bounded, PHI-minimal control-plane read
// surface (internal/integration/operator/service.go:108-188).
var operatorRead = []string{operator.ReadRole}

// operatorRecovery matches operator.Service.recover, which requires the read
// role *and* the Slice 2.3 delivery operator role
// (internal/integration/operator/service.go:235).
var operatorRecovery = []string{operator.ReadRole, delivery.OperatorRole}

// operatorDeployment matches operator.Service.changeState, which requires the
// read role *and* the deployment operator role
// (internal/integration/operator/service.go:299).
var operatorDeployment = []string{operator.ReadRole, operator.DeploymentOperatorRole}

// rootFieldRoles covers all 131 schema root fields. A root field that is absent
// is refused: see transportGateRolesSatisfied. TestTransportGateRoleMapIsExhaustive
// fails when schema.graphql grows a root field this map does not name, so a new
// field cannot reach production without a role decision.
var rootFieldRoles = map[ast.Operation]map[string][]string{
	ast.Query: {
		// Slice 4.2a operator control plane — the narrowed surface.
		"operatorReceipts":         operatorRead,
		"operatorMessageTrace":     operatorRead,
		"operatorDeliveryAttempts": operatorRead,
		"operatorDeliveryAttempt":  operatorRead,
		"operatorDeadLetters":      operatorRead,
		"operatorCircuits":         operatorRead,
		"operatorAttemptAudit":     operatorRead,
		"operatorDeployments":      operatorRead,
		"operatorDeploymentEvents": operatorRead,

		// Health and preview. health stays in the compatibility bucket on
		// purpose: the low-privilege path to it is the untouched
		// integration:preview allowlist, and opening it to any authenticated
		// caller would undo the "unprivileged role stops before resolver" case
		// in oidc_security_test.go.
		"health":                  legacyCompatibility,
		"parsePreview":            legacyCompatibility,
		"parsePreviewWithProfile": legacyCompatibility,

		// Clinical event, patient, and projection read surface.
		"event":                    clinicalRead,
		"events":                   clinicalRead,
		"patient":                  clinicalRead,
		"patients":                 clinicalRead,
		"patientTimeline":          clinicalRead,
		"eventStatistics":          clinicalRead,
		"activeEncounters":         clinicalRead,
		"activeEncounter":          clinicalRead,
		"activeEncounterByPatient": clinicalRead,
		"projectionStatus":         clinicalRead,

		// TODO(S5-legacy-catalog-roles): legacy workflow definition/version
		// catalog. The published production DSL does not run through it
		// (.loom/31-sprint3-execution-specs.md:95); an authoring role belongs
		// with whichever slice retires or productionises the catalog.
		"workflow":                 legacyCompatibility,
		"workflows":                legacyCompatibility,
		"workflowDefinitions":      legacyCompatibility,
		"workflowDefinition":       legacyCompatibility,
		"workflowVersions":         legacyCompatibility,
		"workflowVersion":          legacyCompatibility,
		"workflowRuns":             legacyCompatibility,
		"workflowRun":              legacyCompatibility,
		"workflowApprovalRequests": legacyCompatibility,
		"workflowRunTrace":         legacyCompatibility,

		// TODO(S5-session-workspace-roles): integration session workspace
		// (Slices 3.1-3.4). Session authorization exists in the service layer
		// but is not expressed as a transport role.
		"integrationSession":         legacyCompatibility,
		"integrationSessions":        legacyCompatibility,
		"sessionSamples":             legacyCompatibility,
		"sessionArtifacts":           legacyCompatibility,
		"sessionRuns":                legacyCompatibility,
		"sessionWorkflowSimulations": legacyCompatibility,
		"sessionPublications":        legacyCompatibility,
		"sessionRun":                 legacyCompatibility,
		"sessionDiagnostics":         legacyCompatibility,

		// TODO(S5-profile-roles): profile management (.loom/29).
		"profiles":         legacyCompatibility,
		"profile":          legacyCompatibility,
		"profileRevisions": legacyCompatibility,

		// TODO(S5-llm-roles): LLM/copilot surface. Cost and PHI exposure argue
		// for a dedicated grant.
		"explainWarnings":   legacyCompatibility,
		"extractEntities":   legacyCompatibility,
		"llmCapability":     legacyCompatibility,
		"analyzeQuality":    legacyCompatibility,
		"quickQualityScore": legacyCompatibility,
		"explainWorkflow":   legacyCompatibility,
		"classifyMessage":   legacyCompatibility,

		// TODO(S5-terminology-governance-roles): terminology mapping and
		// autoroute review. .loom/27 already defers role expectations here.
		"listMappings":          legacyCompatibility,
		"getMapping":            legacyCompatibility,
		"lookupMapping":         legacyCompatibility,
		"getUploadBatch":        legacyCompatibility,
		"exportMappingsCSV":     legacyCompatibility,
		"resolveMapping":        legacyCompatibility,
		"suggestMappings":       legacyCompatibility,
		"listPendingAutoroutes": legacyCompatibility,
		"getPendingAutoroute":   legacyCompatibility,
		"pendingAutorouteStats": legacyCompatibility,

		// TODO(S5-legacy-catalog-roles): Temporal introspection and the
		// workflow debugger.
		"temporalWorkflow":  legacyCompatibility,
		"temporalWorkflows": legacyCompatibility,
		"debugSession":      legacyCompatibility,
	},
	ast.Mutation: {
		// Slice 4.2a delivery recovery — read role plus the Slice 2.3 grant.
		"replayDelivery":    operatorRecovery,
		"resubmitMessage":   operatorRecovery,
		"discardDeadLetter": operatorRecovery,

		// Slice 4.2a lifecycle commands — read role plus the deployment grant.
		"pauseIntegrationDeployment":  operatorDeployment,
		"resumeIntegrationDeployment": operatorDeployment,
		"retireIntegrationDeployment": operatorDeployment,
		"deployIntegrationRelease":    operatorDeployment,

		// previewIntegrationMessage is the integration:preview surface. It keeps
		// its compatibility entry so a graphql:operator token reaches it exactly
		// as before; the preview role reaches it through the untouched preview
		// allowlist, not through this map.
		"previewIntegrationMessage": legacyCompatibility,

		// TODO(S5-submit-roles): GraphQL submit mutations. The colon-form submit
		// grants are minted by the ingress, MLLP, and batch transports for their
		// own principals and are never carried by a GraphQL token, so mapping
		// these to them would widen access rather than narrow it.
		"submitMessage":   legacyCompatibility,
		"submitEvent":     legacyCompatibility,
		"submitBatch":     legacyCompatibility,
		"triggerWorkflow": legacyCompatibility,

		// TODO(S5-legacy-catalog-roles): legacy workflow authoring and approval.
		"createWorkflowDefinition":  legacyCompatibility,
		"updateWorkflowDefinition":  legacyCompatibility,
		"saveWorkflowVersion":       legacyCompatibility,
		"publishWorkflowVersion":    legacyCompatibility,
		"rollbackWorkflowVersion":   legacyCompatibility,
		"archiveWorkflowDefinition": legacyCompatibility,
		"requestWorkflowApproval":   legacyCompatibility,
		"approveWorkflowVersion":    legacyCompatibility,
		"rejectWorkflowVersion":     legacyCompatibility,

		// TODO(S5-legacy-catalog-roles): FHIR subscription management.
		"createFhirSubscription": legacyCompatibility,
		"deleteFhirSubscription": legacyCompatibility,
		"pauseFhirSubscription":  legacyCompatibility,
		"resumeFhirSubscription": legacyCompatibility,

		// TODO(S5-session-workspace-roles): integration session workspace.
		// exportIntegrationBundle stays here deliberately —
		// integration.phi.export gates its includeRawPayload argument, not the
		// field, so it is not a field-level role.
		"createIntegrationSession":   legacyCompatibility,
		"archiveIntegrationSession":  legacyCompatibility,
		"addSessionSample":           legacyCompatibility,
		"updateSessionProfileDraft":  legacyCompatibility,
		"updateSessionWorkflowDraft": legacyCompatibility,
		"runSessionPreview":          legacyCompatibility,
		"simulateSessionWorkflow":    legacyCompatibility,
		"publishIntegrationSession":  legacyCompatibility,
		"approveSessionPublication":  legacyCompatibility,
		"deploySessionPublication":   legacyCompatibility,
		"acceptDiagnosticFix":        legacyCompatibility,
		"exportIntegrationBundle":    legacyCompatibility,

		// TODO(S5-profile-roles): profile management (.loom/29).
		"createProfile":    legacyCompatibility,
		"updateProfile":    legacyCompatibility,
		"deleteProfile":    legacyCompatibility,
		"duplicateProfile": legacyCompatibility,

		// TODO(S5-llm-roles): LLM-backed authoring.
		"generateWorkflow": legacyCompatibility,
		"dryRunWorkflow":   legacyCompatibility,

		// TODO(S5-terminology-governance-roles): terminology writes and
		// autoroute approval. .loom/27 defers role expectations here.
		"uploadMappingCSV":             legacyCompatibility,
		"createMapping":                legacyCompatibility,
		"updateMapping":                legacyCompatibility,
		"deleteMapping":                legacyCompatibility,
		"deleteMappingBatch":           legacyCompatibility,
		"approvePendingAutoroute":      legacyCompatibility,
		"rejectPendingAutoroute":       legacyCompatibility,
		"bulkApprovePendingAutoroutes": legacyCompatibility,
		"startTerminologyReview":       legacyCompatibility,
		"signalReviewDecision":         legacyCompatibility,
		"cancelTemporalWorkflow":       legacyCompatibility,

		// TODO(S5-legacy-catalog-roles): interactive workflow debugger.
		"startDebugSession":     legacyCompatibility,
		"debugStep":             legacyCompatibility,
		"debugContinue":         legacyCompatibility,
		"debugSetBreakpoint":    legacyCompatibility,
		"debugRemoveBreakpoint": legacyCompatibility,
		"debugEndSession":       legacyCompatibility,
	},
	ast.Subscription: {
		// TODO(S5-session-workspace-roles): every subscription. The two session
		// streams are additionally constrained by the stream-context rule in
		// integrationSessionStreamOperationAllowed, which this slice leaves
		// byte-for-byte unchanged.
		"eventStream":              legacyCompatibility,
		"workflowEvents":           legacyCompatibility,
		"patientEvents":            legacyCompatibility,
		"integrationSessionEvents": legacyCompatibility,
		"sessionRunEvents":         legacyCompatibility,
		"liveParseStream":          legacyCompatibility,
		"debugStepEvent":           legacyCompatibility,
	},
}

// transportGateRolesSatisfied reports whether the caller may run every root
// field the operation selects. It is default-deny: an unmapped root field, an
// operation with no resolvable root fields, and an unknown operation type all
// refuse.
//
// The GraphQL spec's meta-fields are handled by that default rather than by
// entries of their own. __schema and __type describe the shape of the API, so
// they are deliberately left unmapped and refused here; the only way to
// introspect remains the compatibility grant, which short-circuits before this
// function runs. __typename is a constant that leaks nothing, so it rides along
// with the real root fields it is selected beside — but it cannot carry an
// operation on its own: an operation selecting nothing else is refused, because
// admitting it would let any authenticated caller past a gate that refuses them
// everything else.
func transportGateRolesSatisfied(roles []string, operationContext *gqlgengraphql.OperationContext) bool {
	if operationContext == nil || operationContext.Doc == nil {
		return false
	}
	operation := operationContext.Doc.Operations.ForName(operationContext.OperationName)
	if operation == nil {
		return false
	}
	fieldRoles, known := rootFieldRoles[operation.Operation]
	if !known {
		return false
	}
	fields := rootFieldNames(operation.SelectionSet, make(map[*ast.FragmentDefinition]bool))
	authorized := 0
	for _, field := range fields {
		if field == "__typename" {
			continue
		}
		required, mapped := fieldRoles[field]
		if !mapped || len(required) == 0 {
			return false
		}
		for _, role := range required {
			if !hasOperationRole(roles, role) {
				return false
			}
		}
		authorized++
	}
	return authorized > 0
}

// transportGatePolicySummary describes the compiled mapping for the serve
// banner, so an operator can see at startup that the compatibility grant is
// still in the build and what it expands to.
func transportGatePolicySummary() (total, fineGrained, compatibility int) {
	for _, fields := range rootFieldRoles {
		for _, required := range fields {
			total++
			if len(required) == 1 && required[0] == GraphQLOperatorRole {
				compatibility++
				continue
			}
			fineGrained++
		}
	}
	return total, fineGrained, compatibility
}

// TransportGatePolicyLine is the one-line startup description of the GraphQL
// transport gate. serve prints it beside the GraphQL server so the named
// compatibility grant is visible in the log of every deployment still relying
// on it.
func TransportGatePolicyLine() string {
	total, fineGrained, compatibility := transportGatePolicySummary()
	return fmt.Sprintf(
		"GraphQL transport gate: %d root fields mapped (%d fine-grained, %d behind the %q compatibility grant, which expands to all %d)",
		total, fineGrained, compatibility, GraphQLOperatorRole, total,
	)
}
