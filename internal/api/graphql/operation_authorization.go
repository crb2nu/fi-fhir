package graphql

import (
	"context"
	"log/slog"
	"sort"

	gqlgengraphql "github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/errcode"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
)

const (
	// GraphQLOperatorRole is the named compatibility grant. It expands to every
	// root field, so a token that carries it behaves exactly as it did before
	// Lane S4-E narrowed the gate. Per-root-field roles live in
	// operation_authorization_roles.go; prefer those for new tokens.
	GraphQLOperatorRole = "graphql:operator"
	clinicalReadRole    = "clinical:read"
	previewRole         = "integration:preview"
)

// operationAuthorization is the transport gate. It carries a logger because of
// the gap Lane S4-E left open and Sprint 5's Found Defects named: 115 of the
// 131 root fields are reachable *only* through the `graphql:operator`
// compatibility grant, and nothing recorded when a request used it. A grant
// that expands to everything and leaves no trace cannot be narrowed later,
// because nobody can tell which fields are actually in use.
//
// Slice 4.4d adds the trace and nothing else. The role mapping is unchanged;
// no field moves from the grant to a fine-grained role this sprint.
type operationAuthorization struct {
	logger *slog.Logger
}

type integrationSessionStreamContextKey struct{}

var _ interface {
	gqlgengraphql.HandlerExtension
	gqlgengraphql.OperationContextMutator
} = operationAuthorization{}

func (operationAuthorization) ExtensionName() string {
	return "OperationAuthorization"
}

func (operationAuthorization) Validate(gqlgengraphql.ExecutableSchema) error {
	return nil
}

func (a operationAuthorization) MutateOperationContext(ctx context.Context, operationContext *gqlgengraphql.OperationContext) *gqlerror.Error {
	security, authenticated := requestsecurity.SecurityContextFromContext(ctx)
	if !authenticated {
		authError := gqlerror.Errorf("authentication required")
		errcode.Set(authError, "UNAUTHENTICATED")
		return authError
	}
	if isIntegrationSessionStream(ctx) && !integrationSessionStreamOperationAllowed(operationContext) {
		forbiddenError := gqlerror.Errorf("GraphQL stream operation forbidden")
		errcode.Set(forbiddenError, "FORBIDDEN")
		return forbiddenError
	}
	// Negative control only: the transportgateblanket build tag restores the
	// pre-Sprint-4 allow-everything gate so the kill-test can be proven to fail
	// open. False in every shipped build.
	if transportGateBlanketAllow() {
		return nil
	}
	// The named compatibility grant expands to the full root-field set, so every
	// operator token minted from the docs keeps working unchanged.
	if hasOperationRole(security.Principal.Roles, GraphQLOperatorRole) {
		a.recordCompatibilityGrant(ctx, security.Principal.ID, operationContext)
		return nil
	}
	if hasOperationRole(security.Principal.Roles, previewRole) && previewOperationAllowed(operationContext) {
		return nil
	}
	// Default-deny: transportGateRolesSatisfied refuses any root field that
	// rootFieldRoles does not name.
	if transportGateRolesSatisfied(security.Principal.Roles, operationContext) {
		return nil
	}
	forbiddenError := gqlerror.Errorf("GraphQL operation forbidden")
	errcode.Set(forbiddenError, "FORBIDDEN")
	return forbiddenError
}

// recordCompatibilityGrant emits exactly one line per admission that used the
// compatibility grant.
//
// What it carries: the root field the request asked for, the principal that
// asked, and the grant name. What it deliberately does not carry: the bearer
// token, the principal's other roles, the operation's variables, and anything
// else request-derived. A field name is a schema constant and a principal ID is
// an operator identity; neither is message content.
//
// A request admitted through a fine-grained role emits nothing, so the volume
// of these lines is the size of the migration still owed.
func (a operationAuthorization) recordCompatibilityGrant(ctx context.Context, principalID string, operationContext *gqlgengraphql.OperationContext) {
	if a.logger == nil {
		return
	}
	fields := compatibilityGrantFields(operationContext)
	if len(fields) == 0 {
		return
	}
	for _, field := range fields {
		a.logger.InfoContext(ctx, "GraphQL request admitted through the compatibility grant",
			observability.F(observability.FieldComponent, "transport-gate"),
			observability.F(observability.FieldGrant, GraphQLOperatorRole),
			observability.F(observability.FieldField, field),
			observability.F(observability.FieldPrincipalID, principalID))
	}
}

// compatibilityGrantFields returns the sorted, de-duplicated root field names
// of the admitted operation. Sorting keeps a multi-field operation's lines
// stable across runs so a log-volume comparison is a comparison.
func compatibilityGrantFields(operationContext *gqlgengraphql.OperationContext) []string {
	if operationContext == nil || operationContext.Doc == nil {
		return nil
	}
	operation := operationContext.Doc.Operations.ForName(operationContext.OperationName)
	if operation == nil {
		return nil
	}
	seen := make(map[string]struct{})
	unique := make([]string, 0, 2)
	for _, field := range rootFieldNames(operation.SelectionSet, make(map[*ast.FragmentDefinition]bool)) {
		if field == "__typename" {
			continue
		}
		if _, duplicate := seen[field]; duplicate {
			continue
		}
		seen[field] = struct{}{}
		unique = append(unique, field)
	}
	sort.Strings(unique)
	return unique
}

func withIntegrationSessionStream(ctx context.Context) context.Context {
	return context.WithValue(ctx, integrationSessionStreamContextKey{}, true)
}

func isIntegrationSessionStream(ctx context.Context) bool {
	enabled, _ := ctx.Value(integrationSessionStreamContextKey{}).(bool)
	return enabled
}

func integrationSessionStreamOperationAllowed(operationContext *gqlgengraphql.OperationContext) bool {
	if operationContext == nil || operationContext.Doc == nil {
		return false
	}
	operation := operationContext.Doc.Operations.ForName(operationContext.OperationName)
	if operation == nil || operation.Operation != ast.Subscription {
		return false
	}
	fields := rootFieldNames(operation.SelectionSet, make(map[*ast.FragmentDefinition]bool))
	if len(fields) == 0 {
		return false
	}
	for _, field := range fields {
		if field != "integrationSessionEvents" && field != "sessionRunEvents" && field != "__typename" {
			return false
		}
	}
	return true
}

func previewOperationAllowed(operationContext *gqlgengraphql.OperationContext) bool {
	if operationContext == nil || operationContext.Doc == nil {
		return false
	}
	operation := operationContext.Doc.Operations.ForName(operationContext.OperationName)
	if operation == nil {
		return false
	}
	fields := rootFieldNames(operation.SelectionSet, make(map[*ast.FragmentDefinition]bool))
	if len(fields) == 0 {
		return false
	}
	for _, field := range fields {
		switch operation.Operation {
		case ast.Query:
			if field != "health" && field != "__typename" {
				return false
			}
		case ast.Mutation:
			if field != "previewIntegrationMessage" && field != "__typename" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func rootFieldNames(selections ast.SelectionSet, activeFragments map[*ast.FragmentDefinition]bool) []string {
	fields := make([]string, 0, len(selections))
	for _, selection := range selections {
		switch selected := selection.(type) {
		case *ast.Field:
			fields = append(fields, selected.Name)
		case *ast.InlineFragment:
			fields = append(fields, rootFieldNames(selected.SelectionSet, activeFragments)...)
		case *ast.FragmentSpread:
			fragment := selected.Definition
			if fragment == nil || activeFragments[fragment] {
				continue
			}
			activeFragments[fragment] = true
			fields = append(fields, rootFieldNames(fragment.SelectionSet, activeFragments)...)
			delete(activeFragments, fragment)
		}
	}
	return fields
}

func hasOperationRole(roles []string, wanted string) bool {
	for _, role := range roles {
		if role == wanted {
			return true
		}
	}
	return false
}
