package main

import (
	"testing"
)

// =============================================================================
// ETL Command Tests
// =============================================================================

func TestETL_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "etl")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir etl - ETL Pipeline")
	assertContains(t, stdout, "sync")
	assertContains(t, stdout, "fetch")
	assertContains(t, stdout, "load")
	assertContains(t, stdout, "status")
	assertContains(t, stdout, "validate")
	assertContains(t, stdout, "sources")
}

func TestETL_HelpFlag(t *testing.T) {
	stdout, _, err := runCLI(t, "etl", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir etl - ETL Pipeline")
}

func TestETL_HelpSubcommand(t *testing.T) {
	stdout, _, err := runCLI(t, "etl", "help")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir etl - ETL Pipeline")
}

func TestETL_UnknownSubcommand(t *testing.T) {
	_, _, err := runCLI(t, "etl", "badcmd")
	assertError(t, err)
	assertErrorContains(t, err, "unknown etl subcommand")
}

func TestETL_Fetch_MissingArgs(t *testing.T) {
	// fetch with no args returns an error with usage message
	_, _, err := runCLI(t, "etl", "fetch")
	assertError(t, err)
	assertErrorContains(t, err, "usage:")
}

func TestETL_FetchTest_MissingArgs(t *testing.T) {
	// fetch-test with no args prints help and returns nil
	stdout, _, err := runCLI(t, "etl", "fetch-test")
	assertNoError(t, err)
	assertContains(t, stdout, "Usage:")
	assertContains(t, stdout, "Available Test Data Sources:")
}

func TestETL_Load_MissingArgs(t *testing.T) {
	// load with no args prints help and returns nil
	stdout, _, err := runCLI(t, "etl", "load")
	assertNoError(t, err)
	assertContains(t, stdout, "Usage:")
}

func TestETL_Sources_ListsSources(t *testing.T) {
	stdout, _, err := runCLI(t, "etl", "sources")
	assertNoError(t, err)
	// Should list available sources with NAME header
	assertContains(t, stdout, "NAME")
	assertContains(t, stdout, "TYPE")
	assertContains(t, stdout, "icd10cm")
}
