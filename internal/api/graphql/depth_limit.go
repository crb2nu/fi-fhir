package graphql

import (
	"context"
	"fmt"

	gqlgengraphql "github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/errcode"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const queryDepthLimitExtension = "QueryDepthLimit"

type fixedQueryDepthLimit struct {
	limit int
}

var _ interface {
	gqlgengraphql.HandlerExtension
	gqlgengraphql.OperationContextMutator
} = fixedQueryDepthLimit{}

func (fixedQueryDepthLimit) ExtensionName() string {
	return queryDepthLimitExtension
}

func (limit fixedQueryDepthLimit) Validate(gqlgengraphql.ExecutableSchema) error {
	if limit.limit <= 0 {
		return fmt.Errorf("GraphQL query depth limit must be positive")
	}
	return nil
}

func (limit fixedQueryDepthLimit) MutateOperationContext(_ context.Context, operationContext *gqlgengraphql.OperationContext) *gqlerror.Error {
	if operationContext == nil || operationContext.Doc == nil {
		return gqlerror.Errorf("GraphQL operation is unavailable")
	}
	operation := operationContext.Doc.Operations.ForName(operationContext.OperationName)
	if operation == nil {
		return gqlerror.Errorf("GraphQL operation is unavailable")
	}
	depth := selectionDepth(operation.SelectionSet, 0, make(map[*ast.FragmentDefinition]bool))
	if depth > limit.limit {
		limitError := gqlerror.Errorf("operation has depth %d, which exceeds the limit of %d", depth, limit.limit)
		errcode.Set(limitError, "QUERY_DEPTH_LIMIT_EXCEEDED")
		return limitError
	}
	return nil
}

func selectionDepth(selections ast.SelectionSet, parentDepth int, activeFragments map[*ast.FragmentDefinition]bool) int {
	maximum := parentDepth
	for _, selection := range selections {
		switch selected := selection.(type) {
		case *ast.Field:
			fieldDepth := parentDepth + 1
			if childDepth := selectionDepth(selected.SelectionSet, fieldDepth, activeFragments); childDepth > fieldDepth {
				fieldDepth = childDepth
			}
			if fieldDepth > maximum {
				maximum = fieldDepth
			}
		case *ast.InlineFragment:
			if depth := selectionDepth(selected.SelectionSet, parentDepth, activeFragments); depth > maximum {
				maximum = depth
			}
		case *ast.FragmentSpread:
			fragment := selected.Definition
			if fragment == nil || activeFragments[fragment] {
				continue
			}
			activeFragments[fragment] = true
			depth := selectionDepth(fragment.SelectionSet, parentDepth, activeFragments)
			delete(activeFragments, fragment)
			if depth > maximum {
				maximum = depth
			}
		}
	}
	return maximum
}
