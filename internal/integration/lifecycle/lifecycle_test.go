package lifecycle

import (
	"errors"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestAllowedStateTransitionIsClosedAndTerminal(t *testing.T) {
	tests := []struct {
		from integration.DeploymentState
		to   integration.DeploymentState
		want bool
	}{
		{integration.DeploymentStateDraft, integration.DeploymentStateValidated, true},
		{integration.DeploymentStateValidated, integration.DeploymentStateApproved, true},
		{integration.DeploymentStateApproved, integration.DeploymentStatePublished, true},
		{integration.DeploymentStatePublished, integration.DeploymentStateDeployed, true},
		{integration.DeploymentStateDeployed, integration.DeploymentStatePaused, true},
		{integration.DeploymentStatePaused, integration.DeploymentStateDeployed, true},
		{integration.DeploymentStatePublished, integration.DeploymentStateRetired, true},
		{integration.DeploymentStateDeployed, integration.DeploymentStateRetired, true},
		{integration.DeploymentStatePaused, integration.DeploymentStateRetired, true},
		{integration.DeploymentStateDraft, integration.DeploymentStatePublished, false},
		{integration.DeploymentStateRetired, integration.DeploymentStateDeployed, false},
		{"invented", integration.DeploymentStateDraft, false},
	}
	for _, tt := range tests {
		if got := allowedStateTransition(tt.from, tt.to); got != tt.want {
			t.Fatalf("allowedStateTransition(%q, %q) = %t, want %t", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestCommandAuditRequiresVersionedAuthenticatedActor(t *testing.T) {
	now := time.Date(2026, 7, 14, 18, 0, 0, 0, time.UTC)
	valid := Command{
		TenantID: "tenant-a", DefinitionID: "adt-http", RevisionID: "rev-1",
		ExpectedVersion: 3,
		Principal: integration.Principal{
			ID: "engineer-1", Kind: integration.PrincipalKindHuman,
			AuthMethod: "oidc", Roles: []string{"integration:publisher"},
		},
		Reason: "approve validated connection",
	}
	audit, err := valid.audit(now)
	if err != nil {
		t.Fatalf("valid audit: %v", err)
	}
	valid.Principal.Roles[0] = "mutated"
	if audit.Principal.Roles[0] != "integration:publisher" || !audit.OccurredAt.Equal(now) {
		t.Fatalf("audit was not stamped and copied: %#v", audit)
	}

	tests := []Command{
		{},
		func() Command { command := valid; command.ExpectedVersion = 0; return command }(),
		func() Command { command := valid; command.Principal.ID = ""; return command }(),
		func() Command { command := valid; command.Reason = ""; return command }(),
	}
	for _, command := range tests {
		if _, err := command.audit(now); !errors.Is(err, ErrInvalidCommand) {
			t.Fatalf("invalid command error = %v", err)
		}
	}
}

func TestValidationOutcomeRequiresSafeUniqueCodes(t *testing.T) {
	valid, err := normalizeValidationOutcome(ConnectionValidationOutcome{Passed: true, Codes: []string{"SOURCE_REACHABLE", "AUTH_OK"}})
	if err != nil || len(valid.Codes) != 2 {
		t.Fatalf("valid outcome = %#v, %v", valid, err)
	}
	for _, codes := range [][]string{
		nil,
		{"contains secret detail"},
		{"DUPLICATE", "DUPLICATE"},
	} {
		if _, err := normalizeValidationOutcome(ConnectionValidationOutcome{Codes: codes}); !errors.Is(err, ErrConnectionValidationFailed) {
			t.Fatalf("codes %#v error = %v", codes, err)
		}
	}
}

func TestReleaseDigestBindsImmutablePublication(t *testing.T) {
	snapshot := Snapshot{
		TenantID: "tenant-a",
		DefinitionRevision: integration.ArtifactRevisionRef{
			ArtifactID: "adt-http", RevisionID: "rev-1",
			Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		LastValidationID: "validation-1",
		ApprovalEventID:  "event-approve",
	}
	audit := integration.AuditEnvelope{
		TenantID: "tenant-a",
		Principal: integration.Principal{
			ID: "publisher-1", Kind: integration.PrincipalKindHuman,
			AuthMethod: "oidc", Roles: []string{"publisher", "engineer"},
		},
		Reason:     "publish exact tested revision",
		OccurredAt: time.Date(2026, 7, 14, 18, 0, 0, 0, time.UTC),
	}
	release, err := newReleaseRecord(snapshot, audit)
	if err != nil {
		t.Fatalf("newReleaseRecord: %v", err)
	}
	want, err := releaseRecordDigest(release)
	if err != nil || release.Digest != want {
		t.Fatalf("release digest = %q, want %q: %v", release.Digest, want, err)
	}
	release.ValidationID = "validation-2"
	mutated, err := releaseRecordDigest(release)
	if err != nil || mutated == release.Digest {
		t.Fatalf("release mutation did not change digest: %q %v", mutated, err)
	}
}
