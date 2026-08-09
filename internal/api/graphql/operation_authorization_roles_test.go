package graphql

import (
	"sort"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
)

// TestTransportGateRoleMapIsExhaustive is the compile-time coverage guard
// required by .loom/32-sprint4-execution-specs.md:463. It compares the mapping
// against parsedSchema — the same *ast.Schema the running server executes, not
// a re-parse of the file — so a root field added to schema.graphql without a
// role decision fails here rather than reaching production unmapped.
//
// It fails in both directions on purpose: a missing entry is an accidental
// default-deny in production, and a stale entry is a role decision about a
// field that no longer exists.
func TestTransportGateRoleMapIsExhaustive(t *testing.T) {
	for _, tc := range []struct {
		operation  ast.Operation
		definition *ast.Definition
	}{
		{ast.Query, parsedSchema.Query},
		{ast.Mutation, parsedSchema.Mutation},
		{ast.Subscription, parsedSchema.Subscription},
	} {
		t.Run(string(tc.operation), func(t *testing.T) {
			if tc.definition == nil {
				t.Fatalf("schema has no %s root type", tc.operation)
			}
			mapped, known := rootFieldRoles[tc.operation]
			if !known {
				t.Fatalf("rootFieldRoles has no %s section", tc.operation)
			}
			schemaFields := make(map[string]bool, len(tc.definition.Fields))
			for _, field := range tc.definition.Fields {
				// gqlparser attaches the spec's meta-fields to the root types.
				// They are handled by introspectionRootFields and __typename.
				if field.Name == "__schema" || field.Name == "__type" || field.Name == "__typename" {
					continue
				}
				schemaFields[field.Name] = true
				if _, ok := mapped[field.Name]; !ok {
					t.Errorf("%s.%s has no transport-gate role mapping; add one to rootFieldRoles", tc.operation, field.Name)
				}
			}
			for name := range mapped {
				if !schemaFields[name] {
					t.Errorf("rootFieldRoles names %s.%s, which is not in the schema", tc.operation, name)
				}
			}
		})
	}
}

// TestTransportGateRoleMapShape pins the enumeration the lane re-scoped around
// (.loom/iteration-plan-phase4-transport-gate-roles.md). If a later slice
// narrows part of the compatibility bucket these numbers move, and moving them
// deliberately is the point: the count is the lane's public claim about how much
// of the surface is still ungoverned.
func TestTransportGateRoleMapShape(t *testing.T) {
	total, fineGrained, compatibility := transportGatePolicySummary()
	if total != 131 {
		t.Errorf("mapped root fields = %d, want 131", total)
	}
	if fineGrained != 16 {
		t.Errorf("fine-grained root fields = %d, want 16", fineGrained)
	}
	if compatibility != 115 {
		t.Errorf("compatibility-bucket root fields = %d, want 115", compatibility)
	}
	if total != fineGrained+compatibility {
		t.Fatalf("buckets do not partition the surface: %d + %d != %d", fineGrained, compatibility, total)
	}
}

// TestTransportGateNeverRequiresPHIExportRole holds the acceptance criterion
// that integration.phi.export alone reaches nothing at the transport gate. The
// role gates the includeRawPayload argument of exportIntegrationBundle
// (internal/integration/session/types.go:247), not any field, so it must never
// appear as a field requirement.
func TestTransportGateNeverRequiresPHIExportRole(t *testing.T) {
	const phiExportRole = "integration.phi.export"
	for operation, fields := range rootFieldRoles {
		for field, required := range fields {
			for _, role := range required {
				if role == phiExportRole {
					t.Errorf("%s.%s requires %s; it is an argument-level grant, not a field role", operation, field, phiExportRole)
				}
			}
		}
	}
}

// TestTransportGateRequirementsMatchTheServiceLayer keeps the gate from being
// more permissive than the service behind it. Each 4.2a field's AND-set must be
// exactly the set operator.Service.authorize demands for the same operation
// (internal/integration/operator/service.go:108-299).
func TestTransportGateRequirementsMatchTheServiceLayer(t *testing.T) {
	expected := map[ast.Operation]map[string][]string{
		ast.Query: {
			"operatorReceipts":         {operator.ReadRole},
			"operatorMessageTrace":     {operator.ReadRole},
			"operatorDeliveryAttempts": {operator.ReadRole},
			"operatorDeliveryAttempt":  {operator.ReadRole},
			"operatorDeadLetters":      {operator.ReadRole},
			"operatorCircuits":         {operator.ReadRole},
			"operatorAttemptAudit":     {operator.ReadRole},
			"operatorDeployments":      {operator.ReadRole},
			"operatorDeploymentEvents": {operator.ReadRole},
		},
		ast.Mutation: {
			"replayDelivery":              {operator.ReadRole, delivery.OperatorRole},
			"resubmitMessage":             {operator.ReadRole, delivery.OperatorRole},
			"discardDeadLetter":           {operator.ReadRole, delivery.OperatorRole},
			"pauseIntegrationDeployment":  {operator.ReadRole, operator.DeploymentOperatorRole},
			"resumeIntegrationDeployment": {operator.ReadRole, operator.DeploymentOperatorRole},
			"retireIntegrationDeployment": {operator.ReadRole, operator.DeploymentOperatorRole},
			"deployIntegrationRelease":    {operator.ReadRole, operator.DeploymentOperatorRole},
		},
	}
	for operation, fields := range expected {
		for field, want := range fields {
			got, ok := rootFieldRoles[operation][field]
			if !ok {
				t.Errorf("%s.%s is not mapped", operation, field)
				continue
			}
			if !sameRoleSet(got, want) {
				t.Errorf("%s.%s requires %v, want %v", operation, field, got, want)
			}
		}
	}
}

func sameRoleSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	gotSorted := append([]string(nil), got...)
	wantSorted := append([]string(nil), want...)
	sort.Strings(gotSorted)
	sort.Strings(wantSorted)
	for i := range gotSorted {
		if gotSorted[i] != wantSorted[i] {
			return false
		}
	}
	return true
}
