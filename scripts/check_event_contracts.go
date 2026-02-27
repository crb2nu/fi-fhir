package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	eventTypeConstRE = regexp.MustCompile(`(?m)^\s*[A-Za-z0-9_]+\s+EventType\s*=\s*"([a-z0-9_]+)"`)
)

type set map[string]struct{}

func main() {
	var (
		root   string
		report string
		strict bool
	)

	flag.StringVar(&root, "root", ".", "repository root")
	flag.StringVar(&report, "report", "", "optional markdown report output path")
	flag.BoolVar(&strict, "strict", false, "exit non-zero on contract drift")
	flag.Parse()

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		exitErr(fmt.Errorf("resolve root: %w", err))
	}

	canonicalPath := filepath.Join(rootAbs, "pkg/events/events.go")
	graphqlPath := filepath.Join(rootAbs, "internal/api/graphql/schema.graphql")
	openapiPath := filepath.Join(rootAbs, "api/openapi.yaml")

	canonical, err := parseCanonicalEventTypes(canonicalPath)
	if err != nil {
		exitErr(err)
	}
	graphql, err := parseGraphQLEventTypes(graphqlPath)
	if err != nil {
		exitErr(err)
	}
	openapi, err := parseOpenAPIEventTypes(openapiPath)
	if err != nil {
		exitErr(err)
	}

	gMissing := diff(canonical, graphql)
	gExtra := diff(graphql, canonical)
	oMissing := diff(canonical, openapi)
	oExtra := diff(openapi, canonical)

	matrix := renderMatrix(rootAbs, canonicalPath, graphqlPath, openapiPath, canonical, graphql, openapi, gMissing, gExtra, oMissing, oExtra)
	fmt.Print(matrix)

	if report != "" {
		reportPath := report
		if !filepath.IsAbs(reportPath) {
			reportPath = filepath.Join(rootAbs, report)
		}
		if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
			exitErr(fmt.Errorf("create report directory: %w", err))
		}
		if err := os.WriteFile(reportPath, []byte(matrix), 0o644); err != nil {
			exitErr(fmt.Errorf("write report %s: %w", reportPath, err))
		}
		fmt.Fprintf(os.Stderr, "wrote report: %s\n", reportPath)
	}

	if strict && (len(gMissing) > 0 || len(gExtra) > 0 || len(oMissing) > 0 || len(oExtra) > 0) {
		exitErr(errors.New("contract drift detected in strict mode"))
	}
}

func parseCanonicalEventTypes(path string) (set, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read canonical events file: %w", err)
	}

	matches := eventTypeConstRE.FindAllStringSubmatch(string(content), -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no EventType constants found in %s", path)
	}

	out := make(set, len(matches))
	for _, m := range matches {
		out[m[1]] = struct{}{}
	}
	return out, nil
}

func parseGraphQLEventTypes(path string) (set, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read graphql schema: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	inEnum := false
	out := make(set)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inEnum {
			if strings.HasPrefix(trimmed, "enum EventType") {
				inEnum = true
			}
			continue
		}

		if trimmed == "}" {
			break
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		token := strings.Fields(trimmed)[0]
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		out[strings.ToLower(token)] = struct{}{}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no EventType enum entries found in %s", path)
	}
	return out, nil
}

func parseOpenAPIEventTypes(path string) (set, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read openapi spec: %w", err)
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("parse openapi yaml: %w", err)
	}

	components := mapAt(doc, "components")
	schemas := mapAt(components, "schemas")
	eventSchema := mapAt(schemas, "Event")
	properties := mapAt(eventSchema, "properties")
	typeProp := mapAt(properties, "type")
	enumRaw, ok := typeProp["enum"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("openapi Event.type.enum not found in %s", path)
	}

	out := make(set, len(enumRaw))
	for _, e := range enumRaw {
		s, ok := e.(string)
		if !ok || strings.TrimSpace(s) == "" {
			continue
		}
		out[s] = struct{}{}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("openapi Event.type.enum is empty in %s", path)
	}
	return out, nil
}

func mapAt(m map[string]interface{}, key string) map[string]interface{} {
	v, ok := m[key]
	if !ok {
		return map[string]interface{}{}
	}
	asMap, ok := v.(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return asMap
}

func diff(a, b set) []string {
	var out []string
	for k := range a {
		if _, ok := b[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func keys(s set) []string {
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func allEvents(canonical, graphql, openapi set) []string {
	union := make(set, len(canonical)+len(graphql)+len(openapi))
	for k := range canonical {
		union[k] = struct{}{}
	}
	for k := range graphql {
		union[k] = struct{}{}
	}
	for k := range openapi {
		union[k] = struct{}{}
	}
	return keys(union)
}

func yesNo(s set, key string) string {
	if _, ok := s[key]; ok {
		return "yes"
	}
	return "no"
}

func renderMatrix(rootAbs, canonicalPath, graphqlPath, openapiPath string, canonical, graphql, openapi set, gMissing, gExtra, oMissing, oExtra []string) string {
	var b strings.Builder

	rel := func(p string) string {
		r, err := filepath.Rel(rootAbs, p)
		if err != nil {
			return p
		}
		return filepath.ToSlash(r)
	}

	fmt.Fprintf(&b, "# Event Contract Matrix\n\n")
	fmt.Fprintf(&b, "Generated: %s\n\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "## Inputs\n\n")
	fmt.Fprintf(&b, "- Canonical: `%s`\n", rel(canonicalPath))
	fmt.Fprintf(&b, "- GraphQL: `%s`\n", rel(graphqlPath))
	fmt.Fprintf(&b, "- OpenAPI: `%s`\n\n", rel(openapiPath))

	fmt.Fprintf(&b, "## Summary\n\n")
	fmt.Fprintf(&b, "- Canonical count: %d\n", len(canonical))
	fmt.Fprintf(&b, "- GraphQL count: %d\n", len(graphql))
	fmt.Fprintf(&b, "- OpenAPI count: %d\n", len(openapi))
	fmt.Fprintf(&b, "- Missing in GraphQL (vs canonical): %d\n", len(gMissing))
	fmt.Fprintf(&b, "- Extra in GraphQL (not in canonical): %d\n", len(gExtra))
	fmt.Fprintf(&b, "- Missing in OpenAPI (vs canonical): %d\n", len(oMissing))
	fmt.Fprintf(&b, "- Extra in OpenAPI (not in canonical): %d\n\n", len(oExtra))

	fmt.Fprintf(&b, "## Matrix\n\n")
	fmt.Fprintf(&b, "| Event Type | Canonical | GraphQL | OpenAPI |\n")
	fmt.Fprintf(&b, "|---|---|---|---|\n")
	for _, eventType := range allEvents(canonical, graphql, openapi) {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", eventType, yesNo(canonical, eventType), yesNo(graphql, eventType), yesNo(openapi, eventType))
	}
	fmt.Fprintf(&b, "\n")

	writeList := func(title string, values []string) {
		fmt.Fprintf(&b, "### %s\n\n", title)
		if len(values) == 0 {
			fmt.Fprintf(&b, "- none\n\n")
			return
		}
		for _, v := range values {
			fmt.Fprintf(&b, "- `%s`\n", v)
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "## Drift Details\n\n")
	writeList("Missing In GraphQL", gMissing)
	writeList("Extra In GraphQL", gExtra)
	writeList("Missing In OpenAPI", oMissing)
	writeList("Extra In OpenAPI", oExtra)

	fmt.Fprintf(&b, "## Notes\n\n")
	fmt.Fprintf(&b, "- GraphQL enums are normalized from uppercase to lowercase snake case for comparison.\n")
	fmt.Fprintf(&b, "- OpenAPI values are compared as-is from `components.schemas.Event.properties.type.enum`.\n")
	fmt.Fprintf(&b, "- Use `--strict` to fail fast when drift exists.\n")

	return b.String()
}

func exitErr(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
