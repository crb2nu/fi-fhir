package projections

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/eventsourcing"
)

// ActiveEncounter represents a currently active patient encounter.
type ActiveEncounter struct {
	ID          string    `json:"id"`
	PatientMRN  string    `json:"patient_mrn"`
	PatientName string    `json:"patient_name,omitempty"`
	Class       string    `json:"class"` // inpatient, outpatient, emergency
	Location    string    `json:"location,omitempty"`
	Unit        string    `json:"unit,omitempty"`
	Room        string    `json:"room,omitempty"`
	Bed         string    `json:"bed,omitempty"`
	AdmitTime   time.Time `json:"admit_time"`
	Provider    string    `json:"provider,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	LastUpdated time.Time `json:"last_updated"`
}

// ActiveEncountersProjection maintains the list of currently active encounters.
type ActiveEncountersProjection struct {
	encounters map[string]*ActiveEncounter // encounterID -> encounter
	byLocation map[string][]string         // location -> encounterIDs
	byPatient  map[string]string           // patientMRN -> encounterID
	mu         sync.RWMutex
}

// NewActiveEncountersProjection creates a new active encounters projection.
func NewActiveEncountersProjection() *ActiveEncountersProjection {
	return &ActiveEncountersProjection{
		encounters: make(map[string]*ActiveEncounter),
		byLocation: make(map[string][]string),
		byPatient:  make(map[string]string),
	}
}

// Name returns the projection name.
func (p *ActiveEncountersProjection) Name() string {
	return "active_encounters"
}

// Handle processes an event and updates active encounters.
func (p *ActiveEncountersProjection) Handle(ctx context.Context, event eventsourcing.StoredEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch event.EventType {
	case "patient_admit":
		return p.handleAdmit(event)
	case "patient_discharge":
		return p.handleDischarge(event)
	case "patient_transfer":
		return p.handleTransfer(event)
	}

	return nil
}

func (p *ActiveEncountersProjection) handleAdmit(event eventsourcing.StoredEvent) error {
	var data struct {
		Patient struct {
			MRN        string `json:"mrn"`
			FamilyName string `json:"family_name"`
			GivenName  string `json:"given_name"`
		} `json:"patient"`
		Encounter struct {
			ID    string `json:"id"`
			Class string `json:"class"`
		} `json:"encounter"`
		Location struct {
			Facility string `json:"facility"`
			Unit     string `json:"unit"`
			Room     string `json:"room"`
			Bed      string `json:"bed"`
		} `json:"location"`
		Provider struct {
			FamilyName string `json:"family_name"`
			GivenName  string `json:"given_name"`
		} `json:"attending_provider"`
	}

	if err := json.Unmarshal(event.Data, &data); err != nil {
		// Try alternate format
		return p.handleAdmitAlternate(event)
	}

	encounterID := data.Encounter.ID
	if encounterID == "" {
		encounterID = event.StreamID
	}

	patientName := ""
	if data.Patient.GivenName != "" || data.Patient.FamilyName != "" {
		patientName = data.Patient.GivenName + " " + data.Patient.FamilyName
	}

	providerName := ""
	if data.Provider.GivenName != "" || data.Provider.FamilyName != "" {
		providerName = data.Provider.GivenName + " " + data.Provider.FamilyName
	}

	location := data.Location.Facility
	if location == "" {
		location = data.Location.Unit
	}

	encounter := &ActiveEncounter{
		ID:          encounterID,
		PatientMRN:  data.Patient.MRN,
		PatientName: patientName,
		Class:       data.Encounter.Class,
		Location:    location,
		Unit:        data.Location.Unit,
		Room:        data.Location.Room,
		Bed:         data.Location.Bed,
		AdmitTime:   event.Timestamp,
		Provider:    providerName,
		LastUpdated: time.Now(),
	}

	// Add to maps
	p.encounters[encounterID] = encounter
	if data.Patient.MRN != "" {
		p.byPatient[data.Patient.MRN] = encounterID
	}
	if location != "" {
		p.byLocation[location] = append(p.byLocation[location], encounterID)
	}

	return nil
}

func (p *ActiveEncountersProjection) handleAdmitAlternate(event eventsourcing.StoredEvent) error {
	// Handle simpler event format
	var data map[string]interface{}
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return nil //nolint:nilerr // Intentional: skip malformed events
	}

	encounterID := event.StreamID
	mrn := ""
	class := "inpatient"

	if id, ok := data["encounter_id"].(string); ok {
		encounterID = id
	}
	if m, ok := data["mrn"].(string); ok {
		mrn = m
	}
	if c, ok := data["class"].(string); ok {
		class = c
	}

	encounter := &ActiveEncounter{
		ID:          encounterID,
		PatientMRN:  mrn,
		Class:       class,
		AdmitTime:   event.Timestamp,
		LastUpdated: time.Now(),
	}

	p.encounters[encounterID] = encounter
	if mrn != "" {
		p.byPatient[mrn] = encounterID
	}

	return nil
}

func (p *ActiveEncountersProjection) handleDischarge(event eventsourcing.StoredEvent) error {
	var data struct {
		Patient struct {
			MRN string `json:"mrn"`
		} `json:"patient"`
		Encounter struct {
			ID string `json:"id"`
		} `json:"encounter"`
	}

	if err := json.Unmarshal(event.Data, &data); err != nil {
		return nil //nolint:nilerr // Intentional: skip malformed events
	}

	encounterID := data.Encounter.ID
	if encounterID == "" {
		encounterID = event.StreamID
	}

	// Remove encounter
	if enc, ok := p.encounters[encounterID]; ok {
		// Remove from byLocation
		if enc.Location != "" {
			ids := p.byLocation[enc.Location]
			for i, id := range ids {
				if id == encounterID {
					p.byLocation[enc.Location] = append(ids[:i], ids[i+1:]...)
					break
				}
			}
		}
		// Remove from byPatient
		delete(p.byPatient, enc.PatientMRN)
		// Remove from encounters
		delete(p.encounters, encounterID)
	}

	return nil
}

func (p *ActiveEncountersProjection) handleTransfer(event eventsourcing.StoredEvent) error {
	var data struct {
		Encounter struct {
			ID string `json:"id"`
		} `json:"encounter"`
		NewLocation struct {
			Facility string `json:"facility"`
			Unit     string `json:"unit"`
			Room     string `json:"room"`
			Bed      string `json:"bed"`
		} `json:"new_location"`
	}

	if err := json.Unmarshal(event.Data, &data); err != nil {
		return nil //nolint:nilerr // Intentional: skip malformed events
	}

	encounterID := data.Encounter.ID
	if encounterID == "" {
		encounterID = event.StreamID
	}

	enc, ok := p.encounters[encounterID]
	if !ok {
		return nil // Encounter not found
	}

	// Update location indexes
	if enc.Location != "" {
		ids := p.byLocation[enc.Location]
		for i, id := range ids {
			if id == encounterID {
				p.byLocation[enc.Location] = append(ids[:i], ids[i+1:]...)
				break
			}
		}
	}

	newLocation := data.NewLocation.Facility
	if newLocation == "" {
		newLocation = data.NewLocation.Unit
	}

	// Update encounter
	enc.Location = newLocation
	enc.Unit = data.NewLocation.Unit
	enc.Room = data.NewLocation.Room
	enc.Bed = data.NewLocation.Bed
	enc.LastUpdated = time.Now()

	if newLocation != "" {
		p.byLocation[newLocation] = append(p.byLocation[newLocation], encounterID)
	}

	return nil
}

// GetAllEncounters returns all active encounters.
func (p *ActiveEncountersProjection) GetAllEncounters() []ActiveEncounter {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]ActiveEncounter, 0, len(p.encounters))
	for _, enc := range p.encounters {
		result = append(result, *enc)
	}
	return result
}

// GetEncounter returns a specific encounter by ID.
func (p *ActiveEncountersProjection) GetEncounter(encounterID string) (*ActiveEncounter, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	enc, ok := p.encounters[encounterID]
	if !ok {
		return nil, false
	}
	encCopy := *enc
	return &encCopy, true
}

// GetEncounterByPatient returns the active encounter for a patient.
func (p *ActiveEncountersProjection) GetEncounterByPatient(mrn string) (*ActiveEncounter, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	encounterID, ok := p.byPatient[mrn]
	if !ok {
		return nil, false
	}

	enc, ok := p.encounters[encounterID]
	if !ok {
		return nil, false
	}
	encCopy := *enc
	return &encCopy, true
}

// GetEncountersByLocation returns encounters at a specific location.
func (p *ActiveEncountersProjection) GetEncountersByLocation(location string) []ActiveEncounter {
	p.mu.RLock()
	defer p.mu.RUnlock()

	encounterIDs, ok := p.byLocation[location]
	if !ok {
		return []ActiveEncounter{}
	}

	result := make([]ActiveEncounter, 0, len(encounterIDs))
	for _, id := range encounterIDs {
		if enc, ok := p.encounters[id]; ok {
			result = append(result, *enc)
		}
	}
	return result
}

// GetEncountersByClass returns encounters by class (inpatient, outpatient, etc).
func (p *ActiveEncountersProjection) GetEncountersByClass(class string) []ActiveEncounter {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]ActiveEncounter, 0)
	for _, enc := range p.encounters {
		if enc.Class == class {
			result = append(result, *enc)
		}
	}
	return result
}

// Count returns the number of active encounters.
func (p *ActiveEncountersProjection) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.encounters)
}

// Clear resets the projection (for testing/rebuilding).
func (p *ActiveEncountersProjection) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.encounters = make(map[string]*ActiveEncounter)
	p.byLocation = make(map[string][]string)
	p.byPatient = make(map[string]string)
}
