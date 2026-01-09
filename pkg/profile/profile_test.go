package profile

import "testing"

func TestLoadFromBytes(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		wantID  string
	}{
		{
			name: "minimal valid",
			yaml: `source_profile:
  id: test
  name: Test Profile
  version: "1.0"`,
			wantErr: false,
			wantID:  "test",
		},
		{
			name: "with hl7v2 config",
			yaml: `source_profile:
  id: epic_adt
  name: Epic ADT Feed
  version: "1.0"
  hl7v2:
    default_version: "2.5.1"
    timezone: "America/New_York"
    tolerate:
      missing_segments: ["PV1", "PD1"]
      nte_anywhere: true`,
			wantErr: false,
			wantID:  "epic_adt",
		},
		{
			name: "missing id",
			yaml: `source_profile:
  name: Test Profile`,
			wantErr: true,
		},
		{
			name: "missing name",
			yaml: `source_profile:
  id: test`,
			wantErr: true,
		},
		{
			name: "missing root element",
			yaml: `id: test
name: Test`,
			wantErr: true,
		},
		{
			name:    "invalid yaml",
			yaml:    `{invalid: yaml: here`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			p, err := r.LoadFromBytes([]byte(tt.yaml))

			if tt.wantErr {
				if err == nil {
					t.Errorf("LoadFromBytes() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("LoadFromBytes() unexpected error: %v", err)
				return
			}

			if p.ID != tt.wantID {
				t.Errorf("Profile ID = %q, want %q", p.ID, tt.wantID)
			}

			// Verify profile is registered
			found, ok := r.Get(tt.wantID)
			if !ok {
				t.Errorf("Profile not found in registry")
			}
			if found.ID != tt.wantID {
				t.Errorf("Retrieved profile ID = %q, want %q", found.ID, tt.wantID)
			}
		})
	}
}

func TestDefault(t *testing.T) {
	p := Default()

	if p.ID != "default" {
		t.Errorf("Default profile ID = %q, want 'default'", p.ID)
	}

	if p.HL7v2 == nil {
		t.Fatal("Default profile HL7v2 config is nil")
	}

	if p.HL7v2.DefaultVersion != "2.5.1" {
		t.Errorf("Default version = %q, want '2.5.1'", p.HL7v2.DefaultVersion)
	}

	if p.HL7v2.Tolerate == nil {
		t.Fatal("Default tolerance config is nil")
	}

	if len(p.HL7v2.Tolerate.MissingSegments) == 0 {
		t.Error("Default missing segments list is empty")
	}
}

func TestIsMissingSegmentTolerated(t *testing.T) {
	p := &SourceProfile{
		HL7v2: &HL7v2Config{
			Tolerate: &ToleranceConfig{
				MissingSegments: []string{"PV1", "PD1", "OBR"},
			},
		},
	}

	tests := []struct {
		segment string
		want    bool
	}{
		{"PV1", true},
		{"PD1", true},
		{"OBR", true},
		{"PID", false},
		{"MSH", false},
	}

	for _, tt := range tests {
		t.Run(tt.segment, func(t *testing.T) {
			got := p.IsMissingSegmentTolerated(tt.segment)
			if got != tt.want {
				t.Errorf("IsMissingSegmentTolerated(%q) = %v, want %v", tt.segment, got, tt.want)
			}
		})
	}
}

func TestGetEventClassification(t *testing.T) {
	p := Default()

	tests := []struct {
		name         string
		messageType  string
		patientClass string
		want         string
	}{
		{"A01 inpatient", "ADT^A01", "I", "inpatient_admit"},
		{"A01 outpatient", "ADT^A01", "O", "outpatient_registration"},
		{"A01 emergency", "ADT^A01", "E", "emergency_registration"},
		{"A01 preadmit", "ADT^A01", "P", "preadmit"},
		{"A01 recurring", "ADT^A01", "R", "recurring_patient"},
		{"A01 unknown class", "ADT^A01", "X", "patient_admit"}, // defaults
		{"A04", "ADT^A04", "O", "outpatient_registration"},
		{"unsupported message", "ORU^R01", "I", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.GetEventClassification(tt.messageType, tt.patientClass)
			if got != tt.want {
				t.Errorf("GetEventClassification(%q, %q) = %q, want %q",
					tt.messageType, tt.patientClass, got, tt.want)
			}
		})
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()

	// Empty registry
	if len(r.List()) != 0 {
		t.Errorf("Empty registry should have 0 profiles")
	}

	// Add profiles
	r.LoadFromBytes([]byte(`source_profile:
  id: profile1
  name: Profile 1
  version: "1.0"`))

	r.LoadFromBytes([]byte(`source_profile:
  id: profile2
  name: Profile 2
  version: "1.0"`))

	ids := r.List()
	if len(ids) != 2 {
		t.Errorf("Registry should have 2 profiles, got %d", len(ids))
	}
}

func TestGetAssigningAuthoritySystem(t *testing.T) {
	p := &SourceProfile{
		Identifiers: &IdentifierConfig{
			AssigningAuthorityMap: map[string]string{
				"HOSP_A":     "urn:oid:1.2.3.4.5",
				"SSA":        "urn:oid:2.16.840.1.113883.4.1",
				"ENTERPRISE": "urn:oid:1.2.3.4",
			},
		},
	}

	tests := []struct {
		authority string
		want      string
	}{
		{"HOSP_A", "urn:oid:1.2.3.4.5"},
		{"SSA", "urn:oid:2.16.840.1.113883.4.1"},
		{"UNKNOWN", ""},
	}

	for _, tt := range tests {
		t.Run(tt.authority, func(t *testing.T) {
			got := p.GetAssigningAuthoritySystem(tt.authority)
			if got != tt.want {
				t.Errorf("GetAssigningAuthoritySystem(%q) = %q, want %q",
					tt.authority, got, tt.want)
			}
		})
	}
}

func TestShouldValidateNPI(t *testing.T) {
	tests := []struct {
		name        string
		profile     *SourceProfile
		wantEnabled bool
		wantAction  string
	}{
		{
			name:        "nil identifiers",
			profile:     &SourceProfile{},
			wantEnabled: false,
			wantAction:  "pass",
		},
		{
			name: "enabled with warn",
			profile: &SourceProfile{
				Identifiers: &IdentifierConfig{
					Validation: map[string]*ValidatorConfig{
						"npi": {Enabled: true, OnInvalid: "warn"},
					},
				},
			},
			wantEnabled: true,
			wantAction:  "warn",
		},
		{
			name: "enabled with error",
			profile: &SourceProfile{
				Identifiers: &IdentifierConfig{
					Validation: map[string]*ValidatorConfig{
						"npi": {Enabled: true, OnInvalid: "error"},
					},
				},
			},
			wantEnabled: true,
			wantAction:  "error",
		},
		{
			name: "disabled",
			profile: &SourceProfile{
				Identifiers: &IdentifierConfig{
					Validation: map[string]*ValidatorConfig{
						"npi": {Enabled: false, OnInvalid: "warn"},
					},
				},
			},
			wantEnabled: false,
			wantAction:  "warn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabled, action := tt.profile.ShouldValidateNPI()
			if enabled != tt.wantEnabled {
				t.Errorf("ShouldValidateNPI() enabled = %v, want %v", enabled, tt.wantEnabled)
			}
			if action != tt.wantAction {
				t.Errorf("ShouldValidateNPI() action = %q, want %q", action, tt.wantAction)
			}
		})
	}
}
