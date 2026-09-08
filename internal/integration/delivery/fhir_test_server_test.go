package delivery

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fhirTestServer is an in-test FHIR R4 endpoint with transaction-Bundle
// semantics over a resource store keyed on identifier.
//
// It exists for Slice 4.1c-c's second kill-test and for the inverted 5.1a
// gate. It is deliberately the *narrow* server a real FHIR server is, not a
// forgiving mock:
//
//   - `POST <Type>` always inserts. A server has no way to know that two POSTs
//     are the same patient, and this one does not pretend to. That is the
//     duplicate the outbox's at-least-once contract produces on redelivery.
//   - `PUT <Type>?identifier=<system>|<value>` is a conditional update: it
//     replaces the resource whose identifier matches, or creates it. This is
//     the standard FHIR answer to idempotent writes, and it is what the `fhir`
//     transport must emit.
//   - A conditional write or search whose identifier has no `system` — a bare
//     `?identifier=<value>` — is refused with 400. `?identifier=|MRN-1` matches
//     any system, so a server that accepts it would silently let two facilities'
//     MRNs collide. The refusal is here on day 1 so the projection's "no system
//     → projection error, never a POST" rule (lane riskiest assumption) is
//     exercised against a server that enforces it, not assumed.
//   - Transaction semantics are all-or-nothing: every entry is checked before
//     any is applied, and a failure leaves the store untouched and answers 400
//     with an OperationOutcome.
//   - `Idempotency-Key` is read for the record and honoured for nothing, which
//     is exactly what every FHIR server does with it.
//
// The response is a `transaction-response` Bundle with per-entry statuses and,
// under `Prefer: return=minimal`, no resource bodies.
type fhirTestServer struct {
	mu        sync.Mutex
	server    *httptest.Server
	resources map[string][]fhirStoredResource
	requests  []fhirServedRequest
	nextID    int

	// strictReferences makes the server behave like one with referential
	// integrity checking on, which is how HAPI ships: every `reference` inside a
	// transaction must be an entry fullUrl (`urn:uuid:…`, rewritten to the
	// created resource's `Type/id` on commit), a conditional reference
	// (`Type?identifier=system|value`, resolved against the store), or a literal
	// `Type/id` the store holds. A literal reference to an id the server never
	// issued — `Patient/MRN-000123`, which the mapper emits — is refused with 400.
	//
	// The check is scoped to the resource types this test suite writes (Patient,
	// Encounter, DiagnosticReport, Observation). A literal reference to any other
	// type (`Practitioner/<id>`) is accepted as-is, which is the documented v1
	// limitation: provider references are delivered literally.
	//
	// Off by default so the day-1 kill-test keeps measuring what it measured on
	// main; on for the inverted gate.
	strictReferences bool
}

// fhirBundleScopeTypes are the resource types strictReferences resolves.
var fhirBundleScopeTypes = map[string]bool{
	"Patient": true, "Encounter": true, "DiagnosticReport": true, "Observation": true,
}

// fhirStoredResource is one resource in the store.
type fhirStoredResource struct {
	ID       string
	Resource map[string]any
}

// fhirServedRequest is what the server saw for one HTTP request.
type fhirServedRequest struct {
	Method         string
	Path           string
	ContentType    string
	Accept         string
	Prefer         string
	IdempotencyKey string
	Body           []byte
	EntryMethods   []string
	EntryURLs      []string
	// EntryStatuses are the per-entry response statuses of an applied
	// transaction ("200 OK" for an in-place update, "201 Created" for an insert).
	EntryStatuses []string
	Status        int
}

// fhirTransactionEntry is the subset of a Bundle entry the server reads.
type fhirTransactionEntry struct {
	FullURL  string          `json:"fullUrl"`
	Resource json.RawMessage `json:"resource"`
	Request  *struct {
		Method string `json:"method"`
		URL    string `json:"url"`
	} `json:"request"`
}

type fhirTransactionBundle struct {
	ResourceType string                 `json:"resourceType"`
	Type         string                 `json:"type"`
	Entry        []fhirTransactionEntry `json:"entry"`
}

func newFHIRTestServer(t *testing.T) *fhirTestServer {
	t.Helper()
	server := &fhirTestServer{resources: map[string][]fhirStoredResource{}}
	server.server = httptest.NewUnstartedServer(http.HandlerFunc(server.serve))
	server.server.StartTLS()
	t.Cleanup(server.server.Close)
	return server
}

// URL is the FHIR base URL.
func (s *fhirTestServer) URL() string { return s.server.URL }

// CAPEM is the PEM-encoded certificate the server presents, for a destination
// revision's CA bundle binding.
func (s *fhirTestServer) CAPEM() string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: s.server.Certificate().Raw,
	}))
}

// Count reports how many resources of one type the store holds.
func (s *fhirTestServer) Count(resourceType string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.resources[resourceType])
}

// Requests returns every request served, in order.
func (s *fhirTestServer) Requests() []fhirServedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]fhirServedRequest(nil), s.requests...)
}

// Resources returns copies of every stored resource of one type, in insertion
// order.
func (s *fhirTestServer) Resources(resourceType string) []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]map[string]any, 0, len(s.resources[resourceType]))
	for _, stored := range s.resources[resourceType] {
		out = append(out, stored.Resource)
	}
	return out
}

func (s *fhirTestServer) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	served := fhirServedRequest{
		Method:         r.Method,
		Path:           r.URL.Path,
		ContentType:    r.Header.Get("Content-Type"),
		Accept:         r.Header.Get("Accept"),
		Prefer:         r.Header.Get("Prefer"),
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
		Body:           body,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	status, response := s.handleLocked(r, body, &served)
	served.Status = status
	s.requests = append(s.requests, served)

	w.Header().Set("Content-Type", "application/fhir+json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(response)
}

func (s *fhirTestServer) handleLocked(r *http.Request, body []byte, served *fhirServedRequest) (int, []byte) {
	path := strings.Trim(r.URL.Path, "/")
	switch {
	case r.Method == http.MethodPost && path == "":
		return s.transactionLocked(body, r.Header.Get("Prefer"), served)
	case r.Method == http.MethodGet && path != "" && !strings.Contains(path, "/"):
		return s.searchLocked(path, r.URL.Query())
	default:
		return http.StatusBadRequest, fhirOperationOutcome("not-supported",
			"this in-test server implements the transaction interaction at the base URL and identifier search only")
	}
}

// searchLocked answers `GET <Type>?identifier=…` with a searchset count, and
// refuses a bare-value identifier with 400.
func (s *fhirTestServer) searchLocked(resourceType string, query url.Values) (int, []byte) {
	identifier := query.Get("identifier")
	if identifier == "" {
		return http.StatusBadRequest, fhirOperationOutcome("invalid", "identifier search parameter is required")
	}
	system, value, ok := fhirSplitIdentifierToken(identifier)
	if !ok {
		return http.StatusBadRequest, fhirOperationOutcome("invalid",
			"identifier search requires system|value; a bare value matches any system and is refused")
	}
	matches := 0
	for _, stored := range s.resources[resourceType] {
		if fhirResourceHasIdentifier(stored.Resource, system, value) {
			matches++
		}
	}
	response, _ := json.Marshal(map[string]any{
		"resourceType": "Bundle", "type": "searchset", "total": matches,
	})
	return http.StatusOK, response
}

// transactionLocked applies one transaction Bundle atomically.
func (s *fhirTestServer) transactionLocked(body []byte, prefer string, served *fhirServedRequest) (int, []byte) {
	var bundle fhirTransactionBundle
	if err := json.Unmarshal(body, &bundle); err != nil {
		return http.StatusBadRequest, fhirOperationOutcome("structure", "request body is not a JSON Bundle")
	}
	if bundle.ResourceType != "Bundle" || bundle.Type != "transaction" {
		return http.StatusBadRequest, fhirOperationOutcome("invalid",
			fmt.Sprintf("expected a transaction Bundle, got resourceType=%q type=%q", bundle.ResourceType, bundle.Type))
	}
	if len(bundle.Entry) == 0 {
		return http.StatusBadRequest, fhirOperationOutcome("required", "transaction Bundle has no entries")
	}

	// Phase 1: validate every entry; nothing is written until all pass.
	type plannedWrite struct {
		fullURL      string
		resourceType string
		resource     map[string]any
		replaceIndex int // -1 means insert
		status       string
	}
	planned := make([]plannedWrite, 0, len(bundle.Entry))
	fullURLs := make(map[string]struct{}, len(bundle.Entry))
	for _, entry := range bundle.Entry {
		if entry.FullURL != "" {
			fullURLs[entry.FullURL] = struct{}{}
		}
	}
	for index, entry := range bundle.Entry {
		if entry.Request == nil {
			return http.StatusBadRequest, fhirOperationOutcome("required",
				fmt.Sprintf("entry[%d] has no request", index))
		}
		served.EntryMethods = append(served.EntryMethods, entry.Request.Method)
		served.EntryURLs = append(served.EntryURLs, entry.Request.URL)

		var resource map[string]any
		if err := json.Unmarshal(entry.Resource, &resource); err != nil || resource == nil {
			return http.StatusBadRequest, fhirOperationOutcome("structure",
				fmt.Sprintf("entry[%d] resource is not a JSON object", index))
		}
		resourceType, _ := resource["resourceType"].(string)
		if resourceType == "" {
			return http.StatusBadRequest, fhirOperationOutcome("required",
				fmt.Sprintf("entry[%d] resource has no resourceType", index))
		}
		target, err := url.Parse(entry.Request.URL)
		if err != nil {
			return http.StatusBadRequest, fhirOperationOutcome("invalid",
				fmt.Sprintf("entry[%d] request.url is not a URL", index))
		}
		targetPath := strings.Trim(target.Path, "/")
		if s.strictReferences {
			if problem := s.unresolvableReferenceLocked(resource, fullURLs); problem != "" {
				return http.StatusBadRequest, fhirOperationOutcome("invalid",
					fmt.Sprintf("entry[%d] %s", index, problem))
			}
		}
		switch entry.Request.Method {
		case http.MethodPost:
			if targetPath != resourceType || target.RawQuery != "" {
				return http.StatusBadRequest, fhirOperationOutcome("invalid",
					fmt.Sprintf("entry[%d] POST url must be exactly the resource type", index))
			}
			planned = append(planned, plannedWrite{fullURL: entry.FullURL, resourceType: resourceType, resource: resource, replaceIndex: -1, status: "201 Created"})
		case http.MethodPut:
			if targetPath != resourceType {
				return http.StatusBadRequest, fhirOperationOutcome("invalid",
					fmt.Sprintf("entry[%d] PUT url type %q does not match resource %q", index, targetPath, resourceType))
			}
			identifier := target.Query().Get("identifier")
			if identifier == "" {
				return http.StatusBadRequest, fhirOperationOutcome("not-supported",
					fmt.Sprintf("entry[%d] PUT without ?identifier= is not supported by this server", index))
			}
			system, value, ok := fhirSplitIdentifierToken(identifier)
			if !ok {
				return http.StatusBadRequest, fhirOperationOutcome("invalid",
					fmt.Sprintf("entry[%d] conditional update requires system|value; a bare value matches any system and is refused", index))
			}
			replaceIndex := -1
			for existingIndex, stored := range s.resources[resourceType] {
				if fhirResourceHasIdentifier(stored.Resource, system, value) {
					replaceIndex = existingIndex
					break
				}
			}
			status := "201 Created"
			if replaceIndex >= 0 {
				status = "200 OK"
			}
			planned = append(planned, plannedWrite{fullURL: entry.FullURL, resourceType: resourceType, resource: resource, replaceIndex: replaceIndex, status: status})
		default:
			return http.StatusBadRequest, fhirOperationOutcome("not-supported",
				fmt.Sprintf("entry[%d] request.method %q is not supported", index, entry.Request.Method))
		}
	}

	// Phase 2: apply.
	responseEntries := make([]map[string]any, 0, len(planned))
	assigned := make(map[string]string, len(planned)) // fullUrl → Type/id
	for _, write := range planned {
		var id string
		if write.replaceIndex >= 0 {
			id = s.resources[write.resourceType][write.replaceIndex].ID
			write.resource["id"] = id
			s.resources[write.resourceType][write.replaceIndex] = fhirStoredResource{ID: id, Resource: write.resource}
		} else {
			s.nextID++
			id = strconv.Itoa(s.nextID)
			write.resource["id"] = id
			s.resources[write.resourceType] = append(s.resources[write.resourceType], fhirStoredResource{ID: id, Resource: write.resource})
		}
		if write.fullURL != "" {
			assigned[write.fullURL] = write.resourceType + "/" + id
		}
		served.EntryStatuses = append(served.EntryStatuses, write.status)
		entry := map[string]any{
			"response": map[string]any{
				"status":   write.status,
				"location": write.resourceType + "/" + id + "/_history/1",
			},
		}
		if !strings.Contains(prefer, "return=minimal") {
			entry["resource"] = write.resource
		}
		responseEntries = append(responseEntries, entry)
	}
	// Phase 3: a real server rewrites intra-transaction and conditional
	// references to the ids it issued, so a stored Encounter points at the
	// stored Patient by `Patient/<id>`.
	if s.strictReferences {
		for _, write := range planned {
			s.resolveReferencesLocked(write.resource, assigned)
		}
	}
	response, _ := json.Marshal(map[string]any{
		"resourceType": "Bundle",
		"type":         "transaction-response",
		"entry":        responseEntries,
	})
	return http.StatusOK, response
}

// unresolvableReferenceLocked reports the first reference in the resource the
// server could not resolve under strictReferences, or "".
func (s *fhirTestServer) unresolvableReferenceLocked(resource map[string]any, fullURLs map[string]struct{}) string {
	var problem string
	walkFHIRReferences(resource, func(reference string) {
		if problem != "" {
			return
		}
		if _, inBundle := fullURLs[reference]; inBundle {
			return
		}
		if resourceType, query, conditional := strings.Cut(reference, "?"); conditional {
			values, err := url.ParseQuery(query)
			if err != nil {
				problem = fmt.Sprintf("conditional reference %q is not parseable", reference)
				return
			}
			system, value, ok := fhirSplitIdentifierToken(values.Get("identifier"))
			if !ok {
				problem = fmt.Sprintf("conditional reference %q has no system|value identifier", reference)
				return
			}
			if s.findByIdentifierLocked(resourceType, system, value) < 0 {
				problem = fmt.Sprintf("conditional reference %q matches no stored resource", reference)
			}
			return
		}
		resourceType, id, literal := strings.Cut(reference, "/")
		if !literal || !fhirBundleScopeTypes[resourceType] {
			return
		}
		for _, stored := range s.resources[resourceType] {
			if stored.ID == id {
				return
			}
		}
		problem = fmt.Sprintf("literal reference %q names an id this server never issued", reference)
	})
	return problem
}

// resolveReferencesLocked rewrites entry fullUrls and conditional references
// in a stored resource to the `Type/id` the server holds them under.
func (s *fhirTestServer) resolveReferencesLocked(resource map[string]any, assigned map[string]string) {
	rewriteFHIRReferences(resource, func(reference string) string {
		if target, inBundle := assigned[reference]; inBundle {
			return target
		}
		if resourceType, query, conditional := strings.Cut(reference, "?"); conditional {
			values, err := url.ParseQuery(query)
			if err != nil {
				return reference
			}
			system, value, ok := fhirSplitIdentifierToken(values.Get("identifier"))
			if !ok {
				return reference
			}
			if index := s.findByIdentifierLocked(resourceType, system, value); index >= 0 {
				return resourceType + "/" + s.resources[resourceType][index].ID
			}
		}
		return reference
	})
}

func (s *fhirTestServer) findByIdentifierLocked(resourceType, system, value string) int {
	for index, stored := range s.resources[resourceType] {
		if fhirResourceHasIdentifier(stored.Resource, system, value) {
			return index
		}
	}
	return -1
}

func walkFHIRReferences(value any, visit func(reference string)) {
	rewriteFHIRReferences(value, func(reference string) string {
		visit(reference)
		return reference
	})
}

func rewriteFHIRReferences(value any, rewrite func(reference string) string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "reference" {
				if reference, ok := child.(string); ok {
					typed[key] = rewrite(reference)
				}
				continue
			}
			rewriteFHIRReferences(child, rewrite)
		}
	case []any:
		for _, child := range typed {
			rewriteFHIRReferences(child, rewrite)
		}
	}
}

// fhirSplitIdentifierToken parses a FHIR token search value. It reports false
// for a bare value or an empty system, which this server refuses.
func fhirSplitIdentifierToken(token string) (system, value string, ok bool) {
	index := strings.Index(token, "|")
	if index <= 0 {
		return "", "", false
	}
	system, value = token[:index], token[index+1:]
	if system == "" || value == "" {
		return "", "", false
	}
	return system, value, true
}

func fhirResourceHasIdentifier(resource map[string]any, system, value string) bool {
	identifiers, _ := resource["identifier"].([]any)
	for _, raw := range identifiers {
		identifier, _ := raw.(map[string]any)
		gotSystem, _ := identifier["system"].(string)
		gotValue, _ := identifier["value"].(string)
		if gotSystem == system && gotValue == value {
			return true
		}
	}
	return false
}

func fhirOperationOutcome(code, diagnostics string) []byte {
	response, _ := json.Marshal(map[string]any{
		"resourceType": "OperationOutcome",
		"issue": []map[string]any{{
			"severity":    "error",
			"code":        code,
			"diagnostics": diagnostics,
		}},
	})
	return response
}
