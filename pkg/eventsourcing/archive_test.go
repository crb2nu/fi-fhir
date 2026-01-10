package eventsourcing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRetentionPolicy_GetRetention(t *testing.T) {
	policy := &RetentionPolicy{
		DefaultRetention: 7 * 365 * 24 * time.Hour, // 7 years
		MinRetention:     6 * 365 * 24 * time.Hour, // 6 years
		EventTypeRetention: map[string]time.Duration{
			"audit_log": 3 * 365 * 24 * time.Hour, // 3 years
		},
		StreamPrefixRetention: map[string]time.Duration{
			"patient:": 10 * 365 * 24 * time.Hour, // 10 years for patient data
			"temp:":    1 * 365 * 24 * time.Hour,  // 1 year for temp data
		},
	}

	tests := []struct {
		name      string
		eventType string
		streamID  string
		wantYears int
	}{
		{
			name:      "default retention",
			eventType: "some_event",
			streamID:  "some:stream",
			wantYears: 7,
		},
		{
			name:      "event type override",
			eventType: "audit_log",
			streamID:  "some:stream",
			wantYears: 6, // 3 years requested, but 6 year minimum enforced
		},
		{
			name:      "stream prefix override",
			eventType: "some_event",
			streamID:  "patient:12345",
			wantYears: 10,
		},
		{
			name:      "event type takes precedence over stream",
			eventType: "audit_log",
			streamID:  "patient:12345",
			wantYears: 6, // audit_log = 3 years, but min = 6 years
		},
		{
			name:      "short retention clamped to minimum",
			eventType: "some_event",
			streamID:  "temp:session-123",
			wantYears: 6, // temp = 1 year, but min = 6 years
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := policy.GetRetention(tt.eventType, tt.streamID)
			wantDuration := time.Duration(tt.wantYears) * 365 * 24 * time.Hour

			if got != wantDuration {
				t.Errorf("GetRetention(%q, %q) = %v, want %v",
					tt.eventType, tt.streamID, got, wantDuration)
			}
		})
	}
}

func TestRetentionPolicy_IsEligibleForArchival(t *testing.T) {
	policy := &RetentionPolicy{
		DefaultRetention: 24 * time.Hour, // 1 day for testing
		MinRetention:     1 * time.Hour,  // 1 hour minimum
	}

	now := time.Now()

	tests := []struct {
		name      string
		timestamp time.Time
		want      bool
	}{
		{
			name:      "recent event not eligible",
			timestamp: now.Add(-1 * time.Hour),
			want:      false,
		},
		{
			name:      "old event eligible",
			timestamp: now.Add(-48 * time.Hour),
			want:      true,
		},
		{
			name:      "event just within retention not eligible",
			timestamp: now.Add(-23 * time.Hour), // 23 hours ago, still within 24h retention
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := StoredEvent{
				EventType: "test_event",
				StreamID:  "test:stream",
				Timestamp: tt.timestamp,
			}

			got := policy.IsEligibleForArchival(event)
			if got != tt.want {
				t.Errorf("IsEligibleForArchival() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultRetentionPolicy(t *testing.T) {
	policy := DefaultRetentionPolicy()

	// Check default values
	expectedDefault := 7 * 365 * 24 * time.Hour
	if policy.DefaultRetention != expectedDefault {
		t.Errorf("DefaultRetention = %v, want %v", policy.DefaultRetention, expectedDefault)
	}

	expectedMin := 6 * 365 * 24 * time.Hour
	if policy.MinRetention != expectedMin {
		t.Errorf("MinRetention = %v, want %v", policy.MinRetention, expectedMin)
	}
}

func TestFileArchiveStore(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "events.jsonl")

	// Create archive store
	store, err := NewFileArchiveStore(archivePath)
	if err != nil {
		t.Fatalf("NewFileArchiveStore() error = %v", err)
	}

	// Write events
	events := []StoredEvent{
		{
			Position:      1,
			StreamID:      "test:stream-1",
			StreamVersion: 0,
			EventType:     "test_event",
			Data:          json.RawMessage(`{"key": "value1"}`),
			Metadata:      map[string]string{"source": "test"},
			Timestamp:     time.Now().Add(-72 * time.Hour),
		},
		{
			Position:      2,
			StreamID:      "test:stream-1",
			StreamVersion: 1,
			EventType:     "test_event",
			Data:          json.RawMessage(`{"key": "value2"}`),
			Metadata:      map[string]string{"source": "test"},
			Timestamp:     time.Now().Add(-48 * time.Hour),
		},
	}

	ctx := context.Background()
	if err := store.WriteEvents(ctx, events); err != nil {
		t.Fatalf("WriteEvents() error = %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Verify count
	if store.Count() != 2 {
		t.Errorf("Count() = %d, want 2", store.Count())
	}

	// Read back
	archived, err := ReadArchiveFile(archivePath)
	if err != nil {
		t.Fatalf("ReadArchiveFile() error = %v", err)
	}

	if len(archived) != 2 {
		t.Errorf("ReadArchiveFile() returned %d events, want 2", len(archived))
	}

	// Verify first event
	if archived[0].Position != 1 {
		t.Errorf("archived[0].Position = %d, want 1", archived[0].Position)
	}
	if archived[0].StreamID != "test:stream-1" {
		t.Errorf("archived[0].StreamID = %s, want test:stream-1", archived[0].StreamID)
	}
	if archived[0].ArchivedAt.IsZero() {
		t.Error("archived[0].ArchivedAt should not be zero")
	}
}

func TestArchiveReader(t *testing.T) {
	// Create temp file with events
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "events.jsonl")

	// Write directly to file
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	encoder := json.NewEncoder(file)
	for i := 0; i < 3; i++ {
		event := ArchivedEvent{
			Position:   int64(i),
			StreamID:   "test:stream",
			EventType:  "test_event",
			Data:       json.RawMessage(`{}`),
			Timestamp:  time.Now(),
			ArchivedAt: time.Now(),
		}
		if err := encoder.Encode(event); err != nil {
			t.Fatalf("Failed to encode event: %v", err)
		}
	}
	file.Close()

	// Read with ArchiveReader
	reader, err := NewArchiveReader(archivePath)
	if err != nil {
		t.Fatalf("NewArchiveReader() error = %v", err)
	}
	defer reader.Close()

	count := 0
	for {
		event, err := reader.Next()
		if err != nil {
			break
		}
		if event.Position != int64(count) {
			t.Errorf("event.Position = %d, want %d", event.Position, count)
		}
		count++
	}

	if count != 3 {
		t.Errorf("Read %d events, want 3", count)
	}
}

func TestEventArchiver_DryRun(t *testing.T) {
	// Use in-memory store for this test
	store := NewMemoryStore()

	// Add some events
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		store.Append(ctx, "test:stream", VersionAny, []EventData{
			{EventType: "test_event", Data: json.RawMessage(`{}`)},
		})
	}

	// Create archiver
	archiver := NewEventArchiver(store)

	// Use short retention for testing
	policy := &RetentionPolicy{
		DefaultRetention: 1 * time.Millisecond,
		MinRetention:     1 * time.Nanosecond,
	}

	// Wait a bit for events to be "old enough"
	time.Sleep(10 * time.Millisecond)

	// Dry run archive
	result, err := archiver.Archive(ctx, &ArchiveConfig{
		Policy: policy,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}

	if result.EventsProcessed != 10 {
		t.Errorf("EventsProcessed = %d, want 10", result.EventsProcessed)
	}
	if result.EventsArchived != 10 {
		t.Errorf("EventsArchived = %d, want 10", result.EventsArchived)
	}
	if !result.DryRun {
		t.Error("DryRun should be true")
	}

	// Verify events still exist
	events, _ := store.ReadAll(ctx, 0, 100)
	if len(events) != 10 {
		t.Errorf("Store has %d events after dry run, want 10", len(events))
	}
}

func TestEventArchiver_Archive(t *testing.T) {
	// Use in-memory store
	store := NewMemoryStore()

	// Add events
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		store.Append(ctx, "test:stream", VersionAny, []EventData{
			{EventType: "test_event", Data: json.RawMessage(`{"seq": ` + string(rune('0'+i)) + `}`)},
		})
	}

	// Create temp archive file
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "archive.jsonl")

	archiveStore, err := NewFileArchiveStore(archivePath)
	if err != nil {
		t.Fatalf("NewFileArchiveStore() error = %v", err)
	}

	// Create archiver
	archiver := NewEventArchiver(store)

	// Very short retention for testing
	policy := &RetentionPolicy{
		DefaultRetention: 1 * time.Millisecond,
		MinRetention:     1 * time.Nanosecond,
	}

	// Wait for events to be "old enough"
	time.Sleep(10 * time.Millisecond)

	// Archive without deletion
	result, err := archiver.Archive(ctx, &ArchiveConfig{
		Policy:             policy,
		ArchiveStore:       archiveStore,
		DeleteAfterArchive: false,
	})
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}

	if result.EventsArchived != 5 {
		t.Errorf("EventsArchived = %d, want 5", result.EventsArchived)
	}
	if result.EventsDeleted != 0 {
		t.Errorf("EventsDeleted = %d, want 0", result.EventsDeleted)
	}

	// Verify archive file
	archived, err := ReadArchiveFile(archivePath)
	if err != nil {
		t.Fatalf("ReadArchiveFile() error = %v", err)
	}
	if len(archived) != 5 {
		t.Errorf("Archive has %d events, want 5", len(archived))
	}

	// Verify events still in store
	events, _ := store.ReadAll(ctx, 0, 100)
	if len(events) != 5 {
		t.Errorf("Store has %d events, want 5", len(events))
	}
}

func TestEventArchiver_EstimateArchival(t *testing.T) {
	store := NewMemoryStore()

	ctx := context.Background()
	for i := 0; i < 20; i++ {
		store.Append(ctx, "test:stream", VersionAny, []EventData{
			{EventType: "test_event", Data: json.RawMessage(`{}`)},
		})
	}

	archiver := NewEventArchiver(store)

	// Use policy where all events are eligible
	policy := &RetentionPolicy{
		DefaultRetention: 1 * time.Millisecond,
		MinRetention:     1 * time.Nanosecond,
	}

	time.Sleep(10 * time.Millisecond)

	estimate, err := archiver.EstimateArchival(ctx, policy)
	if err != nil {
		t.Fatalf("EstimateArchival() error = %v", err)
	}

	if estimate.EligibleEvents != 20 {
		t.Errorf("EligibleEvents = %d, want 20", estimate.EligibleEvents)
	}
	if estimate.TotalEvents != 20 {
		t.Errorf("TotalEvents = %d, want 20", estimate.TotalEvents)
	}
}
