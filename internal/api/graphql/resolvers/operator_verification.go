package resolvers

import (
	"context"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
)

// Verification reads (.loom/42 E-2) over the operator control plane: the same
// service, role, tenant scope and error catalogue as the operator reads.

func operatorCanonicalEventFilter(filter *model.OperatorCanonicalEventFilter) operator.CanonicalEventFilter {
	out := operator.CanonicalEventFilter{}
	if filter == nil {
		return out
	}
	out.EventType = optionalStringValue(filter.EventType)
	out.DefinitionID = optionalStringValue(filter.DefinitionID)
	out.ReceiptID = optionalStringValue(filter.ReceiptID)
	out.SourceMessageID = optionalStringValue(filter.SourceMessageID)
	out.CorrelationID = optionalStringValue(filter.CorrelationID)
	out.From = filter.From
	out.To = filter.To
	out.IncludePurged = filter.IncludePurged != nil && *filter.IncludePurged
	return out
}

func projectOperatorCanonicalEvent(record operator.CanonicalEventRecord) model.OperatorCanonicalEvent {
	event := projectOperatorEvent(record.Event)
	definition := projectArtifactRevision(record.Definition)
	projected := model.OperatorCanonicalEvent{
		EventID:          event.EventID,
		EventType:        event.EventType,
		SourceMessageID:  event.SourceMessageID,
		CorrelationID:    event.CorrelationID,
		Classification:   event.Classification,
		RecordedAt:       event.RecordedAt,
		ReceiptID:        event.ReceiptID,
		ReceiptStatus:    record.ReceiptStatus,
		Definition:       &definition,
		PayloadFields:    event.PayloadFields,
		PayloadTruncated: event.PayloadTruncated,
		PurgeAfter:       event.PurgeAfter,
		PurgedAt:         event.PurgedAt,
	}
	if record.Source != nil {
		source := projectArtifactRevision(*record.Source)
		projected.Source = &source
	}
	return projected
}

func (r *queryResolver) operatorCanonicalEvents(
	ctx context.Context,
	filter *model.OperatorCanonicalEventFilter,
	page *model.OperatorPageInput,
) (*model.OperatorCanonicalEventConnection, error) {
	service, err := r.operatorService()
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	result, err := service.ListCanonicalEvents(ctx, operatorCanonicalEventFilter(filter), operatorPageRequest(page))
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	nodes := make([]model.OperatorCanonicalEvent, 0, len(result.Items))
	for _, record := range result.Items {
		nodes = append(nodes, projectOperatorCanonicalEvent(record))
	}
	return &model.OperatorCanonicalEventConnection{
		Nodes:    nodes,
		PageInfo: operatorPageInfo(result.NextCursor, result.HasMore),
	}, nil
}

func operatorStatisticsBucket(bucket model.OperatorStatisticsBucket) operator.StatisticsBucket {
	switch bucket {
	case model.OperatorStatisticsBucketHour:
		return operator.StatisticsBucketHour
	case model.OperatorStatisticsBucketDay:
		return operator.StatisticsBucketDay
	default:
		// The service refuses an unknown width as an invalid request.
		return operator.StatisticsBucket(bucket)
	}
}

func projectOperatorStatisticsBucket(bucket operator.StatisticsBucket) model.OperatorStatisticsBucket {
	if bucket == operator.StatisticsBucketDay {
		return model.OperatorStatisticsBucketDay
	}
	return model.OperatorStatisticsBucketHour
}

func projectOperatorAdmissionStatistics(stats operator.AdmissionStatistics) *model.OperatorAdmissionStatistics {
	byType := make([]model.OperatorKeyCount, 0, len(stats.EventsByType))
	for _, group := range stats.EventsByType {
		byType = append(byType, model.OperatorKeyCount{Key: group.Key, Count: group.Count})
	}
	byDefinition := make([]model.OperatorDefinitionAdmissions, 0, len(stats.ReceiptsByDefinition))
	for _, group := range stats.ReceiptsByDefinition {
		byDefinition = append(byDefinition, model.OperatorDefinitionAdmissions{
			DefinitionID: group.DefinitionID,
			RevisionID:   group.RevisionID,
			Accepted:     group.Accepted,
			Rejected:     group.Rejected,
		})
	}
	byDestination := make([]model.OperatorDestinationAttempts, 0, len(stats.AttemptsByDestination))
	for _, group := range stats.AttemptsByDestination {
		byDestination = append(byDestination, model.OperatorDestinationAttempts{
			DestinationArtifactID: group.DestinationArtifactID,
			Queued:                group.Queued,
			Succeeded:             group.Succeeded,
			Failed:                group.Failed,
		})
	}
	series := make([]model.OperatorStatisticsBucketCounts, 0, len(stats.Series))
	for _, bucket := range stats.Series {
		series = append(series, model.OperatorStatisticsBucketCounts{
			Start:     bucket.Start,
			Accepted:  bucket.Accepted,
			Rejected:  bucket.Rejected,
			Queued:    bucket.Queued,
			Succeeded: bucket.Succeeded,
			Failed:    bucket.Failed,
		})
	}
	return &model.OperatorAdmissionStatistics{
		From:                  stats.From,
		To:                    stats.To,
		Bucket:                projectOperatorStatisticsBucket(stats.Bucket),
		AcceptedReceipts:      stats.AcceptedReceipts,
		RejectedReceipts:      stats.RejectedReceipts,
		CanonicalEvents:       stats.CanonicalEvents,
		PurgedEvents:          stats.PurgedEvents,
		ScheduledForPurge:     stats.ScheduledForPurge,
		QueuedAttempts:        stats.QueuedAttempts,
		SucceededAttempts:     stats.SucceededAttempts,
		FailedAttempts:        stats.FailedAttempts,
		EventsByType:          byType,
		ReceiptsByDefinition:  byDefinition,
		AttemptsByDestination: byDestination,
		GroupsTruncated:       stats.GroupsTruncated,
		Series:                series,
	}
}

func (r *queryResolver) operatorAdmissionStatistics(
	ctx context.Context,
	window model.OperatorStatisticsWindow,
	bucket model.OperatorStatisticsBucket,
) (*model.OperatorAdmissionStatistics, error) {
	service, err := r.operatorService()
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	stats, err := service.AdmissionStatistics(ctx, operator.StatisticsRequest{
		From:   window.From,
		To:     window.To,
		Bucket: operatorStatisticsBucket(bucket),
	})
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	return projectOperatorAdmissionStatistics(stats), nil
}
