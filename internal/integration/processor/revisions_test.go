package processor_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestNewProfileRevisionReferenceCanonicalizesJSONObject(t *testing.T) {
	t.Parallel()

	first, err := processor.NewProfileRevisionReference(
		"profile-adt",
		42,
		[]byte(`{"b":[{"z":2,"a":1}],"a":true}`),
	)
	if err != nil {
		t.Fatalf("NewProfileRevisionReference(first): %v", err)
	}
	second, err := processor.NewProfileRevisionReference(
		"profile-adt",
		42,
		[]byte("{\n  \"a\": true, \"b\": [ { \"a\": 1, \"z\": 2 } ]\n}"),
	)
	if err != nil {
		t.Fatalf("NewProfileRevisionReference(second): %v", err)
	}
	if first != second {
		t.Fatalf("insignificant JSON differences changed reference:\nfirst:  %#v\nsecond: %#v", first, second)
	}
	if first.ArtifactID != "profile-adt" || first.RevisionID != "42" {
		t.Fatalf("unexpected reference identity: %#v", first)
	}
	wantDigest := testDomainDigest(
		"fi-fhir/profile-revision/v1\x00",
		[]byte(`{"a":true,"b":[{"a":1e0,"z":2e0}]}`),
	)
	if first.Digest != wantDigest {
		t.Fatalf("profile digest domain/canonical bytes mismatch: got %q want %q", first.Digest, wantDigest)
	}

	changed, err := processor.NewProfileRevisionReference(
		"profile-adt",
		42,
		[]byte(`{"a":false,"b":[{"a":1,"z":2}]}`),
	)
	if err != nil {
		t.Fatalf("NewProfileRevisionReference(changed): %v", err)
	}
	if first.Digest == changed.Digest {
		t.Fatal("semantic profile mutation did not change digest")
	}
}

func TestNewProfileRevisionReferenceCanonicalizesEquivalentNumbers(t *testing.T) {
	t.Parallel()

	scientific, err := processor.NewProfileRevisionReference(
		"profile-adt",
		42,
		[]byte(`{"threshold":1.230e-5,"whole":1e2}`),
	)
	if err != nil {
		t.Fatalf("NewProfileRevisionReference(scientific): %v", err)
	}
	expanded, err := processor.NewProfileRevisionReference(
		"profile-adt",
		42,
		[]byte(`{"threshold":0.00001230,"whole":100.0}`),
	)
	if err != nil {
		t.Fatalf("NewProfileRevisionReference(expanded): %v", err)
	}
	if scientific != expanded {
		t.Fatalf("equivalent JSON numbers changed reference:\nscientific: %#v\nexpanded:   %#v", scientific, expanded)
	}
}

func TestNewProfileRevisionReferenceRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		artifactID string
		revisionID int
		config     string
	}{
		{name: "empty artifact", revisionID: 1, config: `{}`},
		{name: "noncanonical artifact", artifactID: " profile-adt ", revisionID: 1, config: `{}`},
		{name: "control artifact", artifactID: "profile\nadt", revisionID: 1, config: `{}`},
		{name: "oversized artifact", artifactID: strings.Repeat("a", 257), revisionID: 1, config: `{}`},
		{name: "invalid revision", artifactID: "profile-adt", revisionID: 0, config: `{}`},
		{name: "malformed", artifactID: "profile-adt", revisionID: 1, config: `{"a":`},
		{name: "trailing value", artifactID: "profile-adt", revisionID: 1, config: `{} {}`},
		{name: "non-object", artifactID: "profile-adt", revisionID: 1, config: `[]`},
		{name: "duplicate top-level key", artifactID: "profile-adt", revisionID: 1, config: `{"a":1,"a":2}`},
		{name: "duplicate nested key", artifactID: "profile-adt", revisionID: 1, config: `{"nested":{"a":1,"\u0061":2}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := processor.NewProfileRevisionReference(tt.artifactID, tt.revisionID, []byte(tt.config)); err == nil {
				t.Fatal("expected invalid profile revision to fail")
			}
		})
	}
}

func TestNewProfileRevisionReferenceRejectsInvalidUnicode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config []byte
	}{
		{name: "invalid UTF-8", config: []byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}},
		{name: "unpaired high surrogate", config: []byte(`{"x":"\ud800"}`)},
		{name: "unpaired low surrogate", config: []byte(`{"x":"\udc00"}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := processor.NewProfileRevisionReference("profile-adt", 1, tt.config); err == nil {
				t.Fatal("expected invalid Unicode profile JSON to fail")
			}
		})
	}

	if _, err := processor.NewProfileRevisionReference("profile-adt", 1, []byte(`{"x":"\ud83d\ude00"}`)); err != nil {
		t.Fatalf("valid surrogate pair was rejected: %v", err)
	}
}

func TestNewWorkflowRevisionReferenceUsesExactUTF8Bytes(t *testing.T) {
	t.Parallel()

	withoutNewline, err := processor.NewWorkflowRevisionReference("workflow-adt", "version-1", []byte("name: adt"))
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference(without newline): %v", err)
	}
	withNewline, err := processor.NewWorkflowRevisionReference("workflow-adt", "version-1", []byte("name: adt\n"))
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference(with newline): %v", err)
	}
	if withoutNewline.Digest == withNewline.Digest {
		t.Fatal("workflow byte mutation did not change digest")
	}
	wantDigest := testDomainDigest("fi-fhir/workflow-revision/v1\x00", []byte("name: adt"))
	if withoutNewline.Digest != wantDigest {
		t.Fatalf("workflow digest domain/exact bytes mismatch: got %q want %q", withoutNewline.Digest, wantDigest)
	}

	invalid := []byte{0xff, 0xfe}
	if _, err := processor.NewWorkflowRevisionReference("workflow-adt", "version-1", invalid); err == nil {
		t.Fatal("expected invalid UTF-8 workflow to fail")
	}
	if _, err := processor.NewWorkflowRevisionReference("workflow-adt", " version-1 ", []byte("name: adt\n")); err == nil {
		t.Fatal("expected noncanonical workflow revision ID to fail")
	}
}

func TestRevisionResolverVerifiesContentAndReturnsDefensiveCopies(t *testing.T) {
	t.Parallel()

	profile := []byte(`{"name":"ADT","rules":["A01"]}`)
	workflow := []byte("name: adt\nroutes: []\n")
	profileRef, err := processor.NewProfileRevisionReference("profile-adt", 7, profile)
	if err != nil {
		t.Fatalf("NewProfileRevisionReference: %v", err)
	}
	workflowRef, err := processor.NewWorkflowRevisionReference("workflow-adt", "version-9", workflow)
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference: %v", err)
	}
	loader := &fakeArtifactRevisionLoader{profile: profile, workflow: workflow}
	resolver, err := processor.NewRevisionResolver("tenant-a", loader)
	if err != nil {
		t.Fatalf("NewRevisionResolver: %v", err)
	}

	resolved, err := resolver.Resolve(context.Background(), "tenant-a", profileRef, workflowRef)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	profileCopy := resolved.ProfileJSON()
	workflowCopy := resolved.WorkflowYAML()
	profileCopy[0] = '!'
	workflowCopy[0] = '!'

	again, err := resolver.Resolve(context.Background(), "tenant-a", profileRef, workflowRef)
	if err != nil {
		t.Fatalf("Resolve(second): %v", err)
	}
	if got := string(again.ProfileJSON()); got != string(profile) {
		t.Fatalf("profile bytes were aliased: got %q want %q", got, profile)
	}
	if got := string(again.WorkflowYAML()); got != string(workflow) {
		t.Fatalf("workflow bytes were aliased: got %q want %q", got, workflow)
	}
	if string(loader.profile) != string(profile) || string(loader.workflow) != string(workflow) {
		t.Fatal("resolver mutated loader-owned content")
	}
}

func TestRevisionResolverRejectsWrongTenantBeforeLoading(t *testing.T) {
	t.Parallel()

	loader := &fakeArtifactRevisionLoader{}
	resolver, err := processor.NewRevisionResolver("tenant-a", loader)
	if err != nil {
		t.Fatalf("NewRevisionResolver: %v", err)
	}
	_, err = resolver.Resolve(context.Background(), "tenant-b", integration.ArtifactRevisionRef{}, integration.ArtifactRevisionRef{})
	if !errors.Is(err, processor.ErrTenantMismatch) {
		t.Fatalf("expected tenant mismatch, got %v", err)
	}
	if loader.profileLoads != 0 || loader.workflowLoads != 0 {
		t.Fatalf("wrong tenant reached loader: profile=%d workflow=%d", loader.profileLoads, loader.workflowLoads)
	}
}

func TestRevisionResolverRejectsDigestMismatchAndMalformedStoredContent(t *testing.T) {
	t.Parallel()

	profile := []byte(`{"name":"ADT"}`)
	workflow := []byte("name: adt\n")
	profileRef, err := processor.NewProfileRevisionReference("profile-adt", 1, profile)
	if err != nil {
		t.Fatalf("NewProfileRevisionReference: %v", err)
	}
	workflowRef, err := processor.NewWorkflowRevisionReference("workflow-adt", "version-1", workflow)
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference: %v", err)
	}

	tests := []struct {
		name        string
		loader      *fakeArtifactRevisionLoader
		profileRef  integration.ArtifactRevisionRef
		workflowRef integration.ArtifactRevisionRef
		want        error
	}{
		{
			name:        "profile digest mismatch",
			loader:      &fakeArtifactRevisionLoader{profile: []byte(`{"name":"changed"}`), workflow: workflow},
			profileRef:  profileRef,
			workflowRef: workflowRef,
			want:        processor.ErrArtifactDigestMismatch,
		},
		{
			name:        "workflow digest mismatch",
			loader:      &fakeArtifactRevisionLoader{profile: profile, workflow: []byte("name: changed\n")},
			profileRef:  profileRef,
			workflowRef: workflowRef,
			want:        processor.ErrArtifactDigestMismatch,
		},
		{
			name:        "malformed stored profile",
			loader:      &fakeArtifactRevisionLoader{profile: []byte(`{"a":1,"a":2}`), workflow: workflow},
			profileRef:  profileRef,
			workflowRef: workflowRef,
			want:        processor.ErrInvalidArtifactContent,
		},
		{
			name:        "invalid stored workflow utf8",
			loader:      &fakeArtifactRevisionLoader{profile: profile, workflow: []byte{0xff}},
			profileRef:  profileRef,
			workflowRef: workflowRef,
			want:        processor.ErrInvalidArtifactContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resolver, err := processor.NewRevisionResolver("tenant-a", tt.loader)
			if err != nil {
				t.Fatalf("NewRevisionResolver: %v", err)
			}
			_, err = resolver.Resolve(context.Background(), "tenant-a", tt.profileRef, tt.workflowRef)
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}
}

func TestRevisionResolverRejectsMalformedReferencesBeforeLoading(t *testing.T) {
	t.Parallel()

	profile := []byte(`{"name":"ADT"}`)
	workflow := []byte("name: adt\n")
	profileRef, err := processor.NewProfileRevisionReference("profile-adt", 1, profile)
	if err != nil {
		t.Fatalf("NewProfileRevisionReference: %v", err)
	}
	workflowRef, err := processor.NewWorkflowRevisionReference("workflow-adt", "version-1", workflow)
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference: %v", err)
	}
	profileRef.RevisionID = "01"
	workflowRef.Digest = strings.ToUpper(workflowRef.Digest)

	loader := &fakeArtifactRevisionLoader{profile: profile, workflow: workflow}
	resolver, err := processor.NewRevisionResolver("tenant-a", loader)
	if err != nil {
		t.Fatalf("NewRevisionResolver: %v", err)
	}
	_, err = resolver.Resolve(context.Background(), "tenant-a", profileRef, workflowRef)
	if !errors.Is(err, processor.ErrInvalidArtifactReference) {
		t.Fatalf("expected invalid reference, got %v", err)
	}
	if loader.profileLoads != 0 || loader.workflowLoads != 0 {
		t.Fatalf("malformed reference reached loader: profile=%d workflow=%d", loader.profileLoads, loader.workflowLoads)
	}
}

type fakeArtifactRevisionLoader struct {
	profile       []byte
	workflow      []byte
	profileErr    error
	workflowErr   error
	profileLoads  int
	workflowLoads int
}

func (l *fakeArtifactRevisionLoader) LoadProfileRevision(context.Context, string, string) ([]byte, error) {
	l.profileLoads++
	if l.profileErr != nil {
		return nil, l.profileErr
	}
	return l.profile, nil
}

func (l *fakeArtifactRevisionLoader) LoadWorkflowRevision(context.Context, string, string) ([]byte, error) {
	l.workflowLoads++
	if l.workflowErr != nil {
		return nil, l.workflowErr
	}
	return l.workflow, nil
}

func testDomainDigest(domain string, content []byte) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(domain))
	_, _ = hash.Write(content)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}
