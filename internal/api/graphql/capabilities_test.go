package graphql

import (
	"reflect"
	"strings"
	"testing"

	gqlgengraphql "github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// productionRoles is what fi-fhir-api.yaml granted the static bearer, the
// trusted network, and both Access principals from 2026-09-05 to 2026-09-25.
var productionRoles = []string{previewRole, GraphQLOperatorRole, clinicalReadRole}

// operatorBundle is the documented grant: the compatibility grant for the IDE
// plus every role operator.Service checks.
var operatorBundle = []string{
	previewRole, GraphQLOperatorRole, clinicalReadRole,
	operator.ReadRole, delivery.OperatorRole, operator.DeploymentOperatorRole,
}

func securityFor(authVia, principal string, roles ...string) integration.SecurityContext {
	return integration.SecurityContext{
		TenantID: "tenant-a",
		Principal: integration.Principal{
			ID:         principal,
			Kind:       integration.PrincipalKindHuman,
			AuthMethod: authVia,
			Roles:      roles,
		},
	}
}

// TestAuthCapabilities is the R-A kill-test in table form. Its first row is the
// production outage: a transport grant with no control-plane role must say
// operatorRead=false and name integration.operator as the missing role.
func TestAuthCapabilities(t *testing.T) {
	none := []string{}
	tests := []struct {
		name         string
		security     integration.SecurityContext
		config       ServerConfig
		capabilities accessCapabilities
		missing      missingRoles
	}{
		{
			name:     "transport grant without control-plane roles (the 2026-09 production identity)",
			security: securityFor("network", "fi-fhir-ide-operator", productionRoles...),
			capabilities: accessCapabilities{
				ClinicalRead: true, Subscriptions: none,
			},
			missing: missingRoles{
				OperatorRead:       []string{operator.ReadRole},
				OperatorDelivery:   []string{operator.ReadRole, delivery.OperatorRole},
				OperatorDeployment: []string{operator.ReadRole, operator.DeploymentOperatorRole},
				ClinicalRead:       none,
			},
		},
		{
			name:     "full operator bundle",
			security: securityFor("network", "fi-fhir-ide-operator", operatorBundle...),
			capabilities: accessCapabilities{
				OperatorRead: true, OperatorDelivery: true, OperatorDeployment: true,
				ClinicalRead: true, Subscriptions: none,
			},
			missing: missingRoles{OperatorRead: none, OperatorDelivery: none, OperatorDeployment: none, ClinicalRead: none},
		},
		{
			name:     "control-plane read without the recovery or deployment grants",
			security: securityFor("bearer", "reader", previewRole, GraphQLOperatorRole, operator.ReadRole),
			capabilities: accessCapabilities{
				OperatorRead: true, ClinicalRead: true, Subscriptions: none,
			},
			missing: missingRoles{
				OperatorRead:       none,
				OperatorDelivery:   []string{delivery.OperatorRole},
				OperatorDeployment: []string{operator.DeploymentOperatorRole},
				ClinicalRead:       none,
			},
		},
		{
			name:     "clinical:read alone",
			security: securityFor("cloudflare-access", "analyst@example.com", previewRole, clinicalReadRole),
			capabilities: accessCapabilities{
				ClinicalRead: true, Subscriptions: none,
			},
			missing: missingRoles{
				OperatorRead:       []string{operator.ReadRole},
				OperatorDelivery:   []string{operator.ReadRole, delivery.OperatorRole},
				OperatorDeployment: []string{operator.ReadRole, operator.DeploymentOperatorRole},
				ClinicalRead:       none,
			},
		},
		{
			// The service bearer's fixed roles: operator reads, no clinical data,
			// and no subscription even with streaming on, because the session
			// streams are still behind the compatibility grant.
			name:     "service bearer",
			security: securityFor("service-bearer", "mentatlab", operator.ReadRole, previewRole),
			config:   ServerConfig{IntegrationSessionStreaming: true, IntegrationSessionsConfigured: true},
			capabilities: accessCapabilities{
				OperatorRead: true, IntegrationSessions: true, Streaming: true, Subscriptions: none,
			},
			missing: missingRoles{
				OperatorRead:       none,
				OperatorDelivery:   []string{delivery.OperatorRole},
				OperatorDeployment: []string{operator.DeploymentOperatorRole},
				ClinicalRead:       []string{clinicalReadRole},
			},
		},
		{
			name:     "preview only",
			security: securityFor("bearer", "previewer", previewRole),
			capabilities: accessCapabilities{
				Subscriptions: none,
			},
			missing: missingRoles{
				OperatorRead:       []string{operator.ReadRole},
				OperatorDelivery:   []string{operator.ReadRole, delivery.OperatorRole},
				OperatorDeployment: []string{operator.ReadRole, operator.DeploymentOperatorRole},
				ClinicalRead:       []string{clinicalReadRole},
			},
		},
		{
			name:     "streaming on",
			security: securityFor("network", "fi-fhir-ide-operator", operatorBundle...),
			config:   ServerConfig{IntegrationSessionStreaming: true, IntegrationSessionsConfigured: true, LLMConfigured: true},
			capabilities: accessCapabilities{
				OperatorRead: true, OperatorDelivery: true, OperatorDeployment: true, ClinicalRead: true,
				IntegrationSessions: true, Streaming: true,
				Subscriptions: []string{"integrationSessionEvents", "sessionRunEvents"},
				LLM:           llmState{Configured: true},
			},
			missing: missingRoles{OperatorRead: none, OperatorDelivery: none, OperatorDeployment: none, ClinicalRead: none},
		},
		{
			name:     "streaming off with the session store wired",
			security: securityFor("network", "fi-fhir-ide-operator", operatorBundle...),
			config:   ServerConfig{IntegrationSessionsConfigured: true},
			capabilities: accessCapabilities{
				OperatorRead: true, OperatorDelivery: true, OperatorDeployment: true, ClinicalRead: true,
				IntegrationSessions: true, Subscriptions: none,
			},
			missing: missingRoles{OperatorRead: none, OperatorDelivery: none, OperatorDeployment: none, ClinicalRead: none},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.config
			status := deriveAuthStatus(tt.security, &config)
			if !status.Authenticated || status.AuthVia != tt.security.Principal.AuthMethod || status.Principal != tt.security.Principal.ID {
				t.Fatalf("identity = %+v", status)
			}
			if !reflect.DeepEqual(status.Roles, tt.security.Principal.Roles) {
				t.Fatalf("roles = %v, want %v", status.Roles, tt.security.Principal.Roles)
			}
			if !reflect.DeepEqual(status.Capabilities, tt.capabilities) {
				t.Fatalf("capabilities = %+v\nwant           %+v", status.Capabilities, tt.capabilities)
			}
			if !reflect.DeepEqual(status.MissingRoles, tt.missing) {
				t.Fatalf("missingRoles = %+v\nwant           %+v", status.MissingRoles, tt.missing)
			}
		})
	}
}

// TestAuthCapabilitiesReportEveryAuthVia pins that authVia and principal are
// the SecurityContext's own AuthMethod and ID for each identity the server can
// mint, not a value the handler chooses.
func TestAuthCapabilitiesReportEveryAuthVia(t *testing.T) {
	for _, authVia := range []string{"network", "bearer", "service-bearer", "cloudflare-access", "oidc"} {
		t.Run(authVia, func(t *testing.T) {
			status := deriveAuthStatus(securityFor(authVia, "principal-"+authVia, previewRole), &ServerConfig{})
			if status.AuthVia != authVia || status.Principal != "principal-"+authVia {
				t.Fatalf("status = %+v", status)
			}
		})
	}
}

func TestAuthCapabilitiesNeverAliasTheCallersRoles(t *testing.T) {
	roles := []string{previewRole, GraphQLOperatorRole}
	status := deriveAuthStatus(securityFor("network", "p", roles...), &ServerConfig{})
	status.Roles[0] = "mutated"
	if roles[0] != previewRole {
		t.Fatal("the status body aliases the security context's role slice")
	}
}

// TestAuthCapabilityRepresentativesCoverTheirGroup proves that the one root
// field each role capability evaluates stands for every field of its surface:
// they all carry the same transport requirement. If a surface's fields ever
// diverge, the capability has to be split, and this fails first.
func TestAuthCapabilityRepresentativesCoverTheirGroup(t *testing.T) {
	groups := []struct {
		capability roleCapability
		operation  ast.Operation
		fields     []string
	}{
		{operatorReadCapability, ast.Query, []string{
			"operatorReceipts", "operatorMessageTrace", "operatorDeliveryAttempts", "operatorDeliveryAttempt",
			"operatorDeadLetters", "operatorCircuits", "operatorAttemptAudit", "operatorDeployments", "operatorDeploymentEvents",
		}},
		{operatorDeliveryCapability, ast.Mutation, []string{"replayDelivery", "resubmitMessage", "discardDeadLetter"}},
		{operatorDeploymentCapability, ast.Mutation, []string{
			"pauseIntegrationDeployment", "resumeIntegrationDeployment", "retireIntegrationDeployment", "deployIntegrationRelease",
		}},
		{clinicalReadCapability, ast.Query, []string{
			"event", "events", "patient", "patients", "patientTimeline", "eventStatistics",
			"activeEncounters", "activeEncounter", "activeEncounterByPatient", "projectionStatus",
		}},
	}
	for _, group := range groups {
		representative, mapped := rootFieldRoles[group.capability.operation][group.capability.field]
		if !mapped || len(representative) == 0 {
			t.Fatalf("%s.%s is not mapped by the transport gate", group.capability.operation, group.capability.field)
		}
		if group.capability.operation != group.operation {
			t.Fatalf("%s: representative operation %s, group %s", group.capability.field, group.capability.operation, group.operation)
		}
		for _, field := range group.fields {
			if got := rootFieldRoles[group.operation][field]; !sameRoleSet(got, representative) {
				t.Errorf("%s.%s requires %v; its capability's representative %s requires %v",
					group.operation, field, got, group.capability.field, representative)
			}
		}
		// The service half of an operator capability is the transport half:
		// both are operator.Service.authorize's list.
		if group.capability.serviceRoles != nil && !sameRoleSet(group.capability.serviceRoles, representative) {
			t.Errorf("%s: service roles %v differ from the gate's %v", group.capability.field, group.capability.serviceRoles, representative)
		}
	}
}

// TestAuthCapabilitySubscriptionsAreTheTransportAllowlist proves the
// subscriptions list is read from integrationSessionStreamRoots — the variable
// integrationSessionStreamOperationAllowed checks — and not from a copy.
func TestAuthCapabilitySubscriptionsAreTheTransportAllowlist(t *testing.T) {
	grant := []string{GraphQLOperatorRole}
	if got := streamSubscriptions(grant, true); !reflect.DeepEqual(got, integrationSessionStreamRoots) {
		t.Fatalf("subscriptions = %v, want the transport allowlist %v", got, integrationSessionStreamRoots)
	}
	if got := streamSubscriptions(grant, false); got == nil || len(got) != 0 {
		t.Fatalf("streaming off: subscriptions = %#v, want an empty list", got)
	}

	// The transport accepts exactly the allowlist on the stream...
	for _, document := range []struct {
		root  string
		query string
	}{
		{"integrationSessionEvents", `subscription S { integrationSessionEvents(sessionId: "s") { id } }`},
		{"sessionRunEvents", `subscription S { sessionRunEvents(runId: "r") { id } }`},
		{"eventStream", `subscription S { eventStream { id } }`},
		{"workflowEvents", `subscription S { workflowEvents { id } }`},
		{"debugStepEvent", `subscription S { debugStepEvent(sessionId: "d") { id } }`},
	} {
		allowed := integrationSessionStreamOperationAllowed(streamOperation(t, document.query))
		if allowed != hasOperationRole(integrationSessionStreamRoots, document.root) {
			t.Errorf("stream admits %s = %v, but the allowlist says %v", document.root, allowed, !allowed)
		}
	}

	// ...and a change to that one variable moves both the transport and the
	// status report together.
	original := integrationSessionStreamRoots
	t.Cleanup(func() { integrationSessionStreamRoots = original })
	integrationSessionStreamRoots = []string{"sessionRunEvents"}
	if got := streamSubscriptions(grant, true); !reflect.DeepEqual(got, []string{"sessionRunEvents"}) {
		t.Fatalf("subscriptions did not follow the transport allowlist: %v", got)
	}
	if integrationSessionStreamOperationAllowed(streamOperation(t, `subscription S { integrationSessionEvents(sessionId: "s") { id } }`)) {
		t.Fatal("the transport still admits a root removed from the allowlist")
	}
}

func streamOperation(t *testing.T, query string) *gqlgengraphql.OperationContext {
	t.Helper()
	document, err := parser.ParseQuery(&ast.Source{Input: query})
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	return &gqlgengraphql.OperationContext{Doc: document, OperationName: "S"}
}

func TestOperatorControlPlaneRolesAreTheServiceConstants(t *testing.T) {
	want := []string{operator.ReadRole, delivery.OperatorRole, operator.DeploymentOperatorRole}
	if got := OperatorControlPlaneRoles(); !reflect.DeepEqual(got, want) {
		t.Fatalf("OperatorControlPlaneRoles() = %v, want %v", got, want)
	}
}

func TestTransportGrantWithoutControlPlaneRole(t *testing.T) {
	tests := []struct {
		name              string
		roles             []string
		wantMisconfigured bool
		missing           []string
	}{
		{name: "production identity", roles: productionRoles, wantMisconfigured: true,
			missing: []string{operator.ReadRole, delivery.OperatorRole, operator.DeploymentOperatorRole}},
		{name: "grant plus the recovery role but not the read role", roles: []string{GraphQLOperatorRole, delivery.OperatorRole}, wantMisconfigured: true,
			missing: []string{operator.ReadRole, operator.DeploymentOperatorRole}},
		{name: "full bundle", roles: operatorBundle},
		{name: "grant plus read role only (least privilege, not a misconfiguration)", roles: []string{GraphQLOperatorRole, operator.ReadRole}},
		{name: "no grant at all", roles: []string{previewRole}},
		{name: "service bearer", roles: []string{operator.ReadRole, previewRole}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missing, misconfigured := TransportGrantWithoutControlPlaneRole(tt.roles)
			if misconfigured != tt.wantMisconfigured || !reflect.DeepEqual(missing, tt.missing) {
				t.Fatalf("= (%v, %v), want (%v, %v)", missing, misconfigured, tt.missing, tt.wantMisconfigured)
			}
		})
	}
}

// TestAuthCapabilityFieldNamesAreSchemaRoots keeps the representative field
// names honest against the executed schema, not just the role map.
func TestAuthCapabilityFieldNamesAreSchemaRoots(t *testing.T) {
	roots := map[ast.Operation]*ast.Definition{
		ast.Query:        parsedSchema.Query,
		ast.Mutation:     parsedSchema.Mutation,
		ast.Subscription: parsedSchema.Subscription,
	}
	check := func(operation ast.Operation, field string) {
		if roots[operation] == nil || roots[operation].Fields.ForName(field) == nil {
			t.Errorf("%s.%s is not a root field of the executed schema", strings.ToLower(string(operation)), field)
		}
	}
	for _, capability := range []roleCapability{operatorReadCapability, operatorDeliveryCapability, operatorDeploymentCapability, clinicalReadCapability} {
		check(capability.operation, capability.field)
	}
	for _, root := range integrationSessionStreamRoots {
		check(ast.Subscription, root)
	}
}
