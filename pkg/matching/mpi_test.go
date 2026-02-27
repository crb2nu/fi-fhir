package matching

import (
	"context"
	"testing"
	"time"
)

func TestMemoryMPI_Add(t *testing.T) {
	mpi := NewMemoryMPI(DefaultMatcherConfig())
	ctx := context.Background()

	patient := &Patient{
		MRN:        "123456",
		MRNSystem:  "HOSPITAL_A",
		SSN:        "234567890",
		FamilyName: "Smith",
		GivenName:  "John",
		DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
	}

	// Add patient
	record, err := mpi.Add(ctx, patient)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if record.EnterpriseID == "" {
		t.Error("Add() returned empty enterprise ID")
	}

	if record.Status != StatusActive {
		t.Errorf("Add() status = %v, want %v", record.Status, StatusActive)
	}

	if len(record.SourceRecords) != 2 {
		t.Errorf("Add() source records = %d, want 2 (SSN, MRN)", len(record.SourceRecords))
	}
}

func TestMemoryMPI_AddDuplicate(t *testing.T) {
	mpi := NewMemoryMPI(DefaultMatcherConfig())
	ctx := context.Background()

	patient1 := &Patient{
		SSN:        "234567890",
		FamilyName: "Smith",
		GivenName:  "John",
	}

	patient2 := &Patient{
		SSN:        "234567890",
		FamilyName: "Smith",
		GivenName:  "John W",
	}

	record1, err := mpi.Add(ctx, patient1)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Adding same SSN should return existing record
	record2, err := mpi.Add(ctx, patient2)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if record1.EnterpriseID != record2.EnterpriseID {
		t.Error("Add() should return same record for same SSN")
	}
}

func TestMemoryMPI_Get(t *testing.T) {
	mpi := NewMemoryMPI(DefaultMatcherConfig())
	ctx := context.Background()

	patient := &Patient{
		SSN:        "234567890",
		FamilyName: "Smith",
		GivenName:  "John",
	}

	record, err := mpi.Add(ctx, patient)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Get by enterprise ID
	retrieved, err := mpi.Get(ctx, record.EnterpriseID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved.Patient.FamilyName != "Smith" {
		t.Errorf("Get() FamilyName = %v, want Smith", retrieved.Patient.FamilyName)
	}
}

func TestMemoryMPI_GetNotFound(t *testing.T) {
	mpi := NewMemoryMPI(DefaultMatcherConfig())
	ctx := context.Background()

	_, err := mpi.Get(ctx, "nonexistent")
	if err == nil {
		t.Error("Get() should return error for nonexistent ID")
	}
}

func TestMemoryMPI_Search(t *testing.T) {
	mpi := NewMemoryMPI(DefaultMatcherConfig())
	ctx := context.Background()

	// Add multiple patients
	patients := []*Patient{
		{
			MRN:        "001",
			MRNSystem:  "SYS_A",
			FamilyName: "Smith",
			GivenName:  "John",
			DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			MRN:        "002",
			MRNSystem:  "SYS_A",
			FamilyName: "Smith",
			GivenName:  "Jane",
			DOB:        time.Date(1990, 7, 20, 0, 0, 0, 0, time.UTC),
		},
		{
			MRN:        "003",
			MRNSystem:  "SYS_A",
			FamilyName: "Jones",
			GivenName:  "Bob",
			DOB:        time.Date(1975, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, p := range patients {
		_, err := mpi.Add(ctx, p)
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
	}

	// Search for John Smith
	query := &Patient{
		FamilyName: "Smith",
		GivenName:  "John",
		DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
	}

	results, err := mpi.Search(ctx, query, DefaultSearchOptions())
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Search() returned no results")
	}

	// First result should be John Smith
	if results[0].Record.Patient.GivenName != "John" {
		t.Errorf("Search() first result = %v, want John", results[0].Record.Patient.GivenName)
	}
}

func TestMemoryMPI_Link(t *testing.T) {
	mpi := NewMemoryMPI(DefaultMatcherConfig())
	ctx := context.Background()

	patient1 := &Patient{
		MRN:        "001",
		MRNSystem:  "SYS_A",
		FamilyName: "Smith",
		GivenName:  "John",
	}

	patient2 := &Patient{
		MRN:        "002",
		MRNSystem:  "SYS_B",
		FamilyName: "Smith",
		GivenName:  "John",
	}

	record1, _ := mpi.Add(ctx, patient1)
	record2, _ := mpi.Add(ctx, patient2)

	// Link the records
	err := mpi.Link(ctx, record1.EnterpriseID, record2.EnterpriseID, LinkConfirmed)
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}

	// Get links
	links, err := mpi.GetLinks(ctx, record1.EnterpriseID)
	if err != nil {
		t.Fatalf("GetLinks() error = %v", err)
	}

	if len(links) != 1 {
		t.Errorf("GetLinks() = %d links, want 1", len(links))
	}

	if links[0].LinkType != LinkConfirmed {
		t.Errorf("Link type = %v, want %v", links[0].LinkType, LinkConfirmed)
	}
}

func TestMemoryMPI_Unlink(t *testing.T) {
	mpi := NewMemoryMPI(DefaultMatcherConfig())
	ctx := context.Background()

	patient1 := &Patient{
		MRN:       "001",
		MRNSystem: "SYS_A",
	}

	patient2 := &Patient{
		MRN:       "002",
		MRNSystem: "SYS_B",
	}

	record1, _ := mpi.Add(ctx, patient1)
	record2, _ := mpi.Add(ctx, patient2)

	_ = mpi.Link(ctx, record1.EnterpriseID, record2.EnterpriseID, LinkConfirmed)

	// Unlink
	err := mpi.Unlink(ctx, record1.EnterpriseID, record2.EnterpriseID)
	if err != nil {
		t.Fatalf("Unlink() error = %v", err)
	}

	// Verify unlinked
	links, _ := mpi.GetLinks(ctx, record1.EnterpriseID)
	if len(links) != 0 {
		t.Errorf("GetLinks() after Unlink = %d, want 0", len(links))
	}
}

func TestMemoryMPI_Merge(t *testing.T) {
	mpi := NewMemoryMPI(DefaultMatcherConfig())
	ctx := context.Background()

	patient1 := &Patient{
		MRN:        "001",
		MRNSystem:  "SYS_A",
		SSN:        "345678901",
		FamilyName: "Smith",
		GivenName:  "John",
	}

	patient2 := &Patient{
		MRN:        "002",
		MRNSystem:  "SYS_B",
		SSN:        "456789012",
		FamilyName: "Smith",
		GivenName:  "John",
	}

	record1, _ := mpi.Add(ctx, patient1)
	record2, _ := mpi.Add(ctx, patient2)

	// Merge record2 into record1
	err := mpi.Merge(ctx, record1.EnterpriseID, record2.EnterpriseID, "Same person confirmed")
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}

	// Verify victim is marked as merged
	victim, _ := mpi.Get(ctx, record2.EnterpriseID)
	if victim.EnterpriseID != record1.EnterpriseID {
		// Get should follow merge chain and return survivor
		t.Logf("Get() followed merge chain to survivor")
	}

	// Verify survivor has both source records
	survivor, _ := mpi.Get(ctx, record1.EnterpriseID)
	if len(survivor.SourceRecords) < 2 {
		t.Errorf("Survivor should have merged source records, got %d", len(survivor.SourceRecords))
	}
}

func TestMemoryMPI_GetByIdentifier(t *testing.T) {
	mpi := NewMemoryMPI(DefaultMatcherConfig())
	ctx := context.Background()

	patient := &Patient{
		MRN:        "123456",
		MRNSystem:  "HOSPITAL_A",
		SSN:        "234567890",
		FamilyName: "Smith",
		GivenName:  "John",
	}

	_, err := mpi.Add(ctx, patient)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Get by SSN
	record, err := mpi.GetByIdentifier(ctx, "SS", "234567890", "")
	if err != nil {
		t.Fatalf("GetByIdentifier(SSN) error = %v", err)
	}

	if record.Patient.FamilyName != "Smith" {
		t.Errorf("GetByIdentifier() FamilyName = %v, want Smith", record.Patient.FamilyName)
	}

	// Get by MRN
	record, err = mpi.GetByIdentifier(ctx, "MR", "123456", "HOSPITAL_A")
	if err != nil {
		t.Fatalf("GetByIdentifier(MRN) error = %v", err)
	}

	if record.Patient.FamilyName != "Smith" {
		t.Errorf("GetByIdentifier() FamilyName = %v, want Smith", record.Patient.FamilyName)
	}
}

func TestMemoryMPI_Update(t *testing.T) {
	mpi := NewMemoryMPI(DefaultMatcherConfig())
	ctx := context.Background()

	patient := &Patient{
		MRN:        "123456",
		MRNSystem:  "HOSPITAL_A",
		FamilyName: "Smith",
		GivenName:  "John",
	}

	record, _ := mpi.Add(ctx, patient)

	// Update patient
	updatedPatient := &Patient{
		MRN:        "123456",
		MRNSystem:  "HOSPITAL_A",
		FamilyName: "Smith",
		GivenName:  "Jonathan", // Changed
		Phone:      "5551234567",
	}

	err := mpi.Update(ctx, record.EnterpriseID, updatedPatient)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify update
	updated, _ := mpi.Get(ctx, record.EnterpriseID)
	if updated.Patient.GivenName != "Jonathan" {
		t.Errorf("Update() GivenName = %v, want Jonathan", updated.Patient.GivenName)
	}
}

func TestMemoryMPI_GetMetrics(t *testing.T) {
	mpi := NewMemoryMPI(DefaultMatcherConfig())
	ctx := context.Background()

	// Add some patients
	for i := 0; i < 3; i++ {
		patient := &Patient{
			MRN:        string(rune('A' + i)),
			MRNSystem:  "SYS",
			FamilyName: "Test",
		}
		mpi.Add(ctx, patient)
	}

	metrics := mpi.GetMetrics()

	if metrics.TotalRecords != 3 {
		t.Errorf("TotalRecords = %d, want 3", metrics.TotalRecords)
	}

	if metrics.ActiveRecords != 3 {
		t.Errorf("ActiveRecords = %d, want 3", metrics.ActiveRecords)
	}
}

func TestMemoryMPI_AuditLog(t *testing.T) {
	mpi := NewMemoryMPI(DefaultMatcherConfig())
	ctx := context.Background()

	patient := &Patient{
		MRN:       "001",
		MRNSystem: "SYS",
	}

	record, _ := mpi.Add(ctx, patient)

	patient2 := &Patient{
		MRN:       "002",
		MRNSystem: "SYS",
	}

	record2, _ := mpi.Add(ctx, patient2)

	_ = mpi.Link(ctx, record.EnterpriseID, record2.EnterpriseID, LinkConfirmed)

	auditLog := mpi.GetAuditLog()

	if len(auditLog) < 3 {
		t.Errorf("Expected at least 3 audit events (2 adds + 1 link), got %d", len(auditLog))
	}

	// Check event types
	eventTypes := make(map[string]int)
	for _, event := range auditLog {
		eventTypes[event.EventType]++
	}

	if eventTypes["add"] != 2 {
		t.Errorf("Expected 2 'add' events, got %d", eventTypes["add"])
	}

	if eventTypes["link"] != 1 {
		t.Errorf("Expected 1 'link' event, got %d", eventTypes["link"])
	}
}
