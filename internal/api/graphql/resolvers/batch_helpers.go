package resolvers

import (
	"context"
	"sync"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
)

func (r *mutationResolver) processBatchMessages(ctx context.Context, messages []model.BatchMessageItem, stopOnError, parallel bool) []model.BatchItemResult {
	results := make([]model.BatchItemResult, len(messages))

	processOne := func(i int, msg model.BatchMessageItem) model.BatchItemResult {
		index := i
		if msg.Index != nil {
			index = *msg.Index
		}

		input := model.SubmitMessageInput{
			Format:        msg.Format,
			Data:          msg.Data,
			Source:        msg.Source,
			CorrelationID: msg.CorrelationID,
		}

		submitResult, err := r.SubmitMessage(ctx, input)

		result := model.BatchItemResult{
			Index:    index,
			Warnings: []model.ParseWarning{},
			Errors:   []string{},
		}

		if err != nil {
			result.Success = false
			result.Errors = []string{err.Error()}
		} else {
			result.Success = submitResult.Success
			result.EventID = submitResult.EventID
			result.Warnings = submitResult.Warnings
			result.Errors = submitResult.Errors
			result.WorkflowResults = submitResult.WorkflowResults
		}

		return result
	}

	if parallel {
		var wg sync.WaitGroup
		for i, msg := range messages {
			wg.Add(1)
			go func(idx int, m model.BatchMessageItem) {
				defer wg.Done()
				results[idx] = processOne(idx, m)
			}(i, msg)
		}
		wg.Wait()
		return results
	}

	for i, msg := range messages {
		results[i] = processOne(i, msg)
		if stopOnError && !results[i].Success {
			return results[:i+1]
		}
	}

	return results
}

func (r *mutationResolver) processBatchEvents(ctx context.Context, events []model.BatchEventItem, baseIndex int, stopOnError, parallel bool) []model.BatchItemResult {
	results := make([]model.BatchItemResult, len(events))

	processOne := func(i int, evt model.BatchEventItem) model.BatchItemResult {
		index := baseIndex + i
		if evt.Index != nil {
			index = *evt.Index
		}

		input := model.SubmitEventInput{
			Type:          evt.Type,
			Data:          evt.Data,
			Source:        evt.Source,
			CorrelationID: evt.CorrelationID,
		}

		submitResult, err := r.SubmitEvent(ctx, input)

		result := model.BatchItemResult{
			Index:    index,
			Warnings: []model.ParseWarning{},
			Errors:   []string{},
		}

		if err != nil {
			result.Success = false
			result.Errors = []string{err.Error()}
		} else {
			result.Success = submitResult.Success
			result.EventID = submitResult.EventID
			result.Warnings = submitResult.Warnings
			result.Errors = submitResult.Errors
			result.WorkflowResults = submitResult.WorkflowResults
		}

		return result
	}

	if parallel {
		var wg sync.WaitGroup
		for i, evt := range events {
			wg.Add(1)
			go func(idx int, e model.BatchEventItem) {
				defer wg.Done()
				results[idx] = processOne(idx, e)
			}(i, evt)
		}
		wg.Wait()
		return results
	}

	for i, evt := range events {
		results[i] = processOne(i, evt)
		if stopOnError && !results[i].Success {
			return results[:i+1]
		}
	}

	return results
}
