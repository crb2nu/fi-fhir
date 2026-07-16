package graphql

import (
	"context"

	gqlgengraphql "github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/errcode"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
)

const (
	// GraphQLOperatorRole is the explicit temporary escape hatch for legacy
	// authenticated GraphQL capabilities while operation-level RBAC is built.
	GraphQLOperatorRole = "graphql:operator"
	previewRole         = "integration:preview"
)

type operationAuthorization struct{}

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

func (operationAuthorization) MutateOperationContext(ctx context.Context, operationContext *gqlgengraphql.OperationContext) *gqlerror.Error {
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
	if hasOperationRole(security.Principal.Roles, GraphQLOperatorRole) {
		return nil
	}
	if !hasOperationRole(security.Principal.Roles, previewRole) || !previewOperationAllowed(operationContext) {
		forbiddenError := gqlerror.Errorf("GraphQL operation forbidden")
		errcode.Set(forbiddenError, "FORBIDDEN")
		return forbiddenError
	}
	return nil
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
