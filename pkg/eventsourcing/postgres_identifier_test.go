package eventsourcing

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/lib/pq"
)

func TestPostgresStoreConstructorsQuoteIdentifiers(t *testing.T) {
	configured := `events"; DROP TABLE patients;--`
	wantTable := pq.QuoteIdentifier(configured)

	eventStore := NewPostgresStore(nil, PostgresStoreConfig{TableName: configured})
	if eventStore.tableName != wantTable {
		t.Fatalf("event store table = %q, want %q", eventStore.tableName, wantTable)
	}
	if eventStore.PhysicalTableName() != configured {
		t.Fatalf("event store physical table = %q", eventStore.PhysicalTableName())
	}
	if eventStore.streamIndexName != pq.QuoteIdentifier("idx_"+configured+"_stream") {
		t.Fatalf("event store stream index is not quoted")
	}

	checkpointStore := NewPostgresCheckpointStore(nil, configured)
	if checkpointStore.tableName != wantTable {
		t.Fatalf("checkpoint table = %q, want %q", checkpointStore.tableName, wantTable)
	}
	if checkpointStore.PhysicalTableName() != configured {
		t.Fatalf("checkpoint physical table = %q", checkpointStore.PhysicalTableName())
	}

	snapshotStore := NewPostgresSnapshotStore(nil, configured)
	if snapshotStore.tableName != wantTable {
		t.Fatalf("snapshot table = %q, want %q", snapshotStore.tableName, wantTable)
	}
	if snapshotStore.PhysicalTableName() != configured {
		t.Fatalf("snapshot physical table = %q", snapshotStore.PhysicalTableName())
	}
	if snapshotStore.indexName != pq.QuoteIdentifier("idx_"+configured+"_projection") {
		t.Fatalf("snapshot index is not quoted")
	}

	streamSnapshotStore := NewPostgresStreamSnapshotStore(nil, configured)
	if streamSnapshotStore.tableName != wantTable {
		t.Fatalf("stream snapshot table = %q, want %q", streamSnapshotStore.tableName, wantTable)
	}
	if streamSnapshotStore.PhysicalTableName() != configured {
		t.Fatalf("stream snapshot physical table = %q", streamSnapshotStore.PhysicalTableName())
	}
	if streamSnapshotStore.indexName != pq.QuoteIdentifier("idx_"+configured+"_type") {
		t.Fatalf("stream snapshot index is not quoted")
	}
}

func TestPostgresStoreConstructorsQuoteDefaultIdentifiers(t *testing.T) {
	eventStore := NewPostgresStore(nil, PostgresStoreConfig{})
	if eventStore.tableName != `"events"` {
		t.Fatalf("default event table = %q", eventStore.tableName)
	}

	checkpointStore := NewPostgresCheckpointStore(nil, "")
	if checkpointStore.tableName != `"projection_checkpoints"` {
		t.Fatalf("default checkpoint table = %q", checkpointStore.tableName)
	}

	snapshotStore := NewPostgresSnapshotStore(nil, "")
	if snapshotStore.tableName != `"projection_snapshots"` {
		t.Fatalf("default snapshot table = %q", snapshotStore.tableName)
	}

	streamSnapshotStore := NewPostgresStreamSnapshotStore(nil, "")
	if streamSnapshotStore.tableName != `"stream_snapshots"` {
		t.Fatalf("default stream snapshot table = %q", streamSnapshotStore.tableName)
	}
}

func TestPostgresStoreConstructorsNormalizeNULIdentifiers(t *testing.T) {
	configured := "events\x00archive"
	eventStore := NewPostgresStore(nil, PostgresStoreConfig{TableName: configured})
	checkpointStore := NewPostgresCheckpointStore(nil, configured)
	snapshotStore := NewPostgresSnapshotStore(nil, configured)
	streamSnapshotStore := NewPostgresStreamSnapshotStore(nil, configured)

	stores := []struct {
		name     string
		quoted   string
		physical string
	}{
		{name: "events", quoted: eventStore.tableName, physical: eventStore.PhysicalTableName()},
		{name: "checkpoints", quoted: checkpointStore.tableName, physical: checkpointStore.PhysicalTableName()},
		{name: "projection snapshots", quoted: snapshotStore.tableName, physical: snapshotStore.PhysicalTableName()},
		{name: "stream snapshots", quoted: streamSnapshotStore.tableName, physical: streamSnapshotStore.PhysicalTableName()},
	}

	for _, store := range stores {
		t.Run(store.name, func(t *testing.T) {
			if store.physical == "events" {
				t.Fatal("NUL-containing identifier collided with its truncated prefix")
			}
			if strings.ContainsRune(store.physical, '\x00') {
				t.Fatalf("physical table contains NUL: %q", store.physical)
			}
			if store.quoted != pq.QuoteIdentifier(store.physical) {
				t.Fatalf("quoted table = %q, want quote of %q", store.quoted, store.physical)
			}
		})
	}
}

func TestNormalizePostgresIdentifier(t *testing.T) {
	maxLength := strings.Repeat("e", postgresIdentifierMaxBytes)
	if got := normalizePostgresIdentifier(maxLength); got != maxLength {
		t.Fatalf("maximum-length identifier changed: %q", got)
	}

	long := strings.Repeat("é", postgresIdentifierMaxBytes)
	got := normalizePostgresIdentifier(long)
	if len(got) > postgresIdentifierMaxBytes {
		t.Fatalf("normalized identifier has %d bytes, maximum is %d", len(got), postgresIdentifierMaxBytes)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("normalized identifier is not valid UTF-8: %q", got)
	}
	if got != normalizePostgresIdentifier(long) {
		t.Fatal("normalization is not deterministic")
	}
	if got == normalizePostgresIdentifier(long+"x") {
		t.Fatal("different long identifiers normalized to the same value")
	}

	nul := "events\x00archive"
	nulNormalized := normalizePostgresIdentifier(nul)
	if nulNormalized == "events" || strings.ContainsRune(nulNormalized, '\x00') {
		t.Fatalf("NUL identifier normalized unsafely: %q", nulNormalized)
	}
	if nulNormalized != normalizePostgresIdentifier(nul) {
		t.Fatal("NUL normalization is not deterministic")
	}
}

func TestDerivedPostgresIdentifiersDoNotCollideAtMaximumBaseLength(t *testing.T) {
	base := strings.Repeat("e", postgresIdentifierMaxBytes)
	eventStore := NewPostgresStore(nil, PostgresStoreConfig{TableName: base})
	checkpointStore := NewPostgresCheckpointStore(nil, base+"_checkpoints")
	snapshotStore := NewPostgresSnapshotStore(nil, base+"_snapshots")
	streamSnapshotStore := NewPostgresStreamSnapshotStore(nil, base+"_stream_snapshots")
	identifiers := []string{
		eventStore.tableName,
		checkpointStore.tableName,
		snapshotStore.tableName,
		streamSnapshotStore.tableName,
		eventStore.streamIndexName,
		eventStore.typeIndexName,
		eventStore.timestampIndexName,
		snapshotStore.indexName,
		streamSnapshotStore.indexName,
	}

	seen := make(map[string]struct{}, len(identifiers))
	for _, identifier := range identifiers {
		if len(strings.Trim(identifier, `"`)) > postgresIdentifierMaxBytes {
			t.Fatalf("identifier exceeds PostgreSQL limit: %q", identifier)
		}
		if _, exists := seen[identifier]; exists {
			t.Fatalf("derived identifier collision: %q", identifier)
		}
		seen[identifier] = struct{}{}
	}
}
