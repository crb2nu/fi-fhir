package companion

import (
	"fmt"
	"sync"
)

// Registry manages companion guides and provides lookup by various criteria.
type Registry struct {
	mu            sync.RWMutex
	guides        map[string]*CompanionGuide   // keyed by ID
	byReceiverID  map[string][]*CompanionGuide // keyed by receiver ID
	byPayerID     map[string][]*CompanionGuide // keyed by payer ID
	byTransaction map[string][]*CompanionGuide // keyed by transaction type
}

// NewRegistry creates a new companion guide registry.
func NewRegistry() *Registry {
	return &Registry{
		guides:        make(map[string]*CompanionGuide),
		byReceiverID:  make(map[string][]*CompanionGuide),
		byPayerID:     make(map[string][]*CompanionGuide),
		byTransaction: make(map[string][]*CompanionGuide),
	}
}

// Register adds a companion guide to the registry.
func (r *Registry) Register(guide *CompanionGuide) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if guide.ID == "" {
		return fmt.Errorf("guide ID is required")
	}

	if _, exists := r.guides[guide.ID]; exists {
		return fmt.Errorf("guide with ID %q already registered", guide.ID)
	}

	r.guides[guide.ID] = guide

	// Index by receiver IDs
	for _, receiverID := range guide.ReceiverIDs {
		r.byReceiverID[receiverID] = append(r.byReceiverID[receiverID], guide)
	}

	// Index by payer ID
	if guide.PayerID != "" {
		r.byPayerID[guide.PayerID] = append(r.byPayerID[guide.PayerID], guide)
	}

	// Index by transaction types
	for _, txType := range guide.TransactionTypes {
		r.byTransaction[txType] = append(r.byTransaction[txType], guide)
	}

	return nil
}

// Get retrieves a companion guide by ID.
func (r *Registry) Get(id string) *CompanionGuide {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.guides[id]
}

// GetByReceiverID retrieves a companion guide by X12 receiver ID.
// This is useful for auto-detecting the correct guide from the ISA/GS segments.
func (r *Registry) GetByReceiverID(receiverID string) *CompanionGuide {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if guides := r.byReceiverID[receiverID]; len(guides) > 0 {
		return guides[0]
	}
	return nil
}

// GetByReceiverIDAll retrieves all companion guides indexed by X12 receiver ID.
func (r *Registry) GetByReceiverIDAll(receiverID string) []*CompanionGuide {
	r.mu.RLock()
	defer r.mu.RUnlock()

	guides := r.byReceiverID[receiverID]
	if len(guides) == 0 {
		return nil
	}
	result := make([]*CompanionGuide, len(guides))
	copy(result, guides)
	return result
}

// GetByPayerID retrieves a companion guide by payer identifier.
func (r *Registry) GetByPayerID(payerID string) *CompanionGuide {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if guides := r.byPayerID[payerID]; len(guides) > 0 {
		return guides[0]
	}
	return nil
}

// GetByPayerIDAll retrieves all companion guides indexed by payer identifier.
func (r *Registry) GetByPayerIDAll(payerID string) []*CompanionGuide {
	r.mu.RLock()
	defer r.mu.RUnlock()

	guides := r.byPayerID[payerID]
	if len(guides) == 0 {
		return nil
	}
	result := make([]*CompanionGuide, len(guides))
	copy(result, guides)
	return result
}

// GetByTransactionType retrieves all companion guides for a transaction type.
func (r *Registry) GetByTransactionType(txType string) []*CompanionGuide {
	r.mu.RLock()
	defer r.mu.RUnlock()

	guides := r.byTransaction[txType]
	if len(guides) == 0 {
		return nil
	}

	// Return a copy to prevent modification
	result := make([]*CompanionGuide, len(guides))
	copy(result, guides)
	return result
}

// All returns all registered companion guides.
func (r *Registry) All() []*CompanionGuide {
	r.mu.RLock()
	defer r.mu.RUnlock()

	guides := make([]*CompanionGuide, 0, len(r.guides))
	for _, guide := range r.guides {
		guides = append(guides, guide)
	}
	return guides
}

// List returns the IDs of all registered companion guides.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.guides))
	for id := range r.guides {
		ids = append(ids, id)
	}
	return ids
}

// Unregister removes a companion guide from the registry.
func (r *Registry) Unregister(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	guide, exists := r.guides[id]
	if !exists {
		return false
	}

	delete(r.guides, id)

	// Remove from receiver ID index
	for _, receiverID := range guide.ReceiverIDs {
		guides := r.byReceiverID[receiverID]
		for i, g := range guides {
			if g.ID == id {
				guides = append(guides[:i], guides[i+1:]...)
				break
			}
		}
		if len(guides) == 0 {
			delete(r.byReceiverID, receiverID)
		} else {
			r.byReceiverID[receiverID] = guides
		}
	}

	// Remove from payer ID index
	if guide.PayerID != "" {
		guides := r.byPayerID[guide.PayerID]
		for i, g := range guides {
			if g.ID == id {
				guides = append(guides[:i], guides[i+1:]...)
				break
			}
		}
		if len(guides) == 0 {
			delete(r.byPayerID, guide.PayerID)
		} else {
			r.byPayerID[guide.PayerID] = guides
		}
	}

	// Remove from transaction type index
	for _, txType := range guide.TransactionTypes {
		guides := r.byTransaction[txType]
		for i, g := range guides {
			if g.ID == id {
				r.byTransaction[txType] = append(guides[:i], guides[i+1:]...)
				break
			}
		}
	}

	return true
}

// LoadAll loads all companion guides from a directory into the registry.
func (r *Registry) LoadAll(dir string) error {
	guides, err := LoadGuidesFromDirectory(dir)
	if err != nil {
		return err
	}

	for _, guide := range guides {
		if err := r.Register(guide); err != nil {
			return fmt.Errorf("failed to register guide %s: %w", guide.ID, err)
		}
	}

	return nil
}

// Detect attempts to automatically detect the correct companion guide.
//
// Matching priority is:
//  1. receiver ID (ISA08) + base guide (ST03/GS08) + transaction type (ST01)
//  2. payer ID (from transaction content) + base guide + transaction type
//  3. base guide match only
//  4. transaction type match only
func (r *Registry) Detect(receiverID, payerID, transactionType, baseGuide string) *CompanionGuide {
	best := (*CompanionGuide)(nil)
	bestScore := -1

	supports := func(g *CompanionGuide) bool {
		if baseGuide != "" && g.BaseGuide == baseGuide {
			return true
		}
		if transactionType != "" && contains(g.TransactionTypes, transactionType) {
			return true
		}
		return baseGuide == "" && transactionType == ""
	}

	scoreGuide := func(g *CompanionGuide, receiverMatch, payerMatch bool) int {
		score := 0
		if receiverMatch {
			score += 100
		}
		if payerMatch {
			score += 50
		}
		if baseGuide != "" && g.BaseGuide == baseGuide {
			score += 20
		}
		if transactionType != "" && contains(g.TransactionTypes, transactionType) {
			score += 10
		}
		// Prefer more-specific guides when we only know ST01 (e.g., "837")
		if transactionType == "837" && contains(g.TransactionTypes, "837P") && g.BaseGuide != "" {
			score += 1
		}
		return score
	}

	candidates := make([]*CompanionGuide, 0)
	if receiverID != "" {
		candidates = append(candidates, r.GetByReceiverIDAll(receiverID)...)
	}
	if payerID != "" {
		candidates = append(candidates, r.GetByPayerIDAll(payerID)...)
	}

	// Evaluate receiver/payer candidates first.
	filtered := candidates
	if baseGuide != "" || transactionType != "" {
		filtered = filtered[:0]
		for _, g := range candidates {
			if supports(g) {
				filtered = append(filtered, g)
			}
		}
		// If nothing matches the provided tx/base guide, fall back to all receiver/payer candidates.
		if len(filtered) == 0 {
			filtered = candidates
		}
	}

	for _, g := range filtered {
		receiverMatch := receiverID != "" && contains(g.ReceiverIDs, receiverID)
		payerMatch := payerID != "" && g.PayerID == payerID
		s := scoreGuide(g, receiverMatch, payerMatch)
		if s > bestScore {
			best = g
			bestScore = s
		}
	}

	// If we found a strong match (receiver/payer), return it.
	if best != nil {
		return best
	}

	// Fall back to base guide, then transaction type.
	if baseGuide != "" {
		for _, g := range r.All() {
			if g.BaseGuide == baseGuide {
				return g
			}
		}
	}
	if transactionType != "" {
		if guides := r.GetByTransactionType(transactionType); len(guides) > 0 {
			return guides[0]
		}
	}
	return nil
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// Count returns the number of registered guides.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.guides)
}

// DefaultRegistry is a package-level registry for convenience.
var DefaultRegistry = NewRegistry()

// RegisterGuide registers a guide with the default registry.
func RegisterGuide(guide *CompanionGuide) error {
	return DefaultRegistry.Register(guide)
}

// GetGuide retrieves a guide from the default registry.
func GetGuide(id string) *CompanionGuide {
	return DefaultRegistry.Get(id)
}

// DetectGuide auto-detects a guide using the default registry.
func DetectGuide(receiverID, payerID, transactionType string) *CompanionGuide {
	return DefaultRegistry.Detect(receiverID, payerID, transactionType, "")
}

// DetectGuideWithBaseGuide auto-detects a guide using the default registry including ST03/GS08 base guide.
func DetectGuideWithBaseGuide(receiverID, payerID, transactionType, baseGuide string) *CompanionGuide {
	return DefaultRegistry.Detect(receiverID, payerID, transactionType, baseGuide)
}
