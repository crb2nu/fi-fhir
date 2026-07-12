package sqlutil

import (
	"strings"
	"testing"
)

func TestValidatePostgresIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		wantErr    bool
	}{
		{name: "simple", identifier: "events"},
		{name: "leading underscore", identifier: "_events_2026"},
		{name: "maximum length", identifier: strings.Repeat("e", 63)},
		{name: "empty", identifier: "", wantErr: true},
		{name: "uppercase", identifier: "MyEvents", wantErr: true},
		{name: "starts with digit", identifier: "2026_events", wantErr: true},
		{name: "schema qualified", identifier: "public.events", wantErr: true},
		{name: "contains whitespace", identifier: "events archive", wantErr: true},
		{name: "contains nul", identifier: "events\x00archive", wantErr: true},
		{name: "statement injection", identifier: "events;drop_table", wantErr: true},
		{name: "too long", identifier: strings.Repeat("e", 64), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePostgresIdentifier(tt.identifier)
			if tt.wantErr && err == nil {
				t.Fatalf("ValidatePostgresIdentifier(%q) expected error", tt.identifier)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidatePostgresIdentifier(%q) unexpected error: %v", tt.identifier, err)
			}
		})
	}
}
