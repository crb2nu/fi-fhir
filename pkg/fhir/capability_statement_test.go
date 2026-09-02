package fhir

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestSupportedResourceTypesMatchesMapperSurface(t *testing.T) {
	mapperType := reflect.TypeOf((*USCoreMapper)(nil))
	derivedSet := make(map[string]struct{})
	for i := 0; i < mapperType.NumMethod(); i++ {
		name := mapperType.Method(i).Name
		if !strings.HasPrefix(name, "Map") {
			continue
		}
		suffix := strings.TrimPrefix(name, "Map")
		resourceType, aliased := mapperResourceAliases[suffix]
		if !aliased {
			resourceType = suffix
		}
		derivedSet[resourceType] = struct{}{}
	}

	derived := make([]string, 0, len(derivedSet))
	for resourceType := range derivedSet {
		derived = append(derived, resourceType)
	}
	sort.Strings(derived)
	if got := SupportedResourceTypes(); !reflect.DeepEqual(got, derived) {
		t.Fatalf("SupportedResourceTypes() = %v, mapper surface = %v; add any non-literal Map* suffix to mapperResourceAliases", got, derived)
	}
}

func TestNewCapabilityStatement(t *testing.T) {
	date := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.FixedZone("test", -4*60*60))
	statement := NewCapabilityStatement(CapabilityStatementOptions{Date: date, SoftwareVersion: "1.2.3"})

	if statement.ResourceType != "CapabilityStatement" || statement.Status != "active" || statement.Kind != "instance" {
		t.Fatalf("unexpected identity fields: %#v", statement)
	}
	if statement.Date != "2026-09-02T16:30:00Z" || statement.FHIRVersion != "4.0.1" {
		t.Fatalf("unexpected version/date fields: %#v", statement)
	}
	if statement.Software.Name != "fi-fhir" || statement.Software.Version != "1.2.3" {
		t.Fatalf("unexpected software: %#v", statement.Software)
	}
	if !reflect.DeepEqual(statement.Format, []string{"application/fhir+json"}) {
		t.Fatalf("format = %v", statement.Format)
	}
	wantDescription := "Format-agnostic healthcare integration engine; resources are produced by the US Core mapper and delivered to configured destinations."
	if statement.Implementation.Description != wantDescription {
		t.Fatalf("implementation description = %q", statement.Implementation.Description)
	}
	if len(statement.Rest) != 1 || statement.Rest[0].Mode != "server" || len(statement.Rest[0].Resource) != len(SupportedResourceTypes()) {
		t.Fatalf("unexpected REST surface: %#v", statement.Rest)
	}
	wantProfiles := map[string][]string{
		"DiagnosticReport": {USCoreDiagnosticReportLabProfile, USCoreDiagnosticReportNoteProfile},
		"Observation": {
			USCoreBMIProfile,
			USCoreBloodPressureProfile,
			USCoreBodyHeightProfile,
			USCoreBodyTemperatureProfile,
			USCoreBodyWeightProfile,
			USCoreHeartRateProfile,
			USCoreObservationLabProfile,
			USCorePulseOximetryProfile,
			USCoreRespiratoryRateProfile,
			USCoreVitalSignsProfile,
		},
	}
	for i, resource := range statement.Rest[0].Resource {
		if resource.Type != SupportedResourceTypes()[i] {
			t.Fatalf("resource[%d].type = %q, want %q", i, resource.Type, SupportedResourceTypes()[i])
		}
		if !sort.StringsAreSorted(resource.SupportedProfile) {
			t.Fatalf("resource[%d] supportedProfile is not sorted: %v", i, resource.SupportedProfile)
		}
		if resource.Documentation != capabilityResourceDocumentation {
			t.Fatalf("resource[%d] documentation = %q", i, resource.Documentation)
		}
		if profiles, ok := wantProfiles[resource.Type]; ok {
			// The statement sorts profile URLs; compare as a set with the same
			// ordering rule so the expectation does not depend on how the
			// constants happen to be listed here.
			want := append([]string(nil), profiles...)
			sort.Strings(want)
			if !reflect.DeepEqual(resource.SupportedProfile, want) {
				t.Fatalf("resource[%d] supportedProfile = %v, want %v", i, resource.SupportedProfile, want)
			}
		}
		if (resource.Type == "Claim" || resource.Type == "ExplanationOfBenefit" || resource.Type == "CoverageEligibilityResponse") && len(resource.SupportedProfile) != 0 {
			t.Fatalf("resource[%d] overclaims supported profiles: %v", i, resource.SupportedProfile)
		}
	}

	encoded, err := json.Marshal(statement)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"interaction"`) {
		t.Fatalf("statement invents interactions: %s", encoded)
	}
}

func TestSupportedResourceTypesReturnsCopy(t *testing.T) {
	got := SupportedResourceTypes()
	got[0] = "mutated"
	if SupportedResourceTypes()[0] == "mutated" {
		t.Fatal("SupportedResourceTypes exposed mutable package state")
	}
}
