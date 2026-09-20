package destination

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/fhirout"
)

// The FHIR transport's wire constants. Content negotiation is FHIR R4's
// (`application/fhir+json`); `Prefer: return=minimal` asks the destination not
// to echo the written resources back, which keeps the response small and
// keeps clinical content out of a body this process has to parse.
const (
	fhirContentType = "application/fhir+json; charset=utf-8"
	fhirAccept      = "application/fhir+json"
	fhirPrefer      = "return=minimal"

	// maxFHIRResponseBytes bounds how much of a FHIR response body is read for
	// classification. Unlike the https transport, this body is parsed — for
	// exactly two things, per-entry statuses and OperationOutcome issue codes —
	// so the bound is the ceiling on what a destination can make this process
	// decode. A body past the bound is treated as unparseable, never as a
	// reason to read further.
	maxFHIRResponseBytes = 1 << 18

	// maxOutcomeCodes bounds how many distinct issue codes are recorded.
	maxOutcomeCodes = 16
	// maxOutcomeCodeBytes bounds one sanitised issue code.
	maxOutcomeCodeBytes = 32

	// outcomeCodeResponseUnparsed is recorded in place of issue codes when the
	// destination's body was not a parseable FHIR response. It is this
	// process's own token, not something the destination said.
	outcomeCodeResponseUnparsed = "response-unparsed"
)

// deliverFHIR projects the stored canonical event into US Core resources and
// performs exactly one transaction against the destination's declared base URL
// under its declared identity.
//
// Trust posture is the https transport's, by construction: the client comes
// from newDestinationClient, so TLS floor, declared roots, no proxy, and
// redirect refusal are shared code rather than a copy. What differs is the
// body — a conditional transaction Bundle from fhirout, never the command
// envelope — and the response mapping, which has to read the body to learn
// whether a 200 committed every entry and which issue codes a refusal carried.
func (t *Transport) deliverFHIR(
	ctx context.Context,
	revision Revision,
	attemptID string,
	eventPayload []byte,
) deliveryResult {
	policy := *revision.FHIR

	// Projection first: no credential is resolved and no connection is opened
	// for an event that cannot be delivered as resources.
	eventType, err := fhirout.PayloadEventType(eventPayload)
	if err != nil {
		return t.failed(FailureProjection,
			"delivery event payload declares no canonical event type", false, "")
	}
	projection, err := fhirout.Project(eventType, eventPayload)
	if err != nil {
		return t.failed(FailureProjection, projectionFailureDetail(err), false, "")
	}
	bundle, err := fhirout.CreateConditionalTransactionBundle(projection)
	if err != nil {
		return t.failed(FailureProjection, projectionFailureDetail(err), false, "")
	}
	body, err := json.Marshal(bundle)
	if err != nil {
		return t.failed(FailureProjection,
			"projected FHIR bundle could not be encoded", false, "")
	}
	facts := deliveryResult{
		fhirResourceTypes: boundedFHIRLedgerText(strings.Join(projection.ResourceTypes(), ",")),
		fhirEntryCount:    len(bundle.Entry),
	}

	client, failure := t.newDestinationClient(ctx, policy.TokenBinding, policy.CABundleBinding)
	if failure != nil {
		return withFHIRFacts(*failure, facts)
	}
	defer client.close()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, policy.BaseURL, bytes.NewReader(body))
	if err != nil {
		return withFHIRFacts(t.failed(FailureUnconfigured,
			"destination endpoint is not a usable request target", false, ""), facts)
	}
	request.ContentLength = int64(len(body))
	request.Header.Set("Content-Type", fhirContentType)
	request.Header.Set("Accept", fhirAccept)
	request.Header.Set("Prefer", fhirPrefer)
	request.Header.Set("Authorization", client.authorization)
	// The same server-owned key the https transport sends. No FHIR server
	// honours it; the conditional entries are what make redelivery safe. It is
	// kept so an operator correlating destination logs sees one key per attempt
	// across both transports.
	request.Header.Set("Idempotency-Key", attemptID)

	response, err := client.client.Do(request)
	if err != nil {
		return withFHIRFacts(t.requestFailure(err), facts)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseDrainBytes))
		_ = response.Body.Close()
	}()

	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, maxFHIRResponseBytes+1))
	if len(responseBody) > maxFHIRResponseBytes {
		responseBody = nil
	}

	class := statusClass(response.StatusCode)
	served := servedCertificateSubjectAdvisory(response.TLS)
	var result deliveryResult
	switch {
	case response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices:
		result = t.classifyTransactionResponse(responseBody, len(bundle.Entry), class)
	case response.StatusCode == http.StatusRequestTimeout,
		response.StatusCode == http.StatusTooManyRequests,
		response.StatusCode >= http.StatusInternalServerError:
		result = t.failed(FailureUnavailable,
			"destination returned a retryable HTTP status class", true, class)
		result.fhirOutcomeCodes = operationOutcomeCodes(responseBody)
	default:
		result = t.failed(FailureRejected,
			"destination refused the FHIR transaction with a terminal HTTP status class", false, class)
		result.fhirOutcomeCodes = operationOutcomeCodes(responseBody)
	}
	result.servedCertificate = served
	return withFHIRFacts(result, facts)
}

// classifyTransactionResponse reads a 2xx transaction response for the one
// thing that matters: whether every entry committed. A transaction is
// all-or-nothing by specification, but a destination that answers 200 with a
// failing entry status is treated as a terminal refusal rather than a
// delivery, because the alternative is marking work published that was not.
func (t *Transport) classifyTransactionResponse(responseBody []byte, entries int, class string) deliveryResult {
	statuses, parsed := transactionEntryStatusClasses(responseBody)
	if !parsed {
		// A 2xx with a body this process cannot read is still a commit by the
		// destination's own account; it is recorded as delivered with the
		// unparsed marker so the ledger says the entry statuses were not seen.
		result := deliveryResult{statusClass: class, completedAt: t.clock().UTC()}
		result.fhirOutcomeCodes = outcomeCodeResponseUnparsed
		return result
	}
	failing := make([]string, 0)
	for _, status := range statuses {
		if status != "2xx" {
			failing = append(failing, "entry-"+status)
		}
	}
	if len(statuses) < entries {
		failing = append(failing, "entry-missing")
	}
	if len(failing) == 0 {
		return deliveryResult{statusClass: class, completedAt: t.clock().UTC()}
	}
	result := t.failed(FailureRejected,
		"destination committed the transaction with a non-2xx entry status", false, class)
	result.fhirOutcomeCodes = joinOutcomeCodes(failing)
	return result
}

// transactionEntryStatusClasses reduces a transaction-response Bundle to the
// status class of each entry. Nothing else in the body is read.
func transactionEntryStatusClasses(responseBody []byte) ([]string, bool) {
	if len(responseBody) == 0 {
		return nil, false
	}
	var bundle struct {
		ResourceType string `json:"resourceType"`
		Type         string `json:"type"`
		Entry        []struct {
			Response *struct {
				Status string `json:"status"`
			} `json:"response"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(responseBody, &bundle); err != nil ||
		bundle.ResourceType != "Bundle" || bundle.Type != "transaction-response" {
		return nil, false
	}
	classes := make([]string, 0, len(bundle.Entry))
	for _, entry := range bundle.Entry {
		if entry.Response == nil {
			classes = append(classes, "")
			continue
		}
		classes = append(classes, entryStatusClass(entry.Response.Status))
	}
	return classes, true
}

// entryStatusClass reduces a Bundle.entry.response.status ("201 Created") to
// the same closed vocabulary statusClass uses.
func entryStatusClass(status string) string {
	status = strings.TrimSpace(status)
	if len(status) < 3 {
		return ""
	}
	code := 0
	for _, character := range status[:3] {
		if character < '0' || character > '9' {
			return ""
		}
		code = code*10 + int(character-'0')
	}
	return statusClass(code)
}

// operationOutcomeCodes reduces an OperationOutcome body to its issue codes:
// sanitised to lowercase letters and hyphens, bounded, sorted, deduplicated.
// `diagnostics`, `details`, `expression`, and `location` are never read — a
// FHIR server's diagnostics echo the request, and the ledger holds no clinical
// content. A body that is not an OperationOutcome yields the unparsed marker.
func operationOutcomeCodes(responseBody []byte) string {
	if len(responseBody) == 0 {
		return ""
	}
	var outcome struct {
		ResourceType string `json:"resourceType"`
		Issue        []struct {
			Code string `json:"code"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(responseBody, &outcome); err != nil || outcome.ResourceType != "OperationOutcome" {
		return outcomeCodeResponseUnparsed
	}
	codes := make([]string, 0, len(outcome.Issue))
	for _, issue := range outcome.Issue {
		if code := sanitizeOutcomeCode(issue.Code); code != "" {
			codes = append(codes, code)
		}
	}
	return joinOutcomeCodes(codes)
}

// sanitizeOutcomeCode keeps only what a FHIR issue-type code is made of.
func sanitizeOutcomeCode(code string) string {
	var sanitized strings.Builder
	for _, character := range strings.ToLower(strings.TrimSpace(code)) {
		if sanitized.Len() >= maxOutcomeCodeBytes {
			break
		}
		if (character >= 'a' && character <= 'z') || character == '-' {
			sanitized.WriteRune(character)
		}
	}
	return sanitized.String()
}

func joinOutcomeCodes(codes []string) string {
	seen := make(map[string]struct{}, len(codes))
	unique := make([]string, 0, len(codes))
	for _, code := range codes {
		if code == "" {
			continue
		}
		if _, duplicate := seen[code]; duplicate {
			continue
		}
		seen[code] = struct{}{}
		unique = append(unique, code)
	}
	sort.Strings(unique)
	if len(unique) > maxOutcomeCodes {
		unique = unique[:maxOutcomeCodes]
	}
	return boundedFHIRLedgerText(strings.Join(unique, ","))
}

// boundedFHIRLedgerText truncates at the ledger's column bound, on a comma
// boundary where one exists so a partial token is not recorded.
func boundedFHIRLedgerText(value string) string {
	if len(value) <= maxFHIRLedgerBytes {
		return value
	}
	truncated := value[:maxFHIRLedgerBytes]
	if index := strings.LastIndex(truncated, ","); index > 0 {
		truncated = truncated[:index]
	}
	return truncated
}

// projectionFailureDetail maps a projection error to a bounded, catalog-safe
// detail. The error's own text is never used: a decode error can name a JSON
// member, and the DLQ detail must carry nothing derived from event content.
func projectionFailureDetail(err error) string {
	switch {
	case errors.Is(err, fhirout.ErrUnsupportedEventType):
		return "canonical event type has no FHIR projection"
	case errors.Is(err, fhirout.ErrNoUsableIdentifier):
		return "a projected FHIR resource has no usable identifier for a conditional write"
	default:
		return "canonical event payload could not be projected to FHIR"
	}
}

func withFHIRFacts(result deliveryResult, facts deliveryResult) deliveryResult {
	result.fhirResourceTypes = facts.fhirResourceTypes
	result.fhirEntryCount = facts.fhirEntryCount
	return result
}
