package ingest

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/google/uuid"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/ingest/providers"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/storage"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	temporalWorkflow "go.temporal.io/sdk/workflow"
)

// PollerWorkflowInput contains configuration for the ingest poll job.
type PollerWorkflowInput struct {
	SourceID        string
	ProviderType    string
	PrefixOrBaseDir string
}

// BatchDiscoveredPayload represents the payload sent to the generic workflow engine after successful ingest.
type BatchDiscoveredPayload struct {
	SourceID     string             `json:"source_id"`
	OriginalPath string             `json:"original_path"`
	ObjectURI    string             `json:"object_uri"`
	FileInfo     providers.FileInfo `json:"file_info"`
}

// BatchDiscoveredEvent is the wrapper type dispatched into the fi-fhir engine.
type BatchDiscoveredEvent struct {
	events.EventMeta
	Payload BatchDiscoveredPayload `json:"payload"`
}

// IngestionActivities holds the dependencies needed to execute ingestion tasks.
type IngestionActivities struct {
	Engine      *workflow.Engine
	ObjectStore *storage.MinIOProvider
	Providers   map[string]providers.Provider // "s3" -> S3Provider, "sftp" -> SFTPProvider
}

// PollAndIngestActivity checks the provider for new files, uploads them to internal storage, and dispatches an event.
func (a *IngestionActivities) PollAndIngestActivity(ctx context.Context, input PollerWorkflowInput) (int, error) {
	provider, ok := a.Providers[input.ProviderType]
	if !ok {
		return 0, fmt.Errorf("unknown provider type: %s", input.ProviderType)
	}

	files, err := provider.ListFiles(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list files from provider: %w", err)
	}

	if len(files) == 0 {
		return 0, nil // Nothing to do
	}

	processedCount := 0

	for _, fi := range files {
		// 1. Download from external provider
		fileStream, err := provider.DownloadFile(ctx, fi.Path)
		if err != nil {
			activity.GetLogger(ctx).Error("Failed to download file", "path", fi.Path, "error", err)
			continue
		}

		// 2. Upload to internal Object Storage (MinIO)
		internalKey := path.Join("ingest", input.SourceID, time.Now().UTC().Format("2006/01/02"), uuid.New().String(), path.Base(fi.Path))
		internalPath := "s3://" + a.ObjectStore.Client().EndpointURL().Host + "/" + internalKey
		// Note: MinIOProvider expects paths sometimes as bucket/key or assumes a default bucket.
		// For simplicity, providing a direct path that map to standard Fi-Fhir storage key.

		err = a.ObjectStore.Put(ctx, internalKey, fileStream, fi.Size)
		_ = fileStream.Close()

		if err != nil {
			activity.GetLogger(ctx).Error("Failed to upload to internal storage", "path", fi.Path, "error", err)
			continue
		}

		// 3. Dispatch to Workflow Engine
		eventPayload := BatchDiscoveredPayload{
			SourceID:     input.SourceID,
			OriginalPath: fi.Path,
			ObjectURI:    internalPath,
			FileInfo:     fi,
		}

		canonicalEvent := &BatchDiscoveredEvent{
			EventMeta: events.EventMeta{
				ID:         uuid.New().String(),
				Type:       "ingest.batch_discovered",
				Source:     fmt.Sprintf("provider:%s:%s", input.ProviderType, input.SourceID),
				Timestamp:  time.Now().UTC(),
				ReceivedAt: time.Now().UTC(),
			},
			Payload: eventPayload,
		}

		result := a.Engine.ProcessWithContext(ctx, canonicalEvent)
		if result.HasErrors() {
			activity.GetLogger(ctx).Error("Workflow engine processing failed for batch event", "path", fi.Path, "errors", result.AllErrors())
			// Keep file at source on error (Nack)
			_ = provider.Nack(ctx, fi.Path)
			continue
		}

		// 4. Acknowledge external file
		if err := provider.Ack(ctx, fi.Path); err != nil {
			activity.GetLogger(ctx).Error("Failed to ack file on source provider", "path", fi.Path, "error", err)
		}

		processedCount++
	}

	return processedCount, nil
}

// PollerWorkflow is the Temporal Cron Workflow that orchestrates polling schedules.
func PollerWorkflow(ctx temporalWorkflow.Context, input PollerWorkflowInput) error {
	ao := temporalWorkflow.ActivityOptions{
		StartToCloseTimeout: time.Minute * 10, // Downloads might take a while
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second * 15,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute * 5,
			MaximumAttempts:    3,
		},
	}
	ctx = temporalWorkflow.WithActivityOptions(ctx, ao)

	logger := temporalWorkflow.GetLogger(ctx)
	logger.Info("Starting ingest poller workflow", "sourceID", input.SourceID, "providerType", input.ProviderType)

	var activities *IngestionActivities
	var processedCount int

	err := temporalWorkflow.ExecuteActivity(ctx, activities.PollAndIngestActivity, input).Get(ctx, &processedCount)
	if err != nil {
		logger.Error("Poller workflow activity failed", "error", err)
		return err
	}

	logger.Info("Completed ingest poller workflow", "processedCount", processedCount)
	return nil
}
