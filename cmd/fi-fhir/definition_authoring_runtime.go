package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle/authoring"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// .loom/42 E-1: serve hosts definition authoring. The lifecycle catalog serve
// builds gets one ConnectionValidatorFunc that dispatches on the mode the
// authoring service puts on the context (authoring.WithMode):
//
//   - REAL: the batch validator `lifecycle seed` runs, for the batch source this
//     replica mounts only — its FI_FHIR_BATCH_* credentials are the ones the
//     co-located batch runner already uses, so REAL opens no connection the
//     pod does not already open.
//   - STATIC: contacts nothing; SOURCE_MOUNTED when a fresh replica (the
//     observations ledger) or this one mounts the exact source digest, and
//     the source's binding names bound in the definition.
//   - SKIP: VALIDATION_SKIPPED with a reason of at least 16 bytes.
//
// The catalog is built before the connection catalog service and the engine
// runtime description exist, so the static checks read them late, through
// bind; before bind a static check reports SOURCE_NOT_MOUNTED.

const envLifecycleValidationMaxAge = "FI_FHIR_LIFECYCLE_VALIDATION_MAX_AGE"

// lifecycleValidationMaxAgeFromEnv reads the default evidence max age for
// definitions the editor authors. It must cover the default 5 s validation
// timeout and fit the policy's 86400 s ceiling.
func lifecycleValidationMaxAgeFromEnv() (int64, error) {
	raw := strings.TrimSpace(os.Getenv(envLifecycleValidationMaxAge))
	if raw == "" {
		return authoring.DefaultValidationMaxAgeSeconds, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 5 || value > 86400 || strconv.FormatInt(value, 10) != raw {
		return 0, fmt.Errorf("%s must be a whole number of seconds between 5 and 86400", envLifecycleValidationMaxAge)
	}
	return value, nil
}

type definitionAuthoringRuntime struct {
	maxAge     int64
	real       lifecycle.ConnectionValidatorFunc
	realSource *integration.SourceRevisionRef
	facts      authoring.CatalogFacts
}

// realValidationLimit caps a REAL check an API request starts, whatever the
// definition's validation timeout.
const realValidationLimit = 20 * time.Second

// newDefinitionAuthoringRuntime reads the max age and, when this replica runs
// the batch runner, the credentials its REAL check uses.
func newDefinitionAuthoringRuntime(composition runtimeComposition, logger *slog.Logger) (*definitionAuthoringRuntime, error) {
	maxAge, err := lifecycleValidationMaxAgeFromEnv()
	if err != nil {
		return nil, err
	}
	runtime := &definitionAuthoringRuntime{maxAge: maxAge}
	if composition.batch != nil {
		source := composition.batch.source
		secrets, err := loadBatchProviderSecretsFromEnv(source)
		if err != nil {
			return nil, fmt.Errorf("configure definition authoring real validation: %w", err)
		}
		runtime.real = authoring.BatchValidatorWithLimit(source, secrets, nil, validationDetailWriter{logger: logger}, realValidationLimit)
		runtime.realSource = &integration.SourceRevisionRef{ArtifactRevisionRef: source.Reference(), SourceID: source.SourceID}
	}
	return runtime, nil
}

// validator is the catalog's one ConnectionValidatorFunc.
func (r *definitionAuthoringRuntime) validator() lifecycle.ConnectionValidatorFunc {
	return authoring.Validators{
		Real:   r.real,
		Skip:   authoring.SkipValidator(),
		Static: authoring.StaticValidator(r.facts.Checks()),
	}.Func()
}

// bind hands the static checks the connection catalog and the description.
func (r *definitionAuthoringRuntime) bind(connections *connection.Service, description *connection.RuntimeDescription) {
	r.facts.Bind(connections, description)
}

// validationDetailWriter logs a REAL probe's provider error. The batch
// providers' errors carry no host, credential, or path; the catalog keeps
// codes only.
type validationDetailWriter struct {
	logger *slog.Logger
}

func (w validationDetailWriter) Write(detail []byte) (int, error) {
	if w.logger != nil {
		w.logger.Warn("definition connection validation detail",
			observability.F(observability.FieldComponent, "definition-authoring"),
			observability.F(observability.FieldReason, strings.TrimSpace(string(detail))))
	}
	return len(detail), nil
}
