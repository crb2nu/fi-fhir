//go:build integration

package authoring

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// .loom/42 E-1 kill-test (b): the catalog `serve` builds — the same
// NewPostgresCatalog + Migrate over the submission database — can host
// connection validation in process once it is given a validator. SKIP and
// STATIC record their codes and advance draft -> validated; a static check
// that finds the source unmounted records a failed validation and stays
// draft; a call that names no mode fails closed with CONNECTION_CHECK_ERROR.
func TestDefinitionAuthoringPostgres_ServeCatalogHostsSkipAndStaticValidation(t *testing.T) {
	ctx := t.Context()
	db := authoringDB(t)
	mountedDigest := ""
	validators := Validators{
		Skip: SkipValidator(),
		Static: StaticValidator(StaticChecks{
			SourceMounted: func(_ context.Context, source integration.ArtifactRevisionRef) (bool, error) {
				return source.Digest == mountedDigest, nil
			},
			SourceBindingNames: func(context.Context, integration.ArtifactRevisionRef) ([]string, bool, error) {
				return []string{}, true, nil
			},
		}),
	}
	catalog, err := lifecycle.NewPostgresCatalog(db, lifecycle.Config{ValidateConnection: validators.Func()})
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name       string
		mode       *Mode
		mounted    bool
		wantState  integration.DeploymentState
		wantCodes  []string
		wantPassed bool
	}{
		{name: "skip", mode: modePtr(ModeSkip), wantState: integration.DeploymentStateValidated, wantCodes: []string{CodeSkipped}, wantPassed: true},
		{name: "static-mounted", mode: modePtr(ModeStatic), mounted: true, wantState: integration.DeploymentStateValidated, wantCodes: []string{CodeStatic, CodeSourceMounted}, wantPassed: true},
		{name: "static-unmounted", mode: modePtr(ModeStatic), wantState: integration.DeploymentStateDraft, wantCodes: []string{CodeStatic, CodeSourceNotMounted}},
		{name: "no-mode", wantState: integration.DeploymentStateDraft, wantCodes: []string{"CONNECTION_CHECK_ERROR"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			revision := testDefinition(t, "kill-test-"+test.name, "mllp-"+test.name)
			mountedDigest = ""
			if test.mounted {
				mountedDigest = revision.Source.Digest
			}
			snapshot, err := catalog.CreateDraft(ctx, revision)
			if err != nil {
				t.Fatal(err)
			}
			callCtx := ctx
			if test.mode != nil {
				callCtx = WithMode(ctx, *test.mode)
			}
			snapshot, err = catalog.ValidateConnection(callCtx, testCommand(revision, snapshot.Version, "kill-test validation of "+test.name))
			if test.wantPassed && err != nil {
				t.Fatalf("ValidateConnection = %v", err)
			}
			if !test.wantPassed && !errors.Is(err, lifecycle.ErrConnectionValidationFailed) {
				t.Fatalf("ValidateConnection error = %v, want ErrConnectionValidationFailed", err)
			}
			stored, err := catalog.GetSnapshot(ctx, revision.TenantID, revision.DefinitionID, revision.RevisionID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.State != test.wantState || stored.Version != 2 || stored.ValidationPassed != test.wantPassed {
				t.Fatalf("snapshot = %s v%d passed=%v, want %s v2 passed=%v", stored.State, stored.Version, stored.ValidationPassed, test.wantState, test.wantPassed)
			}
			record, err := catalog.GetValidation(ctx, stored.LastValidationID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(record.Codes, test.wantCodes) || record.Passed != test.wantPassed {
				t.Fatalf("validation record = %v passed=%v, want %v passed=%v", record.Codes, record.Passed, test.wantCodes, test.wantPassed)
			}
			if record.SourceRevision != revision.Source.ArtifactRevisionRef {
				t.Fatalf("validation source = %#v, want %#v", record.SourceRevision, revision.Source.ArtifactRevisionRef)
			}
			if got := record.ExpiresAt.Sub(record.CheckedAt); got != 300*time.Second {
				t.Fatalf("validation max age = %s, want the policy's 300s", got)
			}
			_ = snapshot
		})
	}
}

func modePtr(mode Mode) *Mode { return &mode }
