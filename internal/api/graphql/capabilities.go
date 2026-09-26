package graphql

import (
	"github.com/vektah/gqlparser/v2/ast"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// The auth capabilities contract (IDE repair, Lane R-A).
//
// Between 2026-09-05 and 2026-09-25 nobody in production could reach the
// operator control plane. The transport gate admitted every caller's
// graphql:operator compatibility grant, operator.Service then refused them for
// lacking integration.operator, and /api/auth/status said only
// {"authenticated":true,"authVia":"network"}. The IDE had no way to tell a
// deployment whose identity lacked the control-plane role from one that held
// it, so every surface discovered its failure by failing.
//
// This file derives what the caller can actually do from the two inputs the
// server already trusts: the SecurityContext the transport gate authorizes
// against, and the server's own configuration. It reads nothing else — no
// store, no resolver, no request argument — and it reports role names and
// booleans only, never a token, tenant, hostname, or message-derived value.

// authStatus is the /api/auth/status body for an authenticated caller. An
// unauthenticated caller keeps the pre-contract body {"authenticated":false}.
type authStatus struct {
	Authenticated bool               `json:"authenticated"`
	AuthVia       string             `json:"authVia"`
	Principal     string             `json:"principal"`
	Roles         []string           `json:"roles"`
	Capabilities  accessCapabilities `json:"capabilities"`
	MissingRoles  missingRoles       `json:"missingRoles"`
}

// accessCapabilities has two halves. The four role capabilities describe the
// caller. integrationSessions, streaming, subscriptions, and llm describe the
// deployment: they are the same for every caller except that subscriptions is
// filtered by what this caller's roles clear at the transport gate.
type accessCapabilities struct {
	OperatorRead        bool     `json:"operatorRead"`
	OperatorDelivery    bool     `json:"operatorDelivery"`
	OperatorDeployment  bool     `json:"operatorDeployment"`
	ClinicalRead        bool     `json:"clinicalRead"`
	IntegrationSessions bool     `json:"integrationSessions"`
	Streaming           bool     `json:"streaming"`
	Subscriptions       []string `json:"subscriptions"`
	LLM                 llmState `json:"llm"`
}

type llmState struct {
	Configured bool `json:"configured"`
}

// missingRoles lists, per role capability, exactly the role strings the caller
// would have to be granted for that capability to become true. A capability
// that is already true has an empty list.
type missingRoles struct {
	OperatorRead       []string `json:"operatorRead"`
	OperatorDelivery   []string `json:"operatorDelivery"`
	OperatorDeployment []string `json:"operatorDeployment"`
	ClinicalRead       []string `json:"clinicalRead"`
}

// roleCapability is one IDE surface expressed as the two gates a request to it
// must clear.
//
// The transport half is a representative root field: its rootFieldRoles entry
// is the requirement, evaluated by transportGateAdmitsField with the same
// precedence MutateOperationContext uses. TestAuthCapabilityRepresentativesCoverTheirGroup
// pins that every field of the surface shares that entry.
//
// The service half is what the service answering the field re-checks after the
// gate. For the operator plane it is operator.Service.authorize's role list,
// which the rootFieldRoles values operatorRead/operatorRecovery/operatorDeployment
// already name from the operator and delivery packages' own constants. It is
// nil when the transport gate is the only gate, as it is for clinical reads.
//
// The split is the whole point: the graphql:operator grant satisfies the
// transport half of every operator field and neither the service half.
type roleCapability struct {
	operation    ast.Operation
	field        string
	serviceRoles []string
}

var (
	operatorReadCapability       = roleCapability{operation: ast.Query, field: "operatorReceipts", serviceRoles: operatorRead}
	operatorDeliveryCapability   = roleCapability{operation: ast.Mutation, field: "replayDelivery", serviceRoles: operatorRecovery}
	operatorDeploymentCapability = roleCapability{operation: ast.Mutation, field: "deployIntegrationRelease", serviceRoles: operatorDeployment}
	clinicalReadCapability       = roleCapability{operation: ast.Query, field: "events"}
)

// evaluate reports whether roles clear both gates and, if not, the roles that
// are missing: the transport requirement's unheld roles when the gate refuses,
// followed by the service requirement's unheld roles, in declaration order and
// without duplicates.
func (c roleCapability) evaluate(roles []string) (bool, []string) {
	admitted := transportGateAdmitsField(roles, c.operation, c.field)
	missing := make([]string, 0, 2)
	if !admitted {
		missing = appendUnheld(missing, roles, rootFieldRoles[c.operation][c.field])
	}
	missing = appendUnheld(missing, roles, c.serviceRoles)
	return admitted && len(missing) == 0, missing
}

// transportGateAdmitsField answers, for one root field, the question
// MutateOperationContext answers for a whole operation: the negative-control
// blanket allow, then the named compatibility grant, then the field's own
// AND-set. An unmapped field is refused, as the gate refuses it.
func transportGateAdmitsField(roles []string, operation ast.Operation, field string) bool {
	if transportGateBlanketAllow() || hasOperationRole(roles, GraphQLOperatorRole) {
		return true
	}
	required, mapped := rootFieldRoles[operation][field]
	if !mapped || len(required) == 0 {
		return false
	}
	return hasEveryRole(roles, required)
}

// deriveAuthStatus builds the authenticated status body from the caller's
// verified identity and the server's configuration, and from nothing else.
func deriveAuthStatus(security integration.SecurityContext, config *ServerConfig) authStatus {
	roles := append([]string{}, security.Principal.Roles...)
	status := authStatus{
		Authenticated: true,
		AuthVia:       security.Principal.AuthMethod,
		Principal:     security.Principal.ID,
		Roles:         roles,
	}
	status.Capabilities.OperatorRead, status.MissingRoles.OperatorRead = operatorReadCapability.evaluate(roles)
	status.Capabilities.OperatorDelivery, status.MissingRoles.OperatorDelivery = operatorDeliveryCapability.evaluate(roles)
	status.Capabilities.OperatorDeployment, status.MissingRoles.OperatorDeployment = operatorDeploymentCapability.evaluate(roles)
	status.Capabilities.ClinicalRead, status.MissingRoles.ClinicalRead = clinicalReadCapability.evaluate(roles)

	status.Capabilities.IntegrationSessions = config.IntegrationSessionsConfigured
	status.Capabilities.Streaming = config.IntegrationSessionStreaming
	status.Capabilities.Subscriptions = streamSubscriptions(roles, config.IntegrationSessionStreaming)
	status.Capabilities.LLM.Configured = config.LLMConfigured
	return status
}

// streamSubscriptions is the list of subscription root fields the SSE
// transport will accept for this caller: the transport's own allowlist,
// filtered by the transport gate, and empty when streaming is off.
func streamSubscriptions(roles []string, streaming bool) []string {
	accepted := make([]string, 0, len(integrationSessionStreamRoots))
	if !streaming {
		return accepted
	}
	for _, root := range integrationSessionStreamRoots {
		if transportGateAdmitsField(roles, ast.Subscription, root) {
			accepted = append(accepted, root)
		}
	}
	return accepted
}

// OperatorControlPlaneRoles is the service-role half of the operator bundle:
// every role operator.Service checks, in the order an operator is granted them.
// A deployment's operator identity needs these beside graphql:operator; the
// compatibility grant does not imply them (decision 2026-09-25, "Grant the
// operator bundle rather than alias the transport grant").
func OperatorControlPlaneRoles() []string {
	var bundle []string
	for _, required := range [][]string{operatorRead, operatorRecovery, operatorDeployment} {
		bundle = appendUnheld(bundle, nil, required)
	}
	return bundle
}

// TransportGrantWithoutControlPlaneRole reports whether roles carry the
// graphql:operator compatibility grant but cannot read the operator control
// plane — the exact misconfiguration that made production's operator page
// forbidden — and, when they do, which operator control-plane roles are
// absent. It is the startup-time twin of the operatorRead capability.
func TransportGrantWithoutControlPlaneRole(roles []string) ([]string, bool) {
	if !hasOperationRole(roles, GraphQLOperatorRole) {
		return nil, false
	}
	if readable, _ := operatorReadCapability.evaluate(roles); readable {
		return nil, false
	}
	return appendUnheld(nil, roles, OperatorControlPlaneRoles()), true
}

func hasEveryRole(roles, required []string) bool {
	for _, role := range required {
		if !hasOperationRole(roles, role) {
			return false
		}
	}
	return true
}

// appendUnheld appends each role in wanted that roles does not hold and that
// out does not already contain.
func appendUnheld(out, roles, wanted []string) []string {
	for _, role := range wanted {
		if hasOperationRole(roles, role) || hasOperationRole(out, role) {
			continue
		}
		out = append(out, role)
	}
	return out
}
