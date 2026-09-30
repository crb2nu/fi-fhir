package resolvers

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
)

func TestProjectOperatorAttemptProjectsLedgerRowsOneToOne(t *testing.T) {
	completed := time.Date(2026, 9, 24, 9, 6, 0, 0, time.UTC)
	projected := projectOperatorAttempt(operator.DeliveryAttemptSummary{
		AttemptID: "attempt-a",
		Deliveries: []operator.DestinationDeliverySummary{
			{
				Transport: "fhir", DestinationArtifactID: "destination-fhir",
				DestinationRevisionID: "destination-1", DestinationClass: "production",
				DigestVerified: "sha256:abc", Outcome: "refused",
				FailureCode: "DELIVERY_DESTINATION_REJECTED", HTTPStatusClass: "4xx",
				EndpointAdvisory: "https://fhir.example.test/r4", CompletedAt: completed,
				FHIRResourceTypes: []string{"Patient", "Encounter"}, FHIREntryCount: 2,
				FHIROutcomeCodesAdvisory: []string{"invalid", "not-found"},
			},
			{
				Transport: "https", DestinationArtifactID: "destination-https",
				DestinationRevisionID: "destination-1", DestinationClass: "production",
				DigestVerified: "sha256:def", Outcome: "delivered", HTTPStatusClass: "2xx",
				EndpointAdvisory:                 "https://https.example.test/ingest",
				ServedCertificateSubjectAdvisory: "CN=https.example.test", CompletedAt: completed,
			},
		},
	})

	if len(projected.Deliveries) != 2 {
		t.Fatalf("deliveries = %#v, want 2", projected.Deliveries)
	}
	fhir := projected.Deliveries[0]
	if fhir.Transport != "fhir" || fhir.Outcome != "refused" || fhir.FailureCode != "DELIVERY_DESTINATION_REJECTED" ||
		fhir.HTTPStatusClass != "4xx" || fhir.EndpointAdvisory != "https://fhir.example.test/r4" ||
		fhir.FhirEntryCount != 2 || !fhir.CompletedAt.Equal(completed) ||
		!reflect.DeepEqual(fhir.FhirResourceTypes, []string{"Patient", "Encounter"}) ||
		!reflect.DeepEqual(fhir.FhirOutcomeCodesAdvisory, []string{"invalid", "not-found"}) {
		t.Fatalf("fhir delivery = %#v", fhir)
	}
	// The ledger records only the verified digest, so the destination
	// reference carries it too.
	if fhir.Destination == nil || fhir.Destination.ArtifactID != "destination-fhir" ||
		fhir.Destination.RevisionID != "destination-1" || fhir.Destination.Class != "production" ||
		fhir.Destination.Digest != "sha256:abc" || fhir.DigestVerified != "sha256:abc" {
		t.Fatalf("fhir delivery destination = %#v, digestVerified = %q", fhir.Destination, fhir.DigestVerified)
	}
	https := projected.Deliveries[1]
	if https.ServedCertificateSubjectAdvisory != "CN=https.example.test" || https.FhirEntryCount != 0 {
		t.Fatalf("https delivery = %#v", https)
	}
	// Non-null list fields: nil ledger slices become empty lists, never null.
	if https.FhirResourceTypes == nil || len(https.FhirResourceTypes) != 0 ||
		https.FhirOutcomeCodesAdvisory == nil || len(https.FhirOutcomeCodesAdvisory) != 0 {
		t.Fatalf("https delivery FHIR lists = %#v / %#v, want empty non-nil", https.FhirResourceTypes, https.FhirOutcomeCodesAdvisory)
	}

	empty := projectOperatorAttempt(operator.DeliveryAttemptSummary{AttemptID: "attempt-kafka"})
	if empty.Deliveries == nil || len(empty.Deliveries) != 0 {
		t.Fatalf("an attempt with no ledger rows projected %#v, want an empty non-nil list", empty.Deliveries)
	}
}

// TestCatalogOperatorErrorNamesActionableLifecycleRefusals pins the two
// lifecycle refusals an operator can act on to stable messages the IDE maps
// (operatorErrors.ts), instead of the generic control-plane failure.
func TestCatalogOperatorErrorNamesActionableLifecycleRefusals(t *testing.T) {
	cases := map[error]string{
		operator.ErrValidationRequired:                            "current connection validation required",
		fmt.Errorf("wrapped: %w", operator.ErrValidationRequired): "current connection validation required",
		operator.ErrActiveDeployment:                              "integration definition already has an active deployment",
		errors.New("unmapped"):                                    "operator control-plane request failed",
	}
	for cause, want := range cases {
		if got := catalogOperatorError(cause).Error(); got != want {
			t.Fatalf("catalogOperatorError(%v) = %q, want %q", cause, got, want)
		}
	}
}

func TestProjectOperatorDeploymentAndEventCarryDerivedAndRetentionFields(t *testing.T) {
	expires := time.Date(2026, 9, 29, 12, 5, 0, 0, time.UTC)
	deployment := projectOperatorDeployment(operator.DeploymentSummary{
		State: "deployed", ValidationPassed: true, ValidationExpiresAt: &expires, ValidationCurrent: true,
	})
	if !deployment.ValidationCurrent || deployment.ValidationExpiresAt == nil || !deployment.ValidationExpiresAt.Equal(expires) {
		t.Fatalf("deployment projection = %#v", deployment)
	}
	purgeAfter := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	purgedAt := purgeAfter.Add(time.Hour)
	event := projectOperatorEvent(operator.EventSummary{EventID: "event-a", PurgeAfter: &purgeAfter, PurgedAt: &purgedAt})
	if event.PurgeAfter == nil || !event.PurgeAfter.Equal(purgeAfter) || event.PurgedAt == nil || !event.PurgedAt.Equal(purgedAt) {
		t.Fatalf("event retention projection = %#v", event)
	}
	unstamped := projectOperatorEvent(operator.EventSummary{EventID: "event-b"})
	if unstamped.PurgeAfter != nil || unstamped.PurgedAt != nil {
		t.Fatalf("unstamped event projected retention marks: %#v", unstamped)
	}
}
