package graphql

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"gopkg.in/yaml.v3"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/store"
)

type yamlProfileDoc struct {
	SourceProfile *yamlSourceProfile `yaml:"source_profile"`
}

type yamlSourceProfile struct {
	ID          *string          `yaml:"id,omitempty"`
	Name        *string          `yaml:"name,omitempty"`
	Version     *string          `yaml:"version,omitempty"`
	HL7v2       *yamlHL7v2       `yaml:"hl7v2,omitempty"`
	Identifiers *yamlIdentifiers `yaml:"identifiers,omitempty"`
}

type yamlHL7v2 struct {
	DefaultVersion *string       `yaml:"default_version,omitempty"`
	Timezone       *string       `yaml:"timezone,omitempty"`
	Tolerate       *yamlTolerate `yaml:"tolerate,omitempty"`
}

type yamlTolerate struct {
	MissingSegments       *[]string `yaml:"missing_segments,omitempty"`
	NTEAnywhere           *bool     `yaml:"nte_anywhere,omitempty"`
	ExtraComponents       *bool     `yaml:"extra_components,omitempty"`
	UnknownSegments       *bool     `yaml:"unknown_segments,omitempty"`
	NonStandardDelimiters *bool     `yaml:"non_standard_delimiters,omitempty"`
}

type yamlIdentifiers struct {
	Validation    *yamlValidation    `yaml:"validation,omitempty"`
	Normalization *yamlNormalization `yaml:"normalization,omitempty"`
}

type yamlValidation struct {
	NPI *yamlValidator `yaml:"npi,omitempty"`
	MBI *yamlValidator `yaml:"mbi,omitempty"`
	SSN *yamlValidator `yaml:"ssn,omitempty"`
}

type yamlValidator struct {
	Enabled   *bool   `yaml:"enabled,omitempty"`
	OnInvalid *string `yaml:"on_invalid,omitempty"`
}

type yamlNormalization struct {
	SSN   *yamlSSNNorm   `yaml:"ssn,omitempty"`
	Phone *yamlPhoneNorm `yaml:"phone,omitempty"`
}

type yamlSSNNorm struct {
	StripDashes    *bool     `yaml:"strip_dashes,omitempty"`
	RejectPatterns *[]string `yaml:"reject_patterns,omitempty"`
}

type yamlPhoneNorm struct {
	Normalize *bool   `yaml:"normalize,omitempty"`
	Format    *string `yaml:"format,omitempty"`
}

func registerProfileYAMLEndpoints(mux *http.ServeMux, resolver ResolverRoot, allowedOrigins []string) {
	type profileStoreGetter interface {
		GetProfileStore() store.ProfileStore
	}

	var profileStore store.ProfileStore
	if g, ok := resolver.(profileStoreGetter); ok {
		profileStore = g.GetProfileStore()
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if profileStore == nil {
			http.Error(w, "profile store not configured", http.StatusServiceUnavailable)
			return
		}

		id, ok := extractProfileYAMLID(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}

		switch r.Method {
		case http.MethodGet:
			handleGetProfileYAML(w, r, profileStore, id)
		case http.MethodPut:
			handlePutProfileYAML(w, r, profileStore, id)
		default:
			w.Header().Set("Allow", "GET, PUT, OPTIONS")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.Handle("/api/profiles/", corsMiddleware(handler, allowedOrigins))
}

func extractProfileYAMLID(path string) (id string, ok bool) {
	if !strings.HasPrefix(path, "/api/profiles/") {
		return "", false
	}

	rest := strings.TrimPrefix(path, "/api/profiles/")
	rest = strings.Trim(rest, "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return "", false
	}
	if parts[0] == "" || parts[1] != "yaml" {
		return "", false
	}
	return parts[0], true
}

func handleGetProfileYAML(w http.ResponseWriter, r *http.Request, profileStore store.ProfileStore, id string) {
	p, err := profileStore.GetProfile(r.Context(), id)
	if err != nil {
		http.Error(w, "failed to load profile", http.StatusInternalServerError)
		return
	}
	if p == nil {
		http.NotFound(w, r)
		return
	}

	out, err := profileToYAML(p)
	if err != nil {
		http.Error(w, "failed to render profile yaml", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}

func handlePutProfileYAML(w http.ResponseWriter, r *http.Request, profileStore store.ProfileStore, id string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	var doc yamlProfileDoc
	if err := yaml.Unmarshal(body, &doc); err != nil {
		http.Error(w, "invalid yaml", http.StatusBadRequest)
		return
	}
	if doc.SourceProfile == nil {
		http.Error(w, "missing source_profile root", http.StatusBadRequest)
		return
	}
	if doc.SourceProfile.ID != nil && *doc.SourceProfile.ID != id {
		http.Error(w, "profile id does not match request path", http.StatusBadRequest)
		return
	}

	existing, err := profileStore.GetProfile(r.Context(), id)
	if err != nil {
		http.Error(w, "failed to load profile", http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.NotFound(w, r)
		return
	}

	var config store.ProfileConfig
	if len(existing.Config) > 0 {
		_ = json.Unmarshal(existing.Config, &config) // best-effort; treat invalid as empty
	}

	applyYAMLProfileUpdate(&config, doc.SourceProfile)

	nextConfig, err := json.Marshal(config)
	if err != nil {
		http.Error(w, "failed to serialize profile config", http.StatusInternalServerError)
		return
	}

	if doc.SourceProfile.Name != nil && strings.TrimSpace(*doc.SourceProfile.Name) != "" {
		existing.Name = strings.TrimSpace(*doc.SourceProfile.Name)
	}
	existing.Config = nextConfig
	existing.Version = incrementVersion(existing.Version)

	if err := profileStore.UpdateProfile(r.Context(), existing); err != nil {
		http.Error(w, "failed to update profile", http.StatusInternalServerError)
		return
	}

	resp := map[string]any{
		"id":      existing.ID,
		"version": existing.Version,
		"name":    existing.Name,
	}
	data, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func applyYAMLProfileUpdate(config *store.ProfileConfig, y *yamlSourceProfile) {
	if y == nil {
		return
	}

	if y.HL7v2 != nil {
		if config.HL7v2 == nil {
			config.HL7v2 = &store.HL7v2Config{}
		}
		if y.HL7v2.DefaultVersion != nil {
			config.HL7v2.DefaultVersion = strings.TrimSpace(*y.HL7v2.DefaultVersion)
		}
		if y.HL7v2.Timezone != nil {
			config.HL7v2.Timezone = strings.TrimSpace(*y.HL7v2.Timezone)
		}
		if y.HL7v2.Tolerate != nil {
			if config.HL7v2.Tolerance == nil {
				config.HL7v2.Tolerance = &store.ToleranceConfig{}
			}
			if y.HL7v2.Tolerate.MissingSegments != nil {
				config.HL7v2.Tolerance.MissingSegments = *y.HL7v2.Tolerate.MissingSegments
			}
			if y.HL7v2.Tolerate.NTEAnywhere != nil {
				config.HL7v2.Tolerance.NTEAnywhere = *y.HL7v2.Tolerate.NTEAnywhere
			}
			if y.HL7v2.Tolerate.ExtraComponents != nil {
				config.HL7v2.Tolerance.ExtraComponents = *y.HL7v2.Tolerate.ExtraComponents
			}
			if y.HL7v2.Tolerate.UnknownSegments != nil {
				config.HL7v2.Tolerance.UnknownSegments = *y.HL7v2.Tolerate.UnknownSegments
			}
			if y.HL7v2.Tolerate.NonStandardDelimiters != nil {
				config.HL7v2.Tolerance.NonStandardDelimiters = *y.HL7v2.Tolerate.NonStandardDelimiters
			}
		}
	}

	if y.Identifiers != nil {
		if config.Identifiers == nil {
			config.Identifiers = &store.IdentifierConfig{}
		}

		if y.Identifiers.Validation != nil {
			if config.Identifiers.Validation == nil {
				config.Identifiers.Validation = &store.ValidationConfig{}
			}
			applyYAMLValidator(&config.Identifiers.Validation.NPI, y.Identifiers.Validation.NPI)
			applyYAMLValidator(&config.Identifiers.Validation.MBI, y.Identifiers.Validation.MBI)
			applyYAMLValidator(&config.Identifiers.Validation.SSN, y.Identifiers.Validation.SSN)
		}

		if y.Identifiers.Normalization != nil {
			if config.Identifiers.Normalization == nil {
				config.Identifiers.Normalization = &store.NormalizationConfig{}
			}
			if y.Identifiers.Normalization.SSN != nil {
				if y.Identifiers.Normalization.SSN.StripDashes != nil {
					config.Identifiers.Normalization.SSNStripDashes = *y.Identifiers.Normalization.SSN.StripDashes
				}
				if y.Identifiers.Normalization.SSN.RejectPatterns != nil {
					config.Identifiers.Normalization.SSNRejectPatterns = *y.Identifiers.Normalization.SSN.RejectPatterns
				}
			}
			if y.Identifiers.Normalization.Phone != nil {
				if y.Identifiers.Normalization.Phone.Normalize != nil {
					config.Identifiers.Normalization.PhoneNormalize = *y.Identifiers.Normalization.Phone.Normalize
				}
				if y.Identifiers.Normalization.Phone.Format != nil {
					config.Identifiers.Normalization.PhoneFormat = *y.Identifiers.Normalization.Phone.Format
				}
			}
		}
	}
}

func applyYAMLValidator(dest **store.ValidatorSetting, in *yamlValidator) {
	if in == nil {
		return
	}
	if *dest == nil {
		*dest = &store.ValidatorSetting{
			Enabled:   false,
			OnInvalid: "pass",
		}
	}
	if in.Enabled != nil {
		(*dest).Enabled = *in.Enabled
	}
	if in.OnInvalid != nil {
		(*dest).OnInvalid = strings.TrimSpace(*in.OnInvalid)
	}
}

func profileToYAML(p *store.Profile) ([]byte, error) {
	var config store.ProfileConfig
	if len(p.Config) > 0 {
		if err := json.Unmarshal(p.Config, &config); err != nil {
			return nil, fmt.Errorf("unmarshal config: %w", err)
		}
	}

	doc := yamlProfileDoc{
		SourceProfile: &yamlSourceProfile{},
	}

	id := p.ID
	name := p.Name
	version := p.Version
	doc.SourceProfile.ID = &id
	doc.SourceProfile.Name = &name
	doc.SourceProfile.Version = &version

	// HL7v2 (defaulted, to match UI export)
	hl7v2 := &yamlHL7v2{}
	defaultVersion := "2.5.1"
	timezone := "UTC"
	if config.HL7v2 != nil {
		if strings.TrimSpace(config.HL7v2.DefaultVersion) != "" {
			defaultVersion = config.HL7v2.DefaultVersion
		}
		if strings.TrimSpace(config.HL7v2.Timezone) != "" {
			timezone = config.HL7v2.Timezone
		}
	}
	hl7v2.DefaultVersion = &defaultVersion
	hl7v2.Timezone = &timezone

	tol := &yamlTolerate{}
	missing := []string{}
	nteAnywhere := false
	extraComponents := false
	unknownSegments := false
	nonStandardDelimiters := false
	if config.HL7v2 != nil && config.HL7v2.Tolerance != nil {
		missing = config.HL7v2.Tolerance.MissingSegments
		nteAnywhere = config.HL7v2.Tolerance.NTEAnywhere
		extraComponents = config.HL7v2.Tolerance.ExtraComponents
		unknownSegments = config.HL7v2.Tolerance.UnknownSegments
		nonStandardDelimiters = config.HL7v2.Tolerance.NonStandardDelimiters
	}
	tol.MissingSegments = &missing
	tol.NTEAnywhere = &nteAnywhere
	tol.ExtraComponents = &extraComponents
	tol.UnknownSegments = &unknownSegments
	tol.NonStandardDelimiters = &nonStandardDelimiters
	hl7v2.Tolerate = tol
	doc.SourceProfile.HL7v2 = hl7v2

	// Identifiers (defaulted, to match UI export)
	ids := &yamlIdentifiers{
		Validation: &yamlValidation{},
		Normalization: &yamlNormalization{
			SSN:   &yamlSSNNorm{},
			Phone: &yamlPhoneNorm{},
		},
	}

	npiEnabled, mbiEnabled, ssnEnabled := false, false, false
	npiOnInvalid, mbiOnInvalid, ssnOnInvalid := "pass", "pass", "pass"
	ssnStripDashes := false
	ssnReject := []string{}
	phoneNormalize := false
	var phoneFormat *string

	if config.Identifiers != nil {
		if config.Identifiers.Validation != nil {
			if config.Identifiers.Validation.NPI != nil {
				npiEnabled = config.Identifiers.Validation.NPI.Enabled
				if strings.TrimSpace(config.Identifiers.Validation.NPI.OnInvalid) != "" {
					npiOnInvalid = config.Identifiers.Validation.NPI.OnInvalid
				}
			}
			if config.Identifiers.Validation.MBI != nil {
				mbiEnabled = config.Identifiers.Validation.MBI.Enabled
				if strings.TrimSpace(config.Identifiers.Validation.MBI.OnInvalid) != "" {
					mbiOnInvalid = config.Identifiers.Validation.MBI.OnInvalid
				}
			}
			if config.Identifiers.Validation.SSN != nil {
				ssnEnabled = config.Identifiers.Validation.SSN.Enabled
				if strings.TrimSpace(config.Identifiers.Validation.SSN.OnInvalid) != "" {
					ssnOnInvalid = config.Identifiers.Validation.SSN.OnInvalid
				}
			}
		}
		if config.Identifiers.Normalization != nil {
			ssnStripDashes = config.Identifiers.Normalization.SSNStripDashes
			ssnReject = config.Identifiers.Normalization.SSNRejectPatterns
			phoneNormalize = config.Identifiers.Normalization.PhoneNormalize
			if strings.TrimSpace(config.Identifiers.Normalization.PhoneFormat) != "" {
				f := config.Identifiers.Normalization.PhoneFormat
				phoneFormat = &f
			}
		}
	}

	ids.Validation.NPI = &yamlValidator{Enabled: &npiEnabled, OnInvalid: &npiOnInvalid}
	ids.Validation.MBI = &yamlValidator{Enabled: &mbiEnabled, OnInvalid: &mbiOnInvalid}
	ids.Validation.SSN = &yamlValidator{Enabled: &ssnEnabled, OnInvalid: &ssnOnInvalid}

	ids.Normalization.SSN.StripDashes = &ssnStripDashes
	ids.Normalization.SSN.RejectPatterns = &ssnReject
	ids.Normalization.Phone.Normalize = &phoneNormalize
	ids.Normalization.Phone.Format = phoneFormat
	doc.SourceProfile.Identifiers = ids

	out, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal yaml: %w", err)
	}
	return out, nil
}

func incrementVersion(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return "1.0.1"
	}
	patch := parts[2]
	n := 0
	for _, r := range patch {
		if r < '0' || r > '9' {
			return "1.0.1"
		}
		n = n*10 + int(r-'0')
	}
	return fmt.Sprintf("%s.%s.%d", parts[0], parts[1], n+1)
}
