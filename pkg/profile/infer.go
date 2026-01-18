package profile

import (
	"fmt"
	"sort"
	"strings"
)

type InferHL7v2Options struct {
	ID      string
	Name    string
	Version string

	Timezone string
}

type InferHL7v2Report struct {
	Stats     *HL7v2SampleStats
	InputUsed []string
}

func InferHL7v2ProfileFromPaths(paths []string, opts InferHL7v2Options) (*SourceProfile, *InferHL7v2Report, error) {
	samples, used, err := ReadHL7v2Samples(paths, ReadHL7v2SamplesOptions{})
	if err != nil {
		return nil, nil, err
	}
	return InferHL7v2ProfileFromSamples(samples, used, opts)
}

func InferHL7v2ProfileFromSamples(samples []string, inputUsed []string, opts InferHL7v2Options) (*SourceProfile, *InferHL7v2Report, error) {
	stats, err := AnalyzeHL7v2Samples(samples)
	if err != nil {
		return nil, nil, err
	}

	id := strings.TrimSpace(opts.ID)
	if id == "" {
		id = "inferred_profile"
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "Inferred Profile"
	}
	version := strings.TrimSpace(opts.Version)
	if version == "" {
		version = "0.1.0"
	}

	defaultVersion, _ := mostCommonString(stats.Versions)
	if defaultVersion == "" {
		defaultVersion = "2.5.1"
	}
	charsetDefault, _ := mostCommonString(stats.CharSets)
	if charsetDefault == "" {
		charsetDefault = "UTF-8"
	}

	timezone := strings.TrimSpace(opts.Timezone)
	if timezone == "" {
		timezone = "UTC"
	}

	nonStandardDelimiters := len(stats.FieldSeparators) > 1 || len(stats.EncodingChars) > 1

	zIDs := make([]string, 0, len(stats.ZSegments))
	for id := range stats.ZSegments {
		zIDs = append(zIDs, id)
	}
	sort.Strings(zIDs)

	zMappings := make(map[string][]ZFieldMapping, len(zIDs))
	for _, zid := range zIDs {
		zMappings[zid] = []ZFieldMapping{}
	}

	// Heuristic: tolerate missing segments only for a small, curated set, and only if
	// they appear in some (but not all) samples (or are entirely absent across samples).
	//
	// This avoids inferring an enormous missing_segments list from small sample sets.
	missingSegments := inferMissingSegments(stats)

	p := &SourceProfile{
		ID:      id,
		Name:    name,
		Version: version,
		HL7v2: &HL7v2Config{
			DefaultVersion: defaultVersion,
			Timezone:       timezone,
			Encoding: &EncodingConfig{
				CharsetDefault:   charsetDefault,
				CharsetDetection: true,
				LineEndingMode:   "tolerant",
			},
			Tolerate: &ToleranceConfig{
				MissingSegments:       missingSegments,
				NTEAnywhere:           true,
				ExtraComponents:       true,
				UnknownSegments:       true,
				NonStandardDelimiters: nonStandardDelimiters,
			},
			Datatypes: &DatatypeConfig{
				XCNComponentCount: "flexible",
				CXComponentCount:  "flexible",
				XPNComponentCount: "flexible",
			},
		},
		ZSegments: &ZSegmentConfig{
			PreserveRaw: true,
			Mappings:    zMappings,
		},
	}

	// Sanity check against the existing registry validator.
	b, err := MarshalYAML(p)
	if err != nil {
		return nil, nil, err
	}
	r := NewRegistry()
	if _, err := r.LoadFromBytes(b); err != nil {
		return nil, nil, fmt.Errorf("generated profile did not validate: %w", err)
	}

	return p, &InferHL7v2Report{Stats: stats, InputUsed: inputUsed}, nil
}

func inferMissingSegments(stats *HL7v2SampleStats) []string {
	if stats == nil || stats.MessageCount <= 0 {
		return nil
	}

	candidates := []string{"PV1", "PD1", "OBR", "NK1", "GT1"}

	var out []string
	for _, seg := range candidates {
		if stats.SegmentPresence[seg] < stats.MessageCount {
			out = append(out, seg)
		}
	}
	sort.Strings(out)
	return out
}
