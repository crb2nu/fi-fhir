package resolvers

import (
	"context"
	"errors"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	enginesession "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
)

func (s *integrationSessionService) listSessionSummaries(ctx context.Context, input *model.IntegrationSessionSummaryInput) (*model.IntegrationSessionSummaryPage, error) {
	opts := enginesession.SessionSummaryOptions{Limit: enginesession.DefaultSessionSummaryLimit}
	if input != nil {
		opts.IncludeArchived = input.IncludeArchived
		opts.HasRuns = input.HasRuns
		opts.Limit = input.Limit
		opts.Offset = input.Offset
		if input.Search != nil {
			opts.Search = *input.Search
		}
	}
	page, err := s.store.ListSessionSummaries(ctx, opts)
	if errors.Is(err, enginesession.ErrInvalid) {
		return nil, errors.New("invalid integration session summary request")
	}
	if err != nil {
		return nil, err
	}
	out := &model.IntegrationSessionSummaryPage{
		Nodes:   make([]model.IntegrationSessionSummary, 0, len(page.Nodes)),
		HasMore: page.HasMore, NextOffset: page.NextOffset,
	}
	for _, node := range page.Nodes {
		row := model.IntegrationSessionSummary{
			ID: node.ID, Name: node.Name, Archived: node.Archived,
			CreatedAt: node.CreatedAt, UpdatedAt: node.UpdatedAt,
		}
		if node.LatestRun != nil {
			row.LatestRun = &model.IntegrationSessionRunSummary{
				ID: node.LatestRun.ID, Status: toGraphQLRunStatus(node.LatestRun.Status), CreatedAt: node.LatestRun.CreatedAt,
			}
		}
		out.Nodes = append(out.Nodes, row)
	}
	return out, nil
}
