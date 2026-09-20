package fhirout

import (
	"net/url"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
)

func TestConditionalReferencePreservesIdentifier(t *testing.T) {
	for _, tt := range []struct{ name, system, value, wantToken string }{
		{"observation", "urn:source:lab", "ORD-123#obs-1", "urn:source:lab|ORD-123#obs-1"},
		{"query characters", "https://example.org/id?scope=a&site=b", "MRN+1&status=active%", "https://example.org/id?scope=a&site=b|MRN+1&status=active%"},
		{"FHIR delimiters", "urn:source:lab", "A,B$C\\D", "urn:source:lab|A\\,B\\$C\\\\D"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reference := ConditionalReference("Observation", fhir.Identifier{System: tt.system, Value: tt.value})
			parsed, err := url.Parse(reference)
			if err != nil {
				t.Fatal(err)
			}
			query, err := url.ParseQuery(parsed.RawQuery)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Path != "Observation" || parsed.Fragment != "" || len(query) != 1 || query.Get("identifier") != tt.wantToken {
				t.Fatalf("conditional URL changed identifier: %q -> path=%q fragment=%q query=%v", reference, parsed.Path, parsed.Fragment, query)
			}
		})
	}
}
