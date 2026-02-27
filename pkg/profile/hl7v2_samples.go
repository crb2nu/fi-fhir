package profile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type HL7v2SampleStats struct {
	MessageCount int

	// Counts (value -> occurrences)
	FieldSeparators map[rune]int
	EncodingChars   map[string]int
	Versions        map[string]int
	CharSets        map[string]int
	MessageTypes    map[string]int

	SegmentPresence map[string]int // segment -> number of messages it appears in
	ZSegments       map[string]int // Z-segment ID -> number of messages it appears in

	HasLF bool
	HasCR bool
}

func AnalyzeHL7v2Samples(samples []string) (*HL7v2SampleStats, error) {
	stats := &HL7v2SampleStats{
		FieldSeparators: make(map[rune]int),
		EncodingChars:   make(map[string]int),
		Versions:        make(map[string]int),
		CharSets:        make(map[string]int),
		MessageTypes:    make(map[string]int),
		SegmentPresence: make(map[string]int),
		ZSegments:       make(map[string]int),
	}

	for _, raw := range samples {
		if raw == "" {
			continue
		}

		if strings.Contains(raw, "\n") {
			stats.HasLF = true
		}
		if strings.Contains(raw, "\r") {
			stats.HasCR = true
		}

		mshLine, ok := firstHL7v2SegmentLine(raw)
		if !ok {
			return nil, fmt.Errorf("missing MSH segment")
		}
		fieldSep, encodingChars, mshFields := parseMSHLine(mshLine)

		stats.MessageCount++
		if fieldSep != 0 {
			stats.FieldSeparators[fieldSep]++
		}
		if encodingChars != "" {
			stats.EncodingChars[encodingChars]++
		}

		// MSH-9 (Message Type) is index 8; MSH-12 (Version) is index 11.
		if mt := getMSHField(mshFields, 9); mt != "" {
			stats.MessageTypes[mt]++
		}
		if v := getMSHField(mshFields, 12); v != "" {
			stats.Versions[v]++
		}
		// MSH-18 (Character Set) is index 17.
		if cs := getMSHField(mshFields, 18); cs != "" {
			stats.CharSets[cs]++
		}

		seenSegments := make(map[string]bool)
		for _, line := range splitHL7Lines(raw) {
			id := segmentID(line)
			if id == "" {
				continue
			}
			if seenSegments[id] {
				continue
			}
			seenSegments[id] = true
			stats.SegmentPresence[id]++
			if strings.HasPrefix(id, "Z") {
				stats.ZSegments[id]++
			}
		}
	}

	if stats.MessageCount == 0 {
		return nil, fmt.Errorf("no samples provided")
	}
	return stats, nil
}

type ReadHL7v2SamplesOptions struct {
	MaxFiles int
}

func ReadHL7v2Samples(paths []string, opts ReadHL7v2SamplesOptions) ([]string, []string, error) {
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = 200
	}

	var files []string
	for _, p := range paths {
		if p == "" {
			continue
		}
		info, err := os.Stat(p)
		if err != nil {
			return nil, nil, fmt.Errorf("stat %s: %w", p, err)
		}
		if info.IsDir() {
			err := filepath.WalkDir(p, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					return nil
				}
				if shouldConsiderHL7v2Sample(path) {
					files = append(files, path)
				}
				return nil
			})
			if err != nil {
				return nil, nil, fmt.Errorf("walk %s: %w", p, err)
			}
			continue
		}
		files = append(files, p)
	}

	sort.Strings(files)
	if len(files) > opts.MaxFiles {
		files = files[:opts.MaxFiles]
	}

	var samples []string
	var used []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", f, err)
		}
		s := strings.TrimSpace(string(data))
		if s == "" {
			continue
		}
		samples = append(samples, s)
		used = append(used, f)
	}

	return samples, used, nil
}

func shouldConsiderHL7v2Sample(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".hl7", ".hl7v2", ".txt":
		return true
	default:
		return false
	}
}

func firstHL7v2SegmentLine(raw string) (string, bool) {
	for _, line := range splitHL7Lines(raw) {
		if strings.HasPrefix(line, "MSH") {
			return line, true
		}
	}
	return "", false
}

func splitHL7Lines(raw string) []string {
	// HL7 is typically CR-delimited, but real-world feeds can be LF or CRLF.
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")

	out := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		out = append(out, l)
	}
	return out
}

func segmentID(line string) string {
	if len(line) < 3 {
		return ""
	}
	id := strings.ToUpper(line[:3])
	for _, c := range id {
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return ""
		}
	}
	return id
}

func parseMSHLine(line string) (fieldSep rune, encodingChars string, fields []string) {
	if len(line) < 4 || !strings.HasPrefix(line, "MSH") {
		return 0, "", nil
	}
	fieldSep = rune(line[3])
	parts := strings.Split(line, string(line[3]))
	if len(parts) > 1 {
		encodingChars = parts[1]
	}
	return fieldSep, encodingChars, parts
}

func getMSHField(parts []string, fieldNumber int) string {
	// parts[0] is "MSH", parts[1] is MSH-2. So MSH-n is parts[n-1].
	idx := fieldNumber - 1
	if idx <= 0 || idx >= len(parts) {
		return ""
	}
	return strings.TrimSpace(parts[idx])
}

func mostCommonString(m map[string]int) (value string, count int) {
	for v, c := range m {
		if c > count {
			value = v
			count = c
		}
	}
	return value, count
}
