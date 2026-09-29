package authoring

import (
	"context"
	"errors"
	"fmt"
	"io"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// Mode is what a connection validation actually does (.loom/42 Decision 3).
// Every mode records its evidence in integration_connection_validations; the
// codes say which check ran, so no record claims a check that did not.
type Mode string

const (
	// ModeReal contacts the source: the batch provider the runner would
	// build lists one object of the input location. Only a batch source whose
	// credentials this process holds can be validated for real.
	ModeReal Mode = "real"
	// ModeStatic contacts nothing. It records whether a fresh replica mounts
	// the exact source revision and whether the source's secret bindings
	// resolve on this replica.
	ModeStatic Mode = "static"
	// ModeSkip records VALIDATION_SKIPPED and a reason of at least
	// MinSkipReasonBytes, exactly as `lifecycle seed --validate skip` does.
	ModeSkip Mode = "skip"
)

// MinSkipReasonBytes is the shortest reason a skipped validation accepts.
const MinSkipReasonBytes = 16

// Validation outcome codes recorded in integration_connection_validations.
const (
	CodeSkipped             = "VALIDATION_SKIPPED"
	CodeStatic              = "VALIDATION_STATIC"
	CodeSourceMounted       = "SOURCE_MOUNTED"
	CodeSourceNotMounted    = "SOURCE_NOT_MOUNTED"
	CodeBindingsResolved    = "BINDINGS_RESOLVED"
	CodeBindingUnresolvable = "BINDING_UNRESOLVABLE"
	CodeBindingsNotChecked  = "BINDINGS_NOT_CHECKED"
	CodeReachable           = "SOURCE_REACHABLE"
	CodeHostKeyVerified     = "HOST_KEY_VERIFIED"
	CodeAuthOK              = "AUTH_OK"
	CodeVersioningEnabled   = "BUCKET_VERSIONING_ENABLED"
	CodeInputListed         = "INPUT_LISTED"
	CodeConnectFailed       = "SOURCE_CONNECT_FAILED"
	CodeListFailed          = "INPUT_LIST_FAILED"
	CodeObjectInvalid       = "INPUT_OBJECT_INVALID"
	CodeSourceMismatch      = "SOURCE_REVISION_MISMATCH"
)

type modeKey struct{}

// WithMode tells the validator which check this ValidateConnection call is.
// The catalog's validator reads it from the context the catalog passes on.
func WithMode(ctx context.Context, mode Mode) context.Context {
	return context.WithValue(ctx, modeKey{}, mode)
}

func modeFrom(ctx context.Context) (Mode, bool) {
	mode, ok := ctx.Value(modeKey{}).(Mode)
	return mode, ok
}

// ErrModeMissing is a ValidateConnection call that did not say which check it
// is; the catalog records CONNECTION_CHECK_ERROR for it.
var ErrModeMissing = errors.New("connection validation mode is not set")

// SkipValidator records VALIDATION_SKIPPED and passes.
func SkipValidator() lifecycle.ConnectionValidatorFunc {
	return func(context.Context, integration.IntegrationDefinitionRevision) (lifecycle.ConnectionValidationOutcome, error) {
		return lifecycle.ConnectionValidationOutcome{Passed: true, Codes: []string{CodeSkipped}}, nil
	}
}

// StaticChecks are the facts a static validation reads. Every hook is
// optional; a missing hook makes its check "not checked" or "not mounted",
// never passed.
type StaticChecks struct {
	// SourceMounted reports whether a fresh replica mounts this exact source
	// revision (the observations ledger, or this replica's own description).
	SourceMounted func(ctx context.Context, source integration.ArtifactRevisionRef) (bool, error)
	// SourceBindingNames returns the binding names the source revision
	// requires; found is false when the revision is not in the connection
	// catalog, so its names are unknown.
	SourceBindingNames func(ctx context.Context, source integration.ArtifactRevisionRef) (names []string, found bool, err error)
	// ResolveSecret reads one reference with this replica's secret resolver
	// and discards the value. nil means this replica cannot resolve secrets.
	ResolveSecret func(ctx context.Context, reference integration.SecretReference) error
}

// StaticValidator is the MLLP/HTTP (and any-kind) check that contacts
// nothing. It passes only when the source revision is mounted by a fresh
// replica and no required source binding is missing or unresolvable.
func StaticValidator(checks StaticChecks) lifecycle.ConnectionValidatorFunc {
	return func(ctx context.Context, revision integration.IntegrationDefinitionRevision) (lifecycle.ConnectionValidationOutcome, error) {
		codes := []string{CodeStatic}
		passed := true
		mounted := false
		if checks.SourceMounted != nil {
			var err error
			mounted, err = checks.SourceMounted(ctx, revision.Source.ArtifactRevisionRef)
			if err != nil {
				return lifecycle.ConnectionValidationOutcome{}, err
			}
		}
		if mounted {
			codes = append(codes, CodeSourceMounted)
		} else {
			codes = append(codes, CodeSourceNotMounted)
			passed = false
		}
		if checks.SourceBindingNames == nil {
			return lifecycle.ConnectionValidationOutcome{Passed: passed, Codes: append(codes, CodeBindingsNotChecked)}, nil
		}
		names, found, err := checks.SourceBindingNames(ctx, revision.Source.ArtifactRevisionRef)
		if err != nil {
			return lifecycle.ConnectionValidationOutcome{}, err
		}
		switch {
		case !found:
			codes = append(codes, CodeBindingsNotChecked)
		case len(names) == 0:
			// Nothing to resolve, nothing to claim.
		default:
			bound := make(map[string]integration.SecretReference, len(revision.SecretBindings))
			for _, binding := range revision.SecretBindings {
				bound[binding.Name] = binding.Reference
			}
			unresolvable := false
			for _, name := range names {
				reference, ok := bound[name]
				if !ok {
					unresolvable = true
					continue
				}
				if checks.ResolveSecret == nil {
					continue
				}
				if checks.ResolveSecret(ctx, reference) != nil {
					unresolvable = true
				}
			}
			switch {
			case unresolvable:
				codes = append(codes, CodeBindingUnresolvable)
				passed = false
			case checks.ResolveSecret == nil:
				codes = append(codes, CodeBindingsNotChecked)
			default:
				codes = append(codes, CodeBindingsResolved)
			}
		}
		return lifecycle.ConnectionValidationOutcome{Passed: passed, Codes: codes}, nil
	}
}

// BatchSecrets is the credential material one batch source needs. It exists
// only to construct a provider and is never logged, marshaled, or persisted.
type BatchSecrets struct {
	S3   batch.S3Secrets
	SFTP batch.SFTPSecrets
}

// BatchProviderFactory builds the provider the runner would build.
type BatchProviderFactory func(batch.SourceRevision, BatchSecrets) (batch.Provider, error)

// NewBatchProvider is the runner's own provider construction.
func NewBatchProvider(source batch.SourceRevision, secrets BatchSecrets) (batch.Provider, error) {
	switch source.Provider {
	case batch.ProviderS3:
		provider, err := batch.NewS3Provider(source, secrets.S3)
		if err != nil {
			return nil, fmt.Errorf("configure batch S3 provider: %w", err)
		}
		return provider, nil
	case batch.ProviderSFTP:
		provider, err := batch.NewSFTPProvider(source, secrets.SFTP)
		if err != nil {
			return nil, fmt.Errorf("configure batch SFTP provider: %w", err)
		}
		return provider, nil
	default:
		return nil, fmt.Errorf("configure batch provider: unsupported provider")
	}
}

// BatchValidator builds the provider the runner would build and lists at most
// one object of the input location. The SFTP provider dials without a
// context, so the probe runs beside the catalog's deadline; on expiry the
// catalog records CONNECTION_CHECK_TIMEOUT and the probe closes its own
// provider when its dial returns, so no connection outlives it. Provider
// errors are already free of hosts, credentials, and paths; they go to detail
// as operator detail and never into the catalog, which keeps codes only.
func BatchValidator(
	source batch.SourceRevision,
	secrets BatchSecrets,
	build BatchProviderFactory,
	detail io.Writer,
) lifecycle.ConnectionValidatorFunc {
	if build == nil {
		build = NewBatchProvider
	}
	return func(ctx context.Context, revision integration.IntegrationDefinitionRevision) (lifecycle.ConnectionValidationOutcome, error) {
		if revision.Source.ArtifactRevisionRef != source.Reference() || revision.Source.SourceID != source.SourceID {
			return lifecycle.ConnectionValidationOutcome{Codes: []string{CodeSourceMismatch}}, nil
		}
		type probeResult struct {
			outcome lifecycle.ConnectionValidationOutcome
			detail  error
		}
		done := make(chan probeResult, 1)
		go func() {
			outcome, probeDetail := probeBatchSource(ctx, source, secrets, build)
			done <- probeResult{outcome: outcome, detail: probeDetail}
		}()
		select {
		case <-ctx.Done():
			return lifecycle.ConnectionValidationOutcome{}, ctx.Err()
		case result := <-done:
			if result.detail != nil && detail != nil {
				_, _ = fmt.Fprintf(detail, "connection validation detail: %v\n", result.detail)
			}
			return result.outcome, nil
		}
	}
}

func probeBatchSource(
	ctx context.Context,
	source batch.SourceRevision,
	secrets BatchSecrets,
	build BatchProviderFactory,
) (lifecycle.ConnectionValidationOutcome, error) {
	provider, err := build(source, secrets)
	if err != nil {
		return lifecycle.ConnectionValidationOutcome{Codes: []string{CodeConnectFailed}}, err
	}
	defer func() { _ = provider.Close() }()
	// NewSFTPProvider returns only after the TCP dial, the pinned host-key
	// check, and authentication succeed. The S3 client makes no request until
	// List.
	reached := []string{}
	if source.Provider == batch.ProviderSFTP {
		reached = []string{CodeReachable, CodeHostKeyVerified, CodeAuthOK}
	}
	if _, err := provider.List(ctx, 1); err != nil {
		code := CodeListFailed
		if errors.Is(err, batch.ErrInvalidObject) {
			code = CodeObjectInvalid
		}
		return lifecycle.ConnectionValidationOutcome{Codes: append(reached, code)}, err
	}
	if source.Provider == batch.ProviderS3 {
		// List checks bucket versioning before listing under the input prefix.
		reached = []string{CodeReachable, CodeAuthOK, CodeVersioningEnabled}
	}
	return lifecycle.ConnectionValidationOutcome{Passed: true, Codes: append(reached, CodeInputListed)}, nil
}

// Validators is the catalog's one ConnectionValidatorFunc for a process that
// offers more than one mode, dispatching on WithMode. Real is nil when this
// process holds no batch credentials; a real call then records
// CONNECTION_CHECK_ERROR, which the service prevents by refusing first.
type Validators struct {
	Real   lifecycle.ConnectionValidatorFunc
	Static lifecycle.ConnectionValidatorFunc
	Skip   lifecycle.ConnectionValidatorFunc
}

// ErrModeUnavailable is a mode this process cannot run.
var ErrModeUnavailable = errors.New("connection validation mode is unavailable on this replica")

// Func returns the dispatching validator.
func (v Validators) Func() lifecycle.ConnectionValidatorFunc {
	return func(ctx context.Context, revision integration.IntegrationDefinitionRevision) (lifecycle.ConnectionValidationOutcome, error) {
		mode, ok := modeFrom(ctx)
		if !ok {
			return lifecycle.ConnectionValidationOutcome{}, ErrModeMissing
		}
		var validator lifecycle.ConnectionValidatorFunc
		switch mode {
		case ModeReal:
			validator = v.Real
		case ModeStatic:
			validator = v.Static
		case ModeSkip:
			validator = v.Skip
		}
		if validator == nil {
			return lifecycle.ConnectionValidationOutcome{}, ErrModeUnavailable
		}
		return validator(ctx, revision)
	}
}
