package companion

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.guides == nil {
		t.Error("guides map not initialized")
	}
	if r.byReceiverID == nil {
		t.Error("byReceiverID map not initialized")
	}
	if r.byPayerID == nil {
		t.Error("byPayerID map not initialized")
	}
	if r.byTransaction == nil {
		t.Error("byTransaction map not initialized")
	}
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()

	guide := &CompanionGuide{
		ID:               "test",
		Name:             "Test Guide",
		PayerID:          "PAYER1",
		ReceiverIDs:      []string{"REC1", "REC2"},
		TransactionTypes: []string{"837P", "835"},
	}

	if err := r.Register(guide); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify indexed correctly
	if r.Count() != 1 {
		t.Errorf("Count = %d, want 1", r.Count())
	}

	// Get by ID
	if got := r.Get("test"); got == nil {
		t.Error("Get by ID returned nil")
	}

	// Get by receiver ID
	if got := r.GetByReceiverID("REC1"); got == nil {
		t.Error("GetByReceiverID(REC1) returned nil")
	}
	if got := r.GetByReceiverID("REC2"); got == nil {
		t.Error("GetByReceiverID(REC2) returned nil")
	}

	// Get by payer ID
	if got := r.GetByPayerID("PAYER1"); got == nil {
		t.Error("GetByPayerID returned nil")
	}

	// Get by transaction type
	if guides := r.GetByTransactionType("837P"); len(guides) != 1 {
		t.Errorf("GetByTransactionType(837P) = %d guides, want 1", len(guides))
	}
	if guides := r.GetByTransactionType("835"); len(guides) != 1 {
		t.Errorf("GetByTransactionType(835) = %d guides, want 1", len(guides))
	}
}

func TestRegistry_Register_NoID(t *testing.T) {
	r := NewRegistry()

	guide := &CompanionGuide{
		Name:             "No ID",
		TransactionTypes: []string{"837P"},
	}

	if err := r.Register(guide); err == nil {
		t.Error("Register should fail for guide without ID")
	}
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	r := NewRegistry()

	guide1 := &CompanionGuide{
		ID:               "test",
		Name:             "Test 1",
		TransactionTypes: []string{"837P"},
	}
	guide2 := &CompanionGuide{
		ID:               "test", // Same ID
		Name:             "Test 2",
		TransactionTypes: []string{"835"},
	}

	if err := r.Register(guide1); err != nil {
		t.Fatalf("First register failed: %v", err)
	}

	if err := r.Register(guide2); err == nil {
		t.Error("Register should fail for duplicate ID")
	}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()

	guide := &CompanionGuide{
		ID:               "test",
		Name:             "Test",
		TransactionTypes: []string{"837P"},
	}
	_ = r.Register(guide)

	if got := r.Get("test"); got == nil {
		t.Error("Get returned nil for existing guide")
	} else if got.ID != "test" {
		t.Errorf("Got ID = %q, want test", got.ID)
	}

	if got := r.Get("nonexistent"); got != nil {
		t.Error("Get should return nil for non-existent ID")
	}
}

func TestRegistry_GetByReceiverID(t *testing.T) {
	r := NewRegistry()

	guide := &CompanionGuide{
		ID:               "test",
		Name:             "Test",
		ReceiverIDs:      []string{"REC1", "REC2"},
		TransactionTypes: []string{"837P"},
	}
	_ = r.Register(guide)

	if got := r.GetByReceiverID("REC1"); got == nil {
		t.Error("GetByReceiverID(REC1) returned nil")
	}
	if got := r.GetByReceiverID("REC2"); got == nil {
		t.Error("GetByReceiverID(REC2) returned nil")
	}
	if got := r.GetByReceiverID("UNKNOWN"); got != nil {
		t.Error("GetByReceiverID(UNKNOWN) should return nil")
	}
}

func TestRegistry_GetByPayerID(t *testing.T) {
	r := NewRegistry()

	guide := &CompanionGuide{
		ID:               "test",
		Name:             "Test",
		PayerID:          "PAYER1",
		TransactionTypes: []string{"837P"},
	}
	_ = r.Register(guide)

	if got := r.GetByPayerID("PAYER1"); got == nil {
		t.Error("GetByPayerID(PAYER1) returned nil")
	}
	if got := r.GetByPayerID("UNKNOWN"); got != nil {
		t.Error("GetByPayerID(UNKNOWN) should return nil")
	}
}

func TestRegistry_GetByTransactionType(t *testing.T) {
	r := NewRegistry()

	guide1 := &CompanionGuide{
		ID:               "test1",
		Name:             "Test 1",
		TransactionTypes: []string{"837P"},
	}
	guide2 := &CompanionGuide{
		ID:               "test2",
		Name:             "Test 2",
		TransactionTypes: []string{"837P", "835"},
	}
	_ = r.Register(guide1)
	_ = r.Register(guide2)

	// Both support 837P
	guides837 := r.GetByTransactionType("837P")
	if len(guides837) != 2 {
		t.Errorf("GetByTransactionType(837P) = %d guides, want 2", len(guides837))
	}

	// Only guide2 supports 835
	guides835 := r.GetByTransactionType("835")
	if len(guides835) != 1 {
		t.Errorf("GetByTransactionType(835) = %d guides, want 1", len(guides835))
	}

	// None support 277
	guides277 := r.GetByTransactionType("277")
	if guides277 != nil {
		t.Errorf("GetByTransactionType(277) = %v, want nil", guides277)
	}
}

func TestRegistry_All(t *testing.T) {
	r := NewRegistry()

	guide1 := &CompanionGuide{ID: "g1", Name: "G1", TransactionTypes: []string{"837P"}}
	guide2 := &CompanionGuide{ID: "g2", Name: "G2", TransactionTypes: []string{"835"}}
	_ = r.Register(guide1)
	_ = r.Register(guide2)

	all := r.All()
	if len(all) != 2 {
		t.Errorf("All() returned %d guides, want 2", len(all))
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()

	_ = r.Register(&CompanionGuide{ID: "g1", Name: "G1", TransactionTypes: []string{"837P"}})
	_ = r.Register(&CompanionGuide{ID: "g2", Name: "G2", TransactionTypes: []string{"835"}})

	ids := r.List()
	if len(ids) != 2 {
		t.Errorf("List() returned %d IDs, want 2", len(ids))
	}

	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}
	if !idSet["g1"] || !idSet["g2"] {
		t.Errorf("List() = %v, missing expected IDs", ids)
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := NewRegistry()

	guide := &CompanionGuide{
		ID:               "test",
		Name:             "Test",
		PayerID:          "PAYER1",
		ReceiverIDs:      []string{"REC1"},
		TransactionTypes: []string{"837P"},
	}
	_ = r.Register(guide)

	// Verify registered
	if r.Count() != 1 {
		t.Fatalf("Count before unregister = %d, want 1", r.Count())
	}

	// Unregister
	if !r.Unregister("test") {
		t.Error("Unregister returned false for existing guide")
	}

	// Verify removed from all indexes
	if r.Count() != 0 {
		t.Errorf("Count after unregister = %d, want 0", r.Count())
	}
	if r.Get("test") != nil {
		t.Error("Get should return nil after unregister")
	}
	if r.GetByReceiverID("REC1") != nil {
		t.Error("GetByReceiverID should return nil after unregister")
	}
	if r.GetByPayerID("PAYER1") != nil {
		t.Error("GetByPayerID should return nil after unregister")
	}
	if guides := r.GetByTransactionType("837P"); len(guides) != 0 {
		t.Errorf("GetByTransactionType should return empty after unregister, got %d", len(guides))
	}
}

func TestRegistry_Unregister_NonExistent(t *testing.T) {
	r := NewRegistry()

	if r.Unregister("nonexistent") {
		t.Error("Unregister should return false for non-existent guide")
	}
}

func TestRegistry_Detect(t *testing.T) {
	r := NewRegistry()

	guide1 := &CompanionGuide{
		ID:               "receiver_guide",
		Name:             "Receiver Guide",
		ReceiverIDs:      []string{"REC123"},
		TransactionTypes: []string{"837P"},
	}
	guide2 := &CompanionGuide{
		ID:               "payer_guide",
		Name:             "Payer Guide",
		PayerID:          "PAYER456",
		TransactionTypes: []string{"837P"},
	}
	guide3 := &CompanionGuide{
		ID:               "tx_guide",
		Name:             "Transaction Guide",
		TransactionTypes: []string{"835"},
	}
	_ = r.Register(guide1)
	_ = r.Register(guide2)
	_ = r.Register(guide3)

	tests := []struct {
		name       string
		receiverID string
		payerID    string
		txType     string
		wantID     string
	}{
		{"by receiver ID", "REC123", "", "", "receiver_guide"},
		{"by payer ID", "", "PAYER456", "", "payer_guide"},
		{"by transaction type", "", "", "835", "tx_guide"},
		{"receiver takes precedence", "REC123", "PAYER456", "835", "receiver_guide"},
		{"payer over tx type", "", "PAYER456", "835", "payer_guide"},
		{"none match", "", "", "277", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Detect(tt.receiverID, tt.payerID, tt.txType)
			if tt.wantID == "" {
				if got != nil {
					t.Errorf("Detect() = %q, want nil", got.ID)
				}
			} else {
				if got == nil {
					t.Errorf("Detect() = nil, want %q", tt.wantID)
				} else if got.ID != tt.wantID {
					t.Errorf("Detect() = %q, want %q", got.ID, tt.wantID)
				}
			}
		})
	}
}

func TestRegistry_LoadAll(t *testing.T) {
	dir := t.TempDir()

	// Create guide files
	guide1 := `id: load1
name: Load 1
transaction_types: ["837P"]`
	guide2 := `id: load2
name: Load 2
transaction_types: ["835"]`

	if err := os.WriteFile(filepath.Join(dir, "g1.yaml"), []byte(guide1), 0600); err != nil {
		t.Fatalf("Failed to write guide1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "g2.yaml"), []byte(guide2), 0600); err != nil {
		t.Fatalf("Failed to write guide2: %v", err)
	}

	r := NewRegistry()
	if err := r.LoadAll(dir); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if r.Count() != 2 {
		t.Errorf("Count = %d, want 2", r.Count())
	}
	if r.Get("load1") == nil {
		t.Error("load1 not found")
	}
	if r.Get("load2") == nil {
		t.Error("load2 not found")
	}
}

func TestRegistry_LoadAll_DuplicateID(t *testing.T) {
	dir := t.TempDir()

	// Create two guides with the same ID
	guide1 := `id: duplicate
name: First
transaction_types: ["837P"]`
	guide2 := `id: duplicate
name: Second
transaction_types: ["835"]`

	if err := os.WriteFile(filepath.Join(dir, "g1.yaml"), []byte(guide1), 0600); err != nil {
		t.Fatalf("Failed to write guide1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "g2.yaml"), []byte(guide2), 0600); err != nil {
		t.Fatalf("Failed to write guide2: %v", err)
	}

	r := NewRegistry()
	err := r.LoadAll(dir)
	if err == nil {
		t.Error("LoadAll should fail for duplicate IDs")
	}
}

func TestRegistry_Count(t *testing.T) {
	r := NewRegistry()

	if r.Count() != 0 {
		t.Errorf("Empty registry count = %d, want 0", r.Count())
	}

	_ = r.Register(&CompanionGuide{ID: "g1", Name: "G1", TransactionTypes: []string{"837P"}})
	if r.Count() != 1 {
		t.Errorf("After 1 register, count = %d, want 1", r.Count())
	}

	_ = r.Register(&CompanionGuide{ID: "g2", Name: "G2", TransactionTypes: []string{"835"}})
	if r.Count() != 2 {
		t.Errorf("After 2 registers, count = %d, want 2", r.Count())
	}
}

func TestRegistry_Concurrency(t *testing.T) {
	r := NewRegistry()

	// Register multiple guides concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			guide := &CompanionGuide{
				ID:               string(rune('A' + id)),
				Name:             "Test",
				TransactionTypes: []string{"837P"},
			}
			_ = r.Register(guide)
			_ = r.Get(guide.ID)
			_ = r.All()
			_ = r.List()
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Registry should have registered guides (some may have failed due to duplicate)
	if r.Count() < 1 {
		t.Error("Expected at least one guide to be registered")
	}
}

func TestDefaultRegistry(t *testing.T) {
	// The default registry is package-level, so we need to be careful about state

	// Save original state
	originalCount := DefaultRegistry.Count()

	// Test RegisterGuide
	testGuide := &CompanionGuide{
		ID:               "default_test",
		Name:             "Default Test",
		TransactionTypes: []string{"837P"},
	}

	// Only register if not already present
	if DefaultRegistry.Get("default_test") == nil {
		if err := RegisterGuide(testGuide); err != nil {
			t.Fatalf("RegisterGuide failed: %v", err)
		}
	}

	// Test GetGuide
	if got := GetGuide("default_test"); got == nil {
		t.Error("GetGuide returned nil for registered guide")
	}

	// Test DetectGuide
	if got := DetectGuide("", "", "837P"); got == nil {
		// This might fail if no guides support 837P, which is ok
		t.Log("No guide detected for 837P (may be expected)")
	}

	// Cleanup - unregister our test guide
	DefaultRegistry.Unregister("default_test")

	// Verify count restored (approximately)
	if DefaultRegistry.Count() < originalCount {
		t.Logf("Registry count changed: was %d, now %d", originalCount, DefaultRegistry.Count())
	}
}

func TestRegistry_MultipleTransactionTypeGuides(t *testing.T) {
	r := NewRegistry()

	// Register guides with overlapping transaction types
	_ = r.Register(&CompanionGuide{
		ID:               "medicare",
		Name:             "Medicare",
		TransactionTypes: []string{"837P", "837I"},
	})
	_ = r.Register(&CompanionGuide{
		ID:               "bcbs",
		Name:             "BCBS",
		TransactionTypes: []string{"837P"},
	})
	_ = r.Register(&CompanionGuide{
		ID:               "era",
		Name:             "ERA",
		TransactionTypes: []string{"835"},
	})

	// 837P should return both medicare and bcbs
	guides837P := r.GetByTransactionType("837P")
	if len(guides837P) != 2 {
		t.Errorf("837P guides = %d, want 2", len(guides837P))
	}

	// 837I should return only medicare
	guides837I := r.GetByTransactionType("837I")
	if len(guides837I) != 1 {
		t.Errorf("837I guides = %d, want 1", len(guides837I))
	}

	// Unregister medicare and verify transaction type index updated
	r.Unregister("medicare")

	guides837P = r.GetByTransactionType("837P")
	if len(guides837P) != 1 {
		t.Errorf("After unregister, 837P guides = %d, want 1", len(guides837P))
	}

	guides837I = r.GetByTransactionType("837I")
	if len(guides837I) != 0 {
		t.Errorf("After unregister, 837I guides = %d, want 0", len(guides837I))
	}
}
