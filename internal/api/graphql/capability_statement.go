package graphql

import (
	"encoding/json"
	"net/http"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
)

const CapabilityStatementPath = "/metadata"

func capabilityStatementHandler(startedAt time.Time, softwareVersion string) http.HandlerFunc {
	statement := fhir.NewCapabilityStatement(fhir.CapabilityStatementOptions{
		Date: startedAt, SoftwareVersion: softwareVersion,
	})
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "FHIR metadata requires GET", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/fhir+json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(statement); err != nil {
			http.Error(w, "could not encode FHIR metadata", http.StatusInternalServerError)
		}
	}
}
