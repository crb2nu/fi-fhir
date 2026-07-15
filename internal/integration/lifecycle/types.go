// Package lifecycle persists and advances versioned integration deployments.
package lifecycle

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

var (
	// ErrUnavailable means the PostgreSQL lifecycle catalog is not configured.
	ErrUnavailable = errors.New("integration lifecycle catalog unavailable")
	// ErrNotFound hides lifecycle inventory from callers.
	ErrNotFound = errors.New("integration lifecycle record not found")
	// ErrAlreadyExists means the immutable draft identity is already registered.
	ErrAlreadyExists = errors.New("integration lifecycle draft already exists")
	// ErrVersionConflict means another writer advanced the lifecycle snapshot.
	ErrVersionConflict = errors.New("integration lifecycle version conflict")
	// ErrInvalidTransition means the requested state change is not allowed.
	ErrInvalidTransition = errors.New("invalid integration lifecycle transition")
	// ErrConnectionValidationFailed means validation evidence was recorded but failed.
	ErrConnectionValidationFailed = errors.New("integration connection validation failed")
	// ErrConnectionValidationRequired means successful validation is missing or stale.
	ErrConnectionValidationRequired = errors.New("current integration connection validation required")
	// ErrActiveDeployment means another revision of this definition is deployed or paused.
	ErrActiveDeployment = errors.New("integration definition already has an active deployment")
	// ErrInvalidCommand means an actor or target identity is malformed.
	ErrInvalidCommand = errors.New("invalid integration lifecycle command")
	// ErrImmutableRecord means stored content no longer matches its digest.
	ErrImmutableRecord = errors.New("invalid immutable integration lifecycle record")
)

var validationCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

// ConnectionValidationOutcome contains bounded, catalog-safe validation facts.
type ConnectionValidationOutcome struct {
	Passed bool
	Codes  []string
}

// ConnectionValidatorFunc validates the exact source binding without returning secrets.
type ConnectionValidatorFunc func(context.Context, integration.IntegrationDefinitionRevision) (ConnectionValidationOutcome, error)

// Config supplies deterministic time and the external connection check.
type Config struct {
	Clock              func() time.Time
	ValidateConnection ConnectionValidatorFunc
}

// Command targets one snapshot version and carries the authenticated actor.
type Command struct {
	TenantID        string
	DefinitionID    string
	RevisionID      string
	ExpectedVersion int64
	Principal       integration.Principal
	Reason          string
}

// Snapshot is the mutable optimistic projection over append-only lifecycle facts.
type Snapshot struct {
	TenantID            string
	DefinitionRevision  integration.ArtifactRevisionRef
	State               integration.DeploymentState
	Version             int64
	ReleaseID           string
	Health              integration.DeploymentHealthStatus
	LastValidationID    string
	ValidationPassed    bool
	ValidationCheckedAt time.Time
	ValidationExpiresAt time.Time
	ApprovalEventID     string
	Updated             integration.AuditEnvelope
}

// ValidationRecord is immutable evidence for one exact source revision check.
type ValidationRecord struct {
	ID                 string
	TenantID           string
	DefinitionRevision integration.ArtifactRevisionRef
	SourceRevision     integration.ArtifactRevisionRef
	Passed             bool
	Codes              []string
	CheckedAt          time.Time
	ExpiresAt          time.Time
	Audit              integration.AuditEnvelope
}

// ReleaseRecord is the immutable publication binding consumed by deployments.
type ReleaseRecord struct {
	ID                 string
	TenantID           string
	DefinitionRevision integration.ArtifactRevisionRef
	ValidationID       string
	ApprovalEventID    string
	Published          integration.AuditEnvelope
	Digest             string
}

// EventRecord is one append-only lifecycle or health transition.
type EventRecord struct {
	ID                 string
	TenantID           string
	DefinitionRevision integration.ArtifactRevisionRef
	Version            int64
	Action             string
	FromState          integration.DeploymentState
	ToState            integration.DeploymentState
	Health             integration.DeploymentHealthStatus
	ReleaseID          string
	Audit              integration.AuditEnvelope
}

// RunnableBinding contains the exact active release identity for a source adapter.
type RunnableBinding struct {
	ReleaseID           string
	SnapshotVersion     int64
	Health              integration.DeploymentHealthStatus
	IntegrationRevision integration.ArtifactRevisionRef
	SourceID            string
	Format              events.SourceFormat
	Classification      integration.DataClassification
}

func (c Command) audit(now time.Time) (integration.AuditEnvelope, error) {
	if !validIdentity(c.TenantID) || !validIdentity(c.DefinitionID) || !validIdentity(c.RevisionID) || c.ExpectedVersion <= 0 {
		return integration.AuditEnvelope{}, ErrInvalidCommand
	}
	if !validIdentity(c.Principal.ID) || strings.TrimSpace(c.Principal.AuthMethod) == "" {
		return integration.AuditEnvelope{}, ErrInvalidCommand
	}
	if c.Principal.Kind != integration.PrincipalKindHuman && c.Principal.Kind != integration.PrincipalKindService {
		return integration.AuditEnvelope{}, ErrInvalidCommand
	}
	if len(c.Principal.Roles) > 32 {
		return integration.AuditEnvelope{}, ErrInvalidCommand
	}
	seenRoles := make(map[string]struct{}, len(c.Principal.Roles))
	for _, role := range c.Principal.Roles {
		if !validIdentity(role) {
			return integration.AuditEnvelope{}, ErrInvalidCommand
		}
		if _, duplicate := seenRoles[role]; duplicate {
			return integration.AuditEnvelope{}, ErrInvalidCommand
		}
		seenRoles[role] = struct{}{}
	}
	if c.Principal.Kind == integration.PrincipalKindHuman && strings.TrimSpace(c.Reason) == "" {
		return integration.AuditEnvelope{}, ErrInvalidCommand
	}
	if len(c.Reason) > 1024 {
		return integration.AuditEnvelope{}, ErrInvalidCommand
	}
	return integration.AuditEnvelope{
		TenantID:   c.TenantID,
		Principal:  clonePrincipal(c.Principal),
		Reason:     strings.TrimSpace(c.Reason),
		OccurredAt: now.UTC(),
	}, nil
}

func normalizeValidationOutcome(outcome ConnectionValidationOutcome) (ConnectionValidationOutcome, error) {
	if len(outcome.Codes) == 0 || len(outcome.Codes) > 32 {
		return ConnectionValidationOutcome{}, ErrConnectionValidationFailed
	}
	seen := make(map[string]struct{}, len(outcome.Codes))
	for _, code := range outcome.Codes {
		if !validationCodePattern.MatchString(code) {
			return ConnectionValidationOutcome{}, ErrConnectionValidationFailed
		}
		if _, duplicate := seen[code]; duplicate {
			return ConnectionValidationOutcome{}, ErrConnectionValidationFailed
		}
		seen[code] = struct{}{}
	}
	outcome.Codes = append([]string(nil), outcome.Codes...)
	return outcome, nil
}

func validIdentity(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func clonePrincipal(principal integration.Principal) integration.Principal {
	principal.Roles = append([]string(nil), principal.Roles...)
	return principal
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Updated.Principal = clonePrincipal(snapshot.Updated.Principal)
	return snapshot
}

func allowedStateTransition(from, to integration.DeploymentState) bool {
	switch from {
	case integration.DeploymentStateDraft:
		return to == integration.DeploymentStateValidated
	case integration.DeploymentStateValidated:
		return to == integration.DeploymentStateApproved
	case integration.DeploymentStateApproved:
		return to == integration.DeploymentStatePublished
	case integration.DeploymentStatePublished:
		return to == integration.DeploymentStateDeployed || to == integration.DeploymentStateRetired
	case integration.DeploymentStateDeployed:
		return to == integration.DeploymentStatePaused || to == integration.DeploymentStateRetired
	case integration.DeploymentStatePaused:
		return to == integration.DeploymentStateDeployed || to == integration.DeploymentStateRetired
	default:
		return false
	}
}
