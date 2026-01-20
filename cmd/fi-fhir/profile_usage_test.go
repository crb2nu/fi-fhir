package main

import "testing"

func TestProfile_NoArgs_PrintsUsage(t *testing.T) {
	stdout, _, err := runCLI(t, "profile")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir profile")
	assertContains(t, stdout, "infer")
	assertContains(t, stdout, "lint")
}

func TestProfile_Help_PrintsUsage(t *testing.T) {
	stdout, _, err := runCLI(t, "profile", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir profile")
}

func TestProfileInfer_Help_PrintsUsage(t *testing.T) {
	stdout, _, err := runCLI(t, "profile", "infer", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir profile infer")
	assertContains(t, stdout, "--id")
}

func TestProfileLint_Help_PrintsUsage(t *testing.T) {
	stdout, _, err := runCLI(t, "profile", "lint", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir profile lint")
	assertContains(t, stdout, "--profile")
}

func TestProfileMostCommon(t *testing.T) {
	val, cnt := profileMostCommon(map[string]int{"a": 1, "b": 3, "c": 2})
	if val != "b" || cnt != 3 {
		t.Fatalf("got (%q,%d), want (%q,%d)", val, cnt, "b", 3)
	}
}
