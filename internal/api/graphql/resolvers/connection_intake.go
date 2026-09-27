package resolvers

import (
	"context"
	"errors"
	"strings"

	"github.com/vektah/gqlparser/v2/gqlerror"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	enginesession "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
)

// Sample intake from connections (.loom/38 C-2): the batch peek and the stream
// capture ride the catalog service, which re-checks integration.operator and
// requires a configured session workspace. Like connection_catalog.go, the
// resolvers add nothing to what the service returns and map every failure to a
// stable, inventory-safe message.

// ErrConnectionIntakeUnavailable is the answer when the catalog exists but no
// session workspace does: there is nowhere to put a sample. It is the GraphQL
// face of capabilities.integrationSessions being false.
var ErrConnectionIntakeUnavailable = errors.New("connection sample intake unavailable")

// intakeConnectionError maps intake failures onto stable messages. The
// catalog's own failures keep connection_catalog.go's wording.
func intakeConnectionError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, connection.ErrSessionsUnavailable):
		return ErrConnectionIntakeUnavailable
	case errors.Is(err, connection.ErrSessionNotFound):
		return errors.New("integration session not found")
	case errors.Is(err, connection.ErrSessionArchived):
		return errors.New("integration session is archived")
	case errors.Is(err, connection.ErrCaptureNotFound):
		return errors.New("connection capture not found")
	case errors.Is(err, connection.ErrCaptureConflict):
		return errors.New("a capture is already armed for this source")
	case errors.Is(err, connection.ErrCaptureFinished):
		return errors.New("connection capture is already finished")
	case errors.Is(err, connection.ErrPeekUnsupported):
		return errors.New("peek requires a compiled batch source connection")
	case errors.Is(err, connection.ErrSourceUnavailable):
		// The one intake refusal with a code: C-3 names the fix (mount or
		// compile the source, or peek a batch one) from SOURCE_UNAVAILABLE.
		return &gqlerror.Error{
			Message:    graphqlapi.ConnectionCaptureSourceUnavailableMessage,
			Extensions: map[string]any{"code": connection.CodeSourceUnavailable},
		}
	case errors.Is(err, connection.ErrInvalidRequest):
		return errors.New("invalid connection sample intake request")
	case errors.Is(err, connection.ErrUnauthenticated), errors.Is(err, connection.ErrForbidden),
		errors.Is(err, connection.ErrNotFound), errors.Is(err, connection.ErrArchived),
		errors.Is(err, connection.ErrUnavailable), errors.Is(err, ErrConnectionCatalogUnavailable):
		return catalogConnectionError(err)
	default:
		return errors.New("connection sample intake request failed")
	}
}

// sessionSampleRedactedPayload is SessionSample.redactedPayload: a captured
// sample's text, returned only to a caller whose verified roles include
// integration.operator — the grant that could have peeked or captured it.
// Session reads are gated by the legacy graphql:operator role, which does not
// imply that grant, so every other caller reads null. Roles come from the
// server-owned security context, never from the request.
func (r *Resolver) sessionSampleRedactedPayload(ctx context.Context, obj *model.SessionSample) (*string, error) {
	if obj == nil || obj.CapturedText == nil {
		return nil, nil
	}
	security, authenticated := requestsecurity.SecurityContextFromContext(ctx)
	if !authenticated || !enginesession.HasRole(security.Principal.Roles, connection.ReadRole) {
		return nil, nil
	}
	text := *obj.CapturedText
	return &text, nil
}

// optionalBound reads an optional count argument: absent is the service's
// default (0), and a present value below one is refused here rather than read
// as "absent".
func optionalBound(value *int) (int, bool) {
	if value == nil {
		return 0, true
	}
	return *value, *value >= 1
}

func (r *queryResolver) connectionCaptures(ctx context.Context, sessionID string) ([]model.ConnectionCapture, error) {
	service, err := r.connectionService()
	if err != nil {
		return nil, intakeConnectionError(err)
	}
	captures, err := service.ListCaptures(ctx, sessionID)
	if err != nil {
		return nil, intakeConnectionError(err)
	}
	projected := make([]model.ConnectionCapture, 0, len(captures))
	for _, capture := range captures {
		projected = append(projected, projectConnectionCapture(capture))
	}
	return projected, nil
}

func (r *mutationResolver) peekBatchConnection(ctx context.Context, input model.PeekBatchConnectionInput) (*model.BatchPeekResult, error) {
	service, err := r.connectionService()
	if err != nil {
		return nil, intakeConnectionError(err)
	}
	maxObjects, objectsOK := optionalBound(input.MaxObjects)
	maxMessages, messagesOK := optionalBound(input.MaxMessages)
	if !objectsOK || !messagesOK {
		return nil, intakeConnectionError(connection.ErrInvalidRequest)
	}
	result, err := service.PeekBatch(ctx, connection.PeekRequest{
		ConnectionID: input.ConnectionID,
		SessionID:    input.SessionID,
		ObjectPath:   optionalStringValue(input.ObjectPath),
		MaxObjects:   maxObjects,
		MaxMessages:  maxMessages,
		Reason:       input.Reason,
	})
	if err != nil {
		return nil, intakeConnectionError(err)
	}
	objects := make([]model.BatchPeekObject, 0, len(result.Objects))
	for _, object := range result.Objects {
		projected := model.BatchPeekObject{Path: object.Path, Size: int(object.Size), Version: object.Version}
		if !object.ModifiedAt.IsZero() {
			modified := object.ModifiedAt.UTC()
			projected.ModifiedAt = &modified
		}
		objects = append(objects, projected)
	}
	samples := make([]model.SessionSample, 0, len(result.Samples))
	for _, sample := range result.Samples {
		samples = append(samples, *r.integrationSessions.toGraphQLSample(sample))
	}
	capture := projectConnectionCapture(result.Capture)
	return &model.BatchPeekResult{
		Objects:  objects,
		Samples:  samples,
		Capture:  &capture,
		Problems: projectConnectionProblems(result.Problems),
	}, nil
}

func (r *mutationResolver) startConnectionCapture(ctx context.Context, input model.StartConnectionCaptureInput) (*model.ConnectionCapture, error) {
	service, err := r.connectionService()
	if err != nil {
		return nil, intakeConnectionError(err)
	}
	maxMessages, messagesOK := optionalBound(input.MaxMessages)
	ttlSeconds, ttlOK := optionalBound(input.TTLSeconds)
	if !messagesOK || !ttlOK {
		return nil, intakeConnectionError(connection.ErrInvalidRequest)
	}
	capture, err := service.StartCapture(ctx, connection.StartCaptureRequest{
		SourceID:    input.SourceID,
		SessionID:   input.SessionID,
		MaxMessages: maxMessages,
		TTLSeconds:  ttlSeconds,
		Reason:      input.Reason,
	})
	if err != nil {
		return nil, intakeConnectionError(err)
	}
	projected := projectConnectionCapture(capture)
	return &projected, nil
}

func (r *mutationResolver) cancelConnectionCapture(ctx context.Context, id, reason string) (*model.ConnectionCapture, error) {
	service, err := r.connectionService()
	if err != nil {
		return nil, intakeConnectionError(err)
	}
	capture, err := service.CancelCapture(ctx, id, reason)
	if err != nil {
		return nil, intakeConnectionError(err)
	}
	projected := projectConnectionCapture(capture)
	return &projected, nil
}

func projectConnectionCapture(capture connection.Capture) model.ConnectionCapture {
	projected := model.ConnectionCapture{
		ID:          capture.ID,
		SessionID:   capture.SessionID,
		SourceID:    capture.SourceID,
		Mode:        model.ConnectionCaptureMode(strings.ToUpper(string(capture.Mode))),
		Status:      model.ConnectionCaptureStatus(strings.ToUpper(string(capture.Status))),
		Captured:    capture.Captured,
		MaxMessages: capture.MaxMessages,
		ExpiresAt:   capture.ExpiresAt.UTC(),
		RequestedBy: projectAuditPrincipal(capture.RequestedBy),
		Reason:      capture.Reason,
		RequestedAt: capture.RequestedAt.UTC(),
		Problems:    projectConnectionProblems(capture.Problems),
	}
	if capture.ConnectionArtifactID != "" {
		connectionID := capture.ConnectionArtifactID
		projected.ConnectionID = &connectionID
	}
	if capture.CompletedAt != nil {
		completed := capture.CompletedAt.UTC()
		projected.CompletedAt = &completed
	}
	return projected
}
