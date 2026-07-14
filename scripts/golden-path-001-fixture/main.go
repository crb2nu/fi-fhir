package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/registry"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

type registryDocument struct {
	TenantID     string          `json:"tenant_id"`
	Integrations []registryEntry `json:"integrations"`
}

type registryEntry struct {
	IntegrationID string          `json:"integration_id"`
	Definition    json.RawMessage `json:"definition"`
	Profile       json.RawMessage `json:"profile"`
	Workflow      string          `json:"workflow"`
}

func main() {
	fixtureDir := flag.String("fixture", "testdata/golden/integration/adt-http", "fixture directory")
	outputPath := flag.String("output", ".tmp/golden-path-001/registry.json", "generated registry path")
	flag.Parse()
	if err := run(*fixtureDir, *outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "golden-path-001 fixture: %v\n", err)
		os.Exit(1)
	}
}

func run(fixtureDir, outputPath string) error {
	tolerant, err := readJSONArtifact(filepath.Join(fixtureDir, "tolerant-profile.json"))
	if err != nil {
		return err
	}
	strict, err := readJSONArtifact(filepath.Join(fixtureDir, "strict-profile.json"))
	if err != nil {
		return err
	}
	workflow, err := os.ReadFile(filepath.Join(fixtureDir, "workflow.yaml"))
	if err != nil {
		return fmt.Errorf("read workflow: %w", err)
	}
	if len(workflow) == 0 {
		return fmt.Errorf("workflow is empty")
	}
	workflowRef, err := processor.NewWorkflowRevisionReference("workflow-adt", "workflow-1", workflow)
	if err != nil {
		return fmt.Errorf("build workflow reference: %w", err)
	}

	profiles := []struct {
		integrationID string
		artifactID    string
		raw           []byte
	}{
		{integrationID: "adt-tolerant", artifactID: "profile-adt-tolerant", raw: tolerant},
		{integrationID: "adt-strict", artifactID: "profile-adt-strict", raw: strict},
	}
	document := registryDocument{TenantID: "tenant-a", Integrations: make([]registryEntry, 0, len(profiles))}
	for _, profile := range profiles {
		profileRef, err := processor.NewProfileRevisionReference(profile.artifactID, 1, profile.raw)
		if err != nil {
			return fmt.Errorf("build %s reference: %w", profile.integrationID, err)
		}
		revision, err := definition(profile.integrationID, profileRef, workflowRef)
		if err != nil {
			return err
		}
		definitionJSON, err := json.Marshal(revision)
		if err != nil {
			return fmt.Errorf("marshal %s definition: %w", profile.integrationID, err)
		}
		document.Integrations = append(document.Integrations, registryEntry{
			IntegrationID: profile.integrationID,
			Definition:    definitionJSON,
			Profile:       profile.raw,
			Workflow:      string(workflow),
		})
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	if _, err := registry.DecodeStaticRegistry(bytes.NewReader(raw)); err != nil {
		return fmt.Errorf("self-validate registry: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(outputPath, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write registry: %w", err)
	}
	return nil
}

func readJSONArtifact(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	raw = bytes.TrimSpace(raw)
	if !json.Valid(raw) {
		return nil, fmt.Errorf("%s is not valid JSON", filepath.Base(path))
	}
	return raw, nil
}

func definition(
	integrationID string,
	profile integration.ArtifactRevisionRef,
	workflow integration.ArtifactRevisionRef,
) (integration.IntegrationDefinitionRevision, error) {
	digest := func(character string) string { return "sha256:" + strings.Repeat(character, 64) }
	return integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "integration-" + integrationID,
		RevisionID:   "definition-1",
		TenantID:     "tenant-a",
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: "source-adt-east", RevisionID: "source-1", Digest: digest("a"),
			},
			SourceID: "adt-east",
		},
		Format:   events.FormatHL7v2,
		Profile:  profile,
		Workflow: workflow,
		Destinations: []integration.DestinationRevisionRef{{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: "fhir-primary", RevisionID: "destination-1", Digest: digest("d"),
			},
			Class: integration.DestinationClassProduction,
		}},
		Policy: integration.IntegrationPolicy{
			Classification: integration.DataClassificationPHI,
			RawRetention:   integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral},
		},
		Created: integration.AuditEnvelope{
			TenantID: "tenant-a",
			Principal: integration.Principal{
				ID: "golden-publisher", Kind: integration.PrincipalKindHuman,
				AuthMethod: "oidc", Roles: []string{"publisher"},
			},
			Reason:     "publish Golden Path 001 artifacts",
			OccurredAt: time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC),
		},
	})
}
