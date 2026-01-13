package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/crb2nu/fi-fhir/pkg/etl"
)

// UMLSSourceConfig configures UMLS download source.
type UMLSSourceConfig struct {
	Name            string
	APIKey          string
	BaseLoginURL    string
	BaseDownloadURL string
	// Versions maps release -> archive filename (e.g., "2024AB": "umls-2024AB-full.zip").
	Versions   map[string]string
	HTTPClient *http.Client
}

// UMLSSource implements etl.Source for UMLS downloads using the UTS ticket API.
type UMLSSource struct {
	name            string
	apiKey          string
	baseLoginURL    string
	baseDownloadURL string
	versions        map[string]string
	client          *http.Client
	latestVersion   string
}

// NewUMLSSource creates a new UMLS download source.
func NewUMLSSource(cfg UMLSSourceConfig) *UMLSSource {
	name := cfg.Name
	if name == "" {
		name = "umls"
	}
	baseLogin := cfg.BaseLoginURL
	if baseLogin == "" {
		baseLogin = "https://utslogin.nlm.nih.gov/cas/v1/api-key"
	}
	baseDownload := cfg.BaseDownloadURL
	if baseDownload == "" {
		baseDownload = "https://download.nlm.nih.gov/umls/kss"
	}
	versions := cfg.Versions
	if len(versions) == 0 {
		versions = map[string]string{
			"2024AB": "umls-2024AB-full.zip",
		}
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}

	latest := pickLatestVersion(versions)

	return &UMLSSource{
		name:            name,
		apiKey:          cfg.APIKey,
		baseLoginURL:    baseLogin,
		baseDownloadURL: baseDownload,
		versions:        versions,
		client:          client,
		latestVersion:   latest,
	}
}

// Name returns the source identifier.
func (s *UMLSSource) Name() string {
	return s.name
}

// AvailableVersions reports configured versions.
func (s *UMLSSource) AvailableVersions(ctx context.Context) ([]etl.VersionInfo, error) {
	_ = ctx
	var versions []string
	for v := range s.versions {
		versions = append(versions, v)
	}
	sort.Strings(versions)

	result := make([]etl.VersionInfo, 0, len(versions))
	for _, v := range versions {
		result = append(result, etl.VersionInfo{
			Version:     v,
			DownloadURL: s.downloadURL(v),
			IsLatest:    v == s.latestVersion,
		})
	}
	return result, nil
}

// Download streams the UMLS archive for the given version.
func (s *UMLSSource) Download(ctx context.Context, version string, w io.Writer) (int64, error) {
	if version == "" {
		version = s.latestVersion
	}
	filename, ok := s.versions[version]
	if !ok {
		return 0, fmt.Errorf("unknown UMLS version: %s", version)
	}
	downloadURL := s.downloadURL(version)

	tgt, err := s.requestTGT(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to obtain TGT: %w", err)
	}
	st, err := s.requestServiceTicket(ctx, tgt, downloadURL)
	if err != nil {
		return 0, fmt.Errorf("failed to obtain service ticket: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to build download request: %w", err)
	}
	q := req.URL.Query()
	q.Set("ticket", st)
	req.URL.RawQuery = q.Encode()

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download failed: %s", resp.Status)
	}

	n, err := io.Copy(w, resp.Body)
	if err != nil {
		return n, fmt.Errorf("failed to stream %s: %w", filename, err)
	}
	return n, nil
}

// Validate checks connectivity by ensuring the API key exists.
func (s *UMLSSource) Validate(ctx context.Context) error {
	if s.apiKey == "" {
		return fmt.Errorf("UMLS api key not set")
	}
	// Light-touch validation: fetch TGT only.
	if _, err := s.requestTGT(ctx); err != nil {
		return fmt.Errorf("failed to validate UMLS auth: %w", err)
	}
	return nil
}

func (s *UMLSSource) downloadURL(version string) string {
	filename := s.versions[version]
	return strings.TrimRight(s.baseDownloadURL, "/") + "/" + version + "/" + filename
}

func (s *UMLSSource) requestTGT(ctx context.Context) (string, error) {
	form := url.Values{}
	form.Set("apikey", s.apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseLoginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("TGT request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	if loc := resp.Header.Get("Location"); loc != "" {
		return loc, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return "", fmt.Errorf("failed to read TGT body: %w", err)
	}
	tgt := strings.TrimSpace(string(body))
	if tgt == "" {
		return "", fmt.Errorf("empty TGT response")
	}
	return tgt, nil
}

func (s *UMLSSource) requestServiceTicket(ctx context.Context, tgt string, service string) (string, error) {
	form := url.Values{}
	form.Set("service", service)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tgt, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("service ticket request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if err != nil {
		return "", fmt.Errorf("failed to read service ticket body: %w", err)
	}
	st := strings.TrimSpace(string(body))
	if st == "" {
		return "", fmt.Errorf("empty service ticket")
	}
	return st, nil
}

func pickLatestVersion(versions map[string]string) string {
	if len(versions) == 0 {
		return ""
	}
	var keys []string
	for k := range versions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys[len(keys)-1]
}
