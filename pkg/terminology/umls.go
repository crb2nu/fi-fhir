// Package terminology provides code system mapping for healthcare terminologies.

package terminology

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// UMLS API endpoints
const (
	UMLSAuthURL = "https://utslogin.nlm.nih.gov/cas/v1/api-key"
	UMLSBaseURL = "https://uts-ws.nlm.nih.gov/rest"
)

// UMLS source vocabularies (SAB = Source Abbreviation)
const (
	SAB_SNOMEDCT = "SNOMEDCT_US" // SNOMED CT US Edition
	SAB_ICD10CM  = "ICD10CM"     // ICD-10-CM diagnoses
	SAB_ICD10PCS = "ICD10PCS"    // ICD-10-PCS procedures
	SAB_RXNORM   = "RXNORM"      // RxNorm medications
	SAB_LOINC    = "LNC"         // LOINC lab codes
	SAB_CPT      = "CPT"         // CPT procedure codes
	SAB_HCPCS    = "HCPCS"       // HCPCS billing codes
	SAB_CVX      = "CVX"         // CVX vaccine codes
	SAB_NDC      = "MTHSPL"      // NDC drug codes (via MTHSPL)
)

// UMLSClient provides access to the UMLS Terminology Services API.
// It handles authentication, caching, and rate limiting.
type UMLSClient struct {
	apiKey     string
	httpClient *http.Client

	// Authentication state
	tgt       string // Ticket Granting Ticket
	tgtExpiry time.Time
	authMu    sync.Mutex

	// Cache for cross-walk results
	cache   map[string]*CrossWalkResult
	cacheMu sync.RWMutex

	// Rate limiting
	limiter    *rateLimiter
	maxRetries int
}

// UMLSClientOption configures the UMLS client.
type UMLSClientOption func(*UMLSClient)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) UMLSClientOption {
	return func(c *UMLSClient) {
		c.httpClient = client
	}
}

// WithMaxRetries sets the maximum number of retries for failed requests.
func WithMaxRetries(n int) UMLSClientOption {
	return func(c *UMLSClient) {
		c.maxRetries = n
	}
}

// WithRateLimit sets the rate limit (requests per second).
func WithRateLimit(rps float64) UMLSClientOption {
	return func(c *UMLSClient) {
		c.limiter = newRateLimiter(rps)
	}
}

// NewUMLSClient creates a new UMLS API client.
// The API key can be obtained from https://uts.nlm.nih.gov/uts/profile
func NewUMLSClient(apiKey string, opts ...UMLSClientOption) *UMLSClient {
	c := &UMLSClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache:      make(map[string]*CrossWalkResult),
		limiter:    newRateLimiter(15), // Default 15 req/sec (UMLS limit is 20)
		maxRetries: 3,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// rateLimiter implements a simple token bucket rate limiter.
type rateLimiter struct {
	rate     float64
	tokens   float64
	maxBurst float64
	lastTime time.Time
	mu       sync.Mutex
}

func newRateLimiter(rps float64) *rateLimiter {
	return &rateLimiter{
		rate:     rps,
		tokens:   rps, // Start with full bucket
		maxBurst: rps * 2,
		lastTime: time.Now(),
	}
}

func (r *rateLimiter) wait(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastTime).Seconds()
	r.tokens += elapsed * r.rate
	if r.tokens > r.maxBurst {
		r.tokens = r.maxBurst
	}
	r.lastTime = now

	if r.tokens >= 1 {
		r.tokens--
		return nil
	}

	// Need to wait
	waitTime := time.Duration((1 - r.tokens) / r.rate * float64(time.Second))
	select {
	case <-time.After(waitTime):
		r.tokens = 0
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// getTicketGrantingTicket obtains or refreshes the TGT.
func (c *UMLSClient) getTicketGrantingTicket(ctx context.Context) (string, error) {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	// Check if we have a valid TGT (they last 8 hours, but refresh at 7)
	if c.tgt != "" && time.Now().Before(c.tgtExpiry) {
		return c.tgt, nil
	}

	// Request new TGT
	data := url.Values{}
	data.Set("apikey", c.apiKey)

	req, err := http.NewRequestWithContext(ctx, "POST", UMLSAuthURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create TGT request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request TGT: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("TGT request failed (status %d): %s", resp.StatusCode, string(body))
	}

	// TGT URL is in the Location header
	tgtURL := resp.Header.Get("Location")
	if tgtURL == "" {
		return "", fmt.Errorf("no TGT URL in response")
	}

	c.tgt = tgtURL
	c.tgtExpiry = time.Now().Add(7 * time.Hour) // Refresh before 8h expiry

	return c.tgt, nil
}

// getServiceTicket obtains a single-use service ticket.
func (c *UMLSClient) getServiceTicket(ctx context.Context) (string, error) {
	tgtURL, err := c.getTicketGrantingTicket(ctx)
	if err != nil {
		return "", err
	}

	data := url.Values{}
	data.Set("service", "http://umlsks.nlm.nih.gov")

	req, err := http.NewRequestWithContext(ctx, "POST", tgtURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create ST request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request ST: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ST request failed (status %d): %s", resp.StatusCode, string(body))
	}

	ticket, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read ST: %w", err)
	}

	return string(ticket), nil
}

// doRequest performs an authenticated API request with rate limiting and retries.
func (c *UMLSClient) doRequest(ctx context.Context, method, path string, params url.Values) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second //nolint:gosec // G115: attempt bounded by maxRetries (typically 3-5)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// Rate limit
		if err := c.limiter.wait(ctx); err != nil {
			return nil, err
		}

		// Get service ticket
		ticket, err := c.getServiceTicket(ctx)
		if err != nil {
			lastErr = err
			continue
		}

		// Build URL
		reqURL := UMLSBaseURL + path
		if params == nil {
			params = url.Values{}
		}
		params.Set("ticket", ticket)

		if method == "GET" && len(params) > 0 {
			reqURL += "?" + params.Encode()
		}

		req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			continue
		}
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close() // Response body close errors not actionable

		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("rate limited")
			continue
		}

		if resp.StatusCode == http.StatusUnauthorized {
			// Invalidate TGT and retry
			c.authMu.Lock()
			c.tgt = ""
			c.authMu.Unlock()
			lastErr = fmt.Errorf("unauthorized")
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
		}

		return body, nil
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", c.maxRetries+1, lastErr)
}

// CrossWalkResult represents the result of a cross-walk query.
type CrossWalkResult struct {
	SourceCode   string         `json:"source_code"`
	SourceSystem string         `json:"source_system"`
	TargetCodes  []CrossWalkHit `json:"target_codes"`
	QueryTime    time.Duration  `json:"query_time"`
	FromCache    bool           `json:"from_cache"`
}

// CrossWalkHit represents a single code mapping from UMLS.
type CrossWalkHit struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	RootSource  string `json:"root_source"`
	Equivalence string `json:"equivalence,omitempty"` // derived from relationship
	CUI         string `json:"cui,omitempty"`         // UMLS Concept Unique Identifier
}

// crossWalkResponse represents the UMLS API crosswalk response.
type crossWalkResponse struct {
	Result []struct {
		UI           string `json:"ui"`
		Name         string `json:"name"`
		RootSource   string `json:"rootSource"`
		AtomCount    int    `json:"atomCount"`
		Obsolete     string `json:"obsolete"`
		Suppressible string `json:"suppressible"`
	} `json:"result"`
	PageSize   int `json:"pageSize"`
	PageNumber int `json:"pageNumber"`
	PageCount  int `json:"pageCount"`
}

// CrossWalk finds equivalent codes in a target vocabulary for a given source code.
// This is the primary method for terminology translation (e.g., ICD-10-CM to SNOMED CT).
func (c *UMLSClient) CrossWalk(ctx context.Context, sourceCode, sourceVocab, targetVocab string) (*CrossWalkResult, error) {
	// Check cache
	cacheKey := fmt.Sprintf("%s:%s:%s", sourceVocab, sourceCode, targetVocab)
	c.cacheMu.RLock()
	if cached, ok := c.cache[cacheKey]; ok {
		c.cacheMu.RUnlock()
		result := *cached
		result.FromCache = true
		return &result, nil
	}
	c.cacheMu.RUnlock()

	start := time.Now()

	params := url.Values{}
	params.Set("targetSource", targetVocab)
	params.Set("pageSize", "100")

	path := fmt.Sprintf("/crosswalk/current/source/%s/%s", sourceVocab, url.PathEscape(sourceCode))

	body, err := c.doRequest(ctx, "GET", path, params)
	if err != nil {
		return nil, fmt.Errorf("crosswalk request failed: %w", err)
	}

	var resp crossWalkResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse crosswalk response: %w", err)
	}

	result := &CrossWalkResult{
		SourceCode:   sourceCode,
		SourceSystem: sourceVocab,
		TargetCodes:  make([]CrossWalkHit, 0, len(resp.Result)),
		QueryTime:    time.Since(start),
		FromCache:    false,
	}

	for _, r := range resp.Result {
		if r.Obsolete == "true" || r.Suppressible == "Y" {
			continue // Skip obsolete/suppressed codes
		}
		result.TargetCodes = append(result.TargetCodes, CrossWalkHit{
			Code:       r.UI,
			Name:       r.Name,
			RootSource: r.RootSource,
			CUI:        "", // Would need separate lookup
		})
	}

	// Cache result
	c.cacheMu.Lock()
	c.cache[cacheKey] = result
	c.cacheMu.Unlock()

	return result, nil
}

// ConceptInfo represents information about a UMLS concept.
type ConceptInfo struct {
	CUI             string              `json:"cui"`
	Name            string              `json:"name"`
	SemanticTypes   []string            `json:"semantic_types"`
	Definitions     []ConceptDefinition `json:"definitions"`
	Atoms           []ConceptAtom       `json:"atoms"`
	RelatedConcepts []RelatedConcept    `json:"related_concepts,omitempty"`
}

// ConceptDefinition represents a definition for a concept.
type ConceptDefinition struct {
	Value      string `json:"value"`
	RootSource string `json:"root_source"`
}

// ConceptAtom represents a source-specific representation of a concept.
type ConceptAtom struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	RootSource string `json:"root_source"`
	TermType   string `json:"term_type"` // PT=Preferred Term, SY=Synonym, etc.
}

// RelatedConcept represents a concept related to the queried concept.
type RelatedConcept struct {
	CUI          string `json:"cui"`
	Name         string `json:"name"`
	Relationship string `json:"relationship"`
}

// conceptResponse represents the UMLS API concept response.
type conceptResponse struct {
	Result struct {
		ClassType     string `json:"classType"`
		UI            string `json:"ui"`
		Name          string `json:"name"`
		AtomCount     int    `json:"atomCount"`
		SemanticTypes []struct {
			Name string `json:"name"`
		} `json:"semanticTypes"`
		Definitions []struct {
			Value      string `json:"value"`
			RootSource string `json:"rootSource"`
		} `json:"definitions"`
		Atoms string `json:"atoms"` // URL to atoms endpoint
	} `json:"result"`
}

// atomsResponse represents the UMLS API atoms response.
type atomsResponse struct {
	Result []struct {
		UI         string `json:"ui"`
		Name       string `json:"name"`
		RootSource string `json:"rootSource"`
		TermType   string `json:"termType"`
		Code       string `json:"code"`
	} `json:"result"`
	PageCount int `json:"pageCount"`
}

// GetConcept retrieves detailed information about a UMLS concept by CUI.
func (c *UMLSClient) GetConcept(ctx context.Context, cui string) (*ConceptInfo, error) {
	path := fmt.Sprintf("/content/current/CUI/%s", cui)

	body, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("concept request failed: %w", err)
	}

	var resp conceptResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse concept response: %w", err)
	}

	info := &ConceptInfo{
		CUI:  resp.Result.UI,
		Name: resp.Result.Name,
	}

	for _, st := range resp.Result.SemanticTypes {
		info.SemanticTypes = append(info.SemanticTypes, st.Name)
	}

	for _, def := range resp.Result.Definitions {
		info.Definitions = append(info.Definitions, ConceptDefinition{
			Value:      def.Value,
			RootSource: def.RootSource,
		})
	}

	return info, nil
}

// GetConceptAtoms retrieves all source-specific representations (atoms) of a concept.
func (c *UMLSClient) GetConceptAtoms(ctx context.Context, cui string) ([]ConceptAtom, error) {
	var atoms []ConceptAtom
	page := 1

	for {
		params := url.Values{}
		params.Set("pageSize", "100")
		params.Set("pageNumber", fmt.Sprintf("%d", page))

		path := fmt.Sprintf("/content/current/CUI/%s/atoms", cui)

		body, err := c.doRequest(ctx, "GET", path, params)
		if err != nil {
			return nil, fmt.Errorf("atoms request failed: %w", err)
		}

		var resp atomsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("failed to parse atoms response: %w", err)
		}

		for _, a := range resp.Result {
			atoms = append(atoms, ConceptAtom{
				Code:       a.Code,
				Name:       a.Name,
				RootSource: a.RootSource,
				TermType:   a.TermType,
			})
		}

		if page >= resp.PageCount {
			break
		}
		page++
	}

	return atoms, nil
}

// SearchResult represents the result of a concept search.
type SearchResult struct {
	Concepts  []SearchHit   `json:"concepts"`
	PageSize  int           `json:"page_size"`
	PageCount int           `json:"page_count"`
	QueryTime time.Duration `json:"query_time"`
}

// SearchHit represents a single search result.
type SearchHit struct {
	CUI        string `json:"cui"`
	Name       string `json:"name"`
	RootSource string `json:"root_source"`
	URI        string `json:"uri,omitempty"`
}

// searchResponse represents the UMLS API search response.
type searchResponse struct {
	Result struct {
		Results []struct {
			UI         string `json:"ui"`
			Name       string `json:"name"`
			RootSource string `json:"rootSource"`
			URI        string `json:"uri"`
		} `json:"results"`
	} `json:"result"`
	PageSize  int `json:"pageSize"`
	PageCount int `json:"pageCount"`
}

// SearchOptions configures a concept search.
type SearchOptions struct {
	SearchType     string   // exact, words, leftTruncation, rightTruncation, normalizedString
	InputType      string   // atom, code, sourceConcept, sourceDescriptor, sourceUi
	IncludeSources []string // Filter to specific sources (e.g., SNOMEDCT_US, ICD10CM)
	PageSize       int
	PageNumber     int
}

// Search performs a concept search in UMLS.
func (c *UMLSClient) Search(ctx context.Context, term string, opts *SearchOptions) (*SearchResult, error) {
	start := time.Now()

	params := url.Values{}
	params.Set("string", term)

	if opts != nil {
		if opts.SearchType != "" {
			params.Set("searchType", opts.SearchType)
		}
		if opts.InputType != "" {
			params.Set("inputType", opts.InputType)
		}
		if len(opts.IncludeSources) > 0 {
			params.Set("sabs", strings.Join(opts.IncludeSources, ","))
		}
		if opts.PageSize > 0 {
			params.Set("pageSize", fmt.Sprintf("%d", opts.PageSize))
		}
		if opts.PageNumber > 0 {
			params.Set("pageNumber", fmt.Sprintf("%d", opts.PageNumber))
		}
	}

	body, err := c.doRequest(ctx, "GET", "/search/current", params)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}

	var resp searchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	result := &SearchResult{
		Concepts:  make([]SearchHit, 0, len(resp.Result.Results)),
		PageSize:  resp.PageSize,
		PageCount: resp.PageCount,
		QueryTime: time.Since(start),
	}

	for _, r := range resp.Result.Results {
		result.Concepts = append(result.Concepts, SearchHit{
			CUI:        r.UI,
			Name:       r.Name,
			RootSource: r.RootSource,
			URI:        r.URI,
		})
	}

	return result, nil
}

// NormalizeCode finds the canonical UMLS concept for a code from any source vocabulary.
// It returns the CUI and preferred name if found.
func (c *UMLSClient) NormalizeCode(ctx context.Context, code, sourceVocab string) (string, string, error) {
	params := url.Values{}
	params.Set("string", code)
	params.Set("inputType", "sourceUi")
	params.Set("sabs", sourceVocab)
	params.Set("searchType", "exact")
	params.Set("pageSize", "1")

	body, err := c.doRequest(ctx, "GET", "/search/current", params)
	if err != nil {
		return "", "", fmt.Errorf("normalize request failed: %w", err)
	}

	var resp searchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", fmt.Errorf("failed to parse normalize response: %w", err)
	}

	if len(resp.Result.Results) == 0 {
		return "", "", fmt.Errorf("code not found in UMLS: %s (%s)", code, sourceVocab)
	}

	result := resp.Result.Results[0]
	return result.UI, result.Name, nil
}

// ClearCache clears the cross-walk cache.
func (c *UMLSClient) ClearCache() {
	c.cacheMu.Lock()
	c.cache = make(map[string]*CrossWalkResult)
	c.cacheMu.Unlock()
}

// CacheSize returns the number of cached cross-walk results.
func (c *UMLSClient) CacheSize() int {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()
	return len(c.cache)
}

// ICD10ToSNOMED is a convenience method to translate ICD-10-CM codes to SNOMED CT.
func (c *UMLSClient) ICD10ToSNOMED(ctx context.Context, icd10Code string) (*CrossWalkResult, error) {
	return c.CrossWalk(ctx, icd10Code, SAB_ICD10CM, SAB_SNOMEDCT)
}

// SNOMEDToICD10 is a convenience method to translate SNOMED CT codes to ICD-10-CM.
func (c *UMLSClient) SNOMEDToICD10(ctx context.Context, snomedCode string) (*CrossWalkResult, error) {
	return c.CrossWalk(ctx, snomedCode, SAB_SNOMEDCT, SAB_ICD10CM)
}

// RxNormToNDC is a convenience method to translate RxNorm codes to NDC codes.
func (c *UMLSClient) RxNormToNDC(ctx context.Context, rxnormCode string) (*CrossWalkResult, error) {
	return c.CrossWalk(ctx, rxnormCode, SAB_RXNORM, SAB_NDC)
}

// LOINCLookup is a convenience method to find LOINC codes in UMLS.
func (c *UMLSClient) LOINCLookup(ctx context.Context, loincCode string) (*CrossWalkResult, error) {
	// For LOINC, we search for related concepts rather than cross-walk
	params := url.Values{}
	params.Set("string", loincCode)
	params.Set("inputType", "sourceUi")
	params.Set("sabs", SAB_LOINC)
	params.Set("searchType", "exact")

	body, err := c.doRequest(ctx, "GET", "/search/current", params)
	if err != nil {
		return nil, fmt.Errorf("LOINC lookup failed: %w", err)
	}

	var resp searchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse LOINC response: %w", err)
	}

	result := &CrossWalkResult{
		SourceCode:   loincCode,
		SourceSystem: SAB_LOINC,
		TargetCodes:  make([]CrossWalkHit, 0),
	}

	for _, r := range resp.Result.Results {
		result.TargetCodes = append(result.TargetCodes, CrossWalkHit{
			Code:       loincCode,
			Name:       r.Name,
			RootSource: r.RootSource,
			CUI:        r.UI,
		})
	}

	return result, nil
}
