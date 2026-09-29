package resolvers

import (
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestOperatorCanonicalEventFilterMapsEveryField(t *testing.T) {
	from := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	text := func(value string) *string { return &value }
	include := true
	mapped := operatorCanonicalEventFilter(&model.OperatorCanonicalEventFilter{
		EventType: text("patient_admit"), DefinitionID: text("adt-http"), ReceiptID: text("receipt-1"),
		SourceMessageID: text("MSH-10"), CorrelationID: text("corr"), From: &from, To: &to, IncludePurged: &include,
	})
	want := operator.CanonicalEventFilter{
		EventType: "patient_admit", DefinitionID: "adt-http", ReceiptID: "receipt-1",
		SourceMessageID: "MSH-10", CorrelationID: "corr", From: &from, To: &to, IncludePurged: true,
	}
	if mapped != want {
		t.Fatalf("filter = %+v, want %+v", mapped, want)
	}
	if empty := operatorCanonicalEventFilter(nil); empty != (operator.CanonicalEventFilter{}) || empty.IncludePurged {
		t.Fatalf("nil filter = %+v, want the zero filter (tombstoned rows excluded)", empty)
	}
}

func TestProjectOperatorCanonicalEventCarriesReceiptDefinitionAndSource(t *testing.T) {
	recorded := time.Date(2026, 9, 29, 4, 11, 0, 0, time.UTC)
	purged := recorded.Add(time.Hour)
	record := operator.CanonicalEventRecord{
		Event: operator.EventSummary{
			EventID: "event-1", ReceiptID: "receipt-1", EventType: "patient_admit",
			SourceMessageID: "E2E-FIXTURE-001", CorrelationID: "corr-1", Classification: "phi",
			RecordedAt: recorded, PurgedAt: &purged,
			PayloadFields: []operator.PayloadField{{Path: "patient.mrn", Kind: "string"}},
		},
		ReceiptStatus: "accepted",
		Definition:    integration.ArtifactRevisionRef{ArtifactID: "adt-east", RevisionID: "1", Digest: "sha256:d"},
		Source:        &integration.ArtifactRevisionRef{ArtifactID: "source-adt", RevisionID: "s1", Digest: "sha256:s"},
	}
	projected := projectOperatorCanonicalEvent(record)
	if projected.EventID != "event-1" || projected.ReceiptStatus != "accepted" || projected.SourceMessageID != "E2E-FIXTURE-001" ||
		projected.Definition == nil || projected.Definition.ArtifactID != "adt-east" ||
		projected.Source == nil || projected.Source.ArtifactID != "source-adt" ||
		projected.PurgedAt == nil || !projected.PurgedAt.Equal(purged) ||
		len(projected.PayloadFields) != 1 || projected.PayloadFields[0].Path != "patient.mrn" {
		t.Fatalf("projected = %+v", projected)
	}
	record.Source = nil
	record.Event.PayloadFields = nil
	bare := projectOperatorCanonicalEvent(record)
	if bare.Source != nil {
		t.Fatalf("an event without lineage projected source %+v", bare.Source)
	}
	if bare.PayloadFields == nil {
		t.Fatal("payloadFields is non-null in the schema; an empty summary must project an empty list")
	}
}

func TestProjectOperatorAdmissionStatisticsKeepsListsNonNilAndTheBucket(t *testing.T) {
	start := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	for _, bucket := range []model.OperatorStatisticsBucket{model.OperatorStatisticsBucketHour, model.OperatorStatisticsBucketDay} {
		mapped := operatorStatisticsBucket(bucket)
		if projectOperatorStatisticsBucket(mapped) != bucket {
			t.Fatalf("bucket %s does not round-trip (%s)", bucket, mapped)
		}
	}
	if operatorStatisticsBucket("WEEK") == operator.StatisticsBucketHour || operatorStatisticsBucket("WEEK") == operator.StatisticsBucketDay {
		t.Fatal("an unknown bucket must reach the service as unknown, so the service refuses it")
	}
	projected := projectOperatorAdmissionStatistics(operator.AdmissionStatistics{
		From: start, To: start.Add(24 * time.Hour), Bucket: operator.StatisticsBucketDay,
		AcceptedReceipts: 3, SucceededAttempts: 2, FailedAttempts: 1,
		Series: []operator.StatisticsBucketCounts{{Start: start, Accepted: 3, Succeeded: 2, Failed: 1}},
	})
	if projected.Bucket != model.OperatorStatisticsBucketDay || projected.AcceptedReceipts != 3 ||
		len(projected.Series) != 1 || projected.Series[0].Succeeded != 2 || !projected.Series[0].Start.Equal(start) {
		t.Fatalf("projected = %+v", projected)
	}
	if projected.EventsByType == nil || projected.ReceiptsByDefinition == nil || projected.AttemptsByDestination == nil {
		t.Fatal("grouped lists are non-null in the schema; empty groups must project empty lists")
	}
}
