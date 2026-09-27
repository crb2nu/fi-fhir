package graphql

import (
	"context"
	"reflect"
	"testing"

	"github.com/vektah/gqlparser/v2/gqlerror"
)

// TestPresenterKeepsOnlyTheSourceUnavailableCode: the capture-source refusal
// keeps its message and exactly one extension, the fixed SOURCE_UNAVAILABLE
// code — whatever a resolver attached is dropped.
func TestPresenterKeepsOnlyTheSourceUnavailableCode(t *testing.T) {
	presented := catalogSafeErrorPresenter(context.Background(), &gqlerror.Error{
		Message:    ConnectionCaptureSourceUnavailableMessage,
		Extensions: map[string]any{"code": "SOMETHING_ELSE", "sourceId": "adt-east", "inventory": []string{"a"}},
	})
	if presented.Message != ConnectionCaptureSourceUnavailableMessage ||
		!reflect.DeepEqual(presented.Extensions, map[string]any{"code": "SOURCE_UNAVAILABLE"}) {
		t.Fatalf("presented = %q %v", presented.Message, presented.Extensions)
	}
}
