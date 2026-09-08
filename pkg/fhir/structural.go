package fhir

// Slice 5.1b, Option C: a structural validator over pinned offline FHIR IG
// packages.
//
// What this file adds over `validate.go`
//
// `validate.go` is a hand-written checker. Its required-element list for six
// resource types and its map of expected profile canonicals are literals in Go
// source; nothing in the repository resolves either against the implementation
// guide they claim to follow. That is exactly what Slice 5.1a recorded as the
// limit of its own work, and why `docs/operations/SUPPORTED-1.0.md` says
// "version *tolerance* is not version *resolution*".
//
// This file resolves. Every rule it enforces is read at run time out of
// `testdata/fhir/packages/*.tgz` — the byte-pinned `hl7.fhir.r4.core#4.0.1` and
// `hl7.fhir.us.core#9.0.0` archives — so a rule cannot drift from the IG
// without the IG file changing, and the archives cannot change without
// `SHA256SUMS` failing.
//
// What it does NOT do, stated up front so no reader infers more than is true:
//
//   - It is NOT the official FHIR validator and produces no conformance
//     certificate. `validator_cli.jar` as a CI-only job (Option A) is Sprint 7.
//   - No terminology. A `ValueSet` binding of any strength is not evaluated, so
//     a code outside a required binding passes here.
//   - No invariants. FHIRPath `constraint` expressions are not evaluated.
//   - No slicing. Elements whose snapshot `id` carries a `:` discriminator are
//     skipped, because deciding which slice a JSON element belongs to needs the
//     terminology this validator does not have. Slice cardinality is therefore
//     unchecked; the unsliced element's cardinality is checked.
//   - No primitive-type or regex checking, no reference-target checking, no
//     extension validation, no `contentReference` expansion.
//
// Confinement (ratified 2026-08-08, `.loom/decisions/`): nothing here reaches
// the shipped image. This file is stdlib-only — `archive/tar`, `compress/gzip`,
// `crypto/sha256`, `encoding/json` — so it adds no `go.mod` dependency, and
// `.dockerignore:20` excludes `testdata/`, so the archives it reads are absent
// from the build context that produces the distroless runtime image.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// PinnedPackage names one offline IG archive under testdata/fhir/packages and
// the digest that archive must have.
//
// The digest lives in Go source as well as in SHA256SUMS on purpose: SHA256SUMS
// is what a human regenerates when a pin deliberately moves, and these
// constants are what fails a build if only one of the two was updated.
type PinnedPackage struct {
	Name    string
	Version string
	SHA256  string
}

// The two packages Slice 5.1b pins. See testdata/fhir/packages/README.md for
// provenance, including the registry-published SHA-1 that independently
// attests these are the upstream bytes.
var (
	// PinnedR4Core is FHIR R4 4.0.1, the base specification every US Core
	// profile derives from.
	PinnedR4Core = PinnedPackage{
		Name:    "hl7.fhir.r4.core",
		Version: "4.0.1",
		SHA256:  "ebd7731df7d36b5b7d39d5fb6c9d77b44bb7fe5742f1a2e87f164738c3289d44",
	}
	// PinnedUSCore is US Core 9.0.0, the profile set this product asserts.
	PinnedUSCore = PinnedPackage{
		Name:    "hl7.fhir.us.core",
		Version: "9.0.0",
		SHA256:  "d7b54d2ec2a48cea94ffea5d939ad67a681f80b94d69594a08cebac36da9e059",
	}
)

// PinnedPackages is the load order: R4 core first, US Core second, so a US Core
// profile's baseDefinition chain always has somewhere to terminate.
func PinnedPackages() []PinnedPackage {
	return []PinnedPackage{PinnedR4Core, PinnedUSCore}
}

// Filename is the archive's name under the packages directory.
func (p PinnedPackage) Filename() string {
	return p.Name + "-" + p.Version + ".tgz"
}

// String renders the FHIR package identifier form, `name#version`.
func (p PinnedPackage) String() string {
	return p.Name + "#" + p.Version
}

// ErrProfileNotResolved is returned when a canonical reference names no
// StructureDefinition in the pinned packages. It is deliberately distinct from
// "the profile resolved but the resource violates it": an unresolvable profile
// means the packages and the mapper disagree about what exists, which is a
// pinning failure rather than a conformance failure.
var ErrProfileNotResolved = errors.New("profile does not resolve in the pinned packages")

// ErrProfileVersionMismatch is returned when a canonical asserts `|version` and
// the resolved StructureDefinition carries a different one. Slice 5.1a's policy
// is that the mapper asserts bare canonicals and the checker accepts either
// form; accepting a *wrong* pinned version would make the tolerance a hole.
var ErrProfileVersionMismatch = errors.New("asserted profile version does not match the pinned package")

// ElementDefinition is the subset of a FHIR R4 ElementDefinition this validator
// reads. Everything omitted — bindings, constraints, type refinements, slicing
// discriminators — is omitted because this validator does not evaluate it, not
// because the IG lacks it.
type ElementDefinition struct {
	ID               string `json:"id"`
	Path             string `json:"path"`
	Min              *int   `json:"min"`
	Max              string `json:"max"`
	MustSupport      bool   `json:"mustSupport"`
	ContentReference string `json:"contentReference"`
}

// IsSlice reports whether this element constrains one slice rather than the
// element itself. Snapshot ids use `path:sliceName` for slices while `path`
// stays unsliced, so the id is the only place the distinction survives.
func (e ElementDefinition) IsSlice() bool { return strings.Contains(e.ID, ":") }

// MinOrZero treats an absent `min` as 0, which is what FHIR R4 §5.1.0.7 means
// by an omitted cardinality lower bound in a differential; snapshots normally
// carry it explicitly.
func (e ElementDefinition) MinOrZero() int {
	if e.Min == nil {
		return 0
	}
	return *e.Min
}

// MaxIsOne reports whether the element is single-valued, i.e. must not be a
// JSON array.
func (e ElementDefinition) MaxIsOne() bool { return e.Max == "1" }

// MaxIsZero reports whether the element is prohibited by this profile.
func (e ElementDefinition) MaxIsZero() bool { return e.Max == "0" }

// StructureDefinition is the subset of a FHIR R4 StructureDefinition this
// validator reads. Only `snapshot` is used: a differential expresses a delta
// against a base this validator would then have to merge itself, and a merge
// bug would silently weaken every rule. Both pinned packages ship snapshots.
type StructureDefinition struct {
	ResourceType   string `json:"resourceType"`
	URL            string `json:"url"`
	Version        string `json:"version"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	Kind           string `json:"kind"`
	Abstract       bool   `json:"abstract"`
	Derivation     string `json:"derivation"`
	BaseDefinition string `json:"baseDefinition"`
	Snapshot       struct {
		Element []ElementDefinition `json:"element"`
	} `json:"snapshot"`
}

// IGPackage is one loaded archive.
type IGPackage struct {
	Name    string
	Version string
	// Dependencies is `package.json`'s dependency map, package name to version.
	// It is read so a test can assert the two pins are compatible by the IG's
	// own declaration rather than by our assumption: US Core 9.0.0 declares
	// `hl7.fhir.r4.core: 4.0.1`, which is the R4 archive pinned beside it.
	Dependencies map[string]string
	Definition   map[string]*StructureDefinition // keyed by canonical url
}

// PackageSet is the resolution scope: the pinned archives, indexed together.
//
// A single flat index across both packages is correct here because canonical
// URLs are globally unique by construction (FHIR R4 §2.24.1.3) and the two
// packages share no URL — asserted by
// TestFHIRStructural_PinnedPackagesDoNotOverlap.
type PackageSet struct {
	Packages   []*IGPackage
	definition map[string]*StructureDefinition
}

// LoadPinnedPackages loads every package in PinnedPackages() from dir, verifying
// each archive's SHA-256 against its PinnedPackage constant before parsing a
// byte of it, and each archive's package.json against the declared name and
// version after.
//
// dir is the path to testdata/fhir/packages. Tests pass
// filepath.Join("..", "..", "testdata", "fhir", "packages").
func LoadPinnedPackages(dir string) (*PackageSet, error) {
	set := &PackageSet{definition: make(map[string]*StructureDefinition)}
	for _, pinned := range PinnedPackages() {
		pkg, err := LoadPackageArchive(filepath.Join(dir, pinned.Filename()), pinned)
		if err != nil {
			return nil, err
		}
		set.Packages = append(set.Packages, pkg)
		for url, sd := range pkg.Definition {
			if _, clash := set.definition[url]; clash {
				return nil, fmt.Errorf("canonical %q is defined by more than one pinned package", url)
			}
			set.definition[url] = sd
		}
	}
	return set, nil
}

// LoadPackageArchive reads one FHIR package `.tgz`.
//
// The digest is computed over the whole file first and compared against
// pinned.SHA256; a mismatch aborts before any parsing, so a swapped archive can
// never contribute a single rule. Only `package/package.json` and
// `package/StructureDefinition-*.json` are parsed — the R4 core archive holds
// 5,046 entries of which 658 are StructureDefinitions, and parsing the other
// 4,388 (SearchParameters, ValueSets, CodeSystems, OpenAPI schemas) would cost
// seconds per test run to build an index nothing reads.
func LoadPackageArchive(archivePath string, pinned PinnedPackage) (*IGPackage, error) {
	raw, err := os.ReadFile(archivePath) // #nosec G304 -- path is built from a compile-time package list plus a caller-supplied test fixture directory.
	if err != nil {
		return nil, fmt.Errorf("read pinned package %s: %w", pinned, err)
	}

	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != pinned.SHA256 {
		return nil, fmt.Errorf(
			"pinned package %s digest mismatch: archive %s has sha256 %s, want %s "+
				"(if the pin moved deliberately, update both PinnedPackage and testdata/fhir/packages/SHA256SUMS)",
			pinned, archivePath, got, pinned.SHA256)
	}

	pkg := &IGPackage{Definition: make(map[string]*StructureDefinition)}

	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("gunzip pinned package %s: %w", pinned, err)
	}
	defer func() { _ = gz.Close() }()

	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read pinned package %s: %w", pinned, err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}

		name := path.Clean(header.Name)
		dir, base := path.Split(name)
		// Only the archive's own `package/` root. `package/example/`,
		// `package/openapi/`, `package/other/` and `package/xml/` are examples
		// and alternate representations, not definitions.
		if dir != "package/" {
			continue
		}

		switch {
		case base == "package.json":
			var manifest struct {
				Name         string            `json:"name"`
				Version      string            `json:"version"`
				Dependencies map[string]string `json:"dependencies"`
			}
			if err := decodeTarEntry(reader, header, &manifest); err != nil {
				return nil, fmt.Errorf("pinned package %s: package.json: %w", pinned, err)
			}
			pkg.Name, pkg.Version, pkg.Dependencies = manifest.Name, manifest.Version, manifest.Dependencies

		case strings.HasPrefix(base, "StructureDefinition-") && strings.HasSuffix(base, ".json"):
			sd := &StructureDefinition{}
			if err := decodeTarEntry(reader, header, sd); err != nil {
				return nil, fmt.Errorf("pinned package %s: %s: %w", pinned, base, err)
			}
			// Defence against a filename that lies about its contents.
			if sd.ResourceType != "StructureDefinition" || sd.URL == "" {
				continue
			}
			pkg.Definition[sd.URL] = sd
		}
	}

	if pkg.Name != pinned.Name || pkg.Version != pinned.Version {
		return nil, fmt.Errorf(
			"pinned package %s declares %s#%s in its package.json",
			pinned, pkg.Name, pkg.Version)
	}
	if len(pkg.Definition) == 0 {
		return nil, fmt.Errorf("pinned package %s contains no StructureDefinition", pinned)
	}
	return pkg, nil
}

// maxPackageEntryBytes caps a single decoded archive entry. The largest
// StructureDefinition in either pinned package is well under a megabyte; the
// cap exists so a future re-pin of a hostile archive cannot decompress without
// bound (gosec G110).
const maxPackageEntryBytes = 32 << 20

func decodeTarEntry(reader io.Reader, header *tar.Header, into any) error {
	if header.Size > maxPackageEntryBytes {
		return fmt.Errorf("entry is %d bytes, over the %d-byte cap", header.Size, maxPackageEntryBytes)
	}
	body, err := io.ReadAll(io.LimitReader(reader, maxPackageEntryBytes))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, into)
}

// Resolve looks up one canonical reference, applying Slice 5.1a's
// profile-version assertion policy.
//
// A bare canonical resolves by URL. A `url|version` canonical resolves by URL
// and then requires the pinned StructureDefinition to carry that exact version.
// So `…/us-core-patient` and `…/us-core-patient|9.0.0` both resolve against the
// pinned US Core 9.0.0, and `…/us-core-patient|8.0.0` is
// ErrProfileVersionMismatch rather than a silent pass. This is the step Slice
// 5.1a could not take and named as 5.1b's job: version *resolution*, not merely
// version *tolerance*.
func (s *PackageSet) Resolve(canonical string) (*StructureDefinition, error) {
	bare := ProfileCanonical(canonical)
	sd, ok := s.definition[bare]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrProfileNotResolved, canonical)
	}

	trimmed := strings.TrimSpace(canonical)
	if index := strings.Index(trimmed, "|"); index >= 0 {
		asserted := strings.TrimSpace(trimmed[index+1:])
		if asserted != "" && asserted != sd.Version {
			return nil, fmt.Errorf("%w: %q asserts %s, pinned package has %s",
				ErrProfileVersionMismatch, canonical, asserted, sd.Version)
		}
	}
	return sd, nil
}

// R4BaseCanonical is the canonical URL of the R4 core StructureDefinition for a
// resource type.
func R4BaseCanonical(resourceType string) string {
	return "http://hl7.org/fhir/StructureDefinition/" + resourceType
}

// ResolveResourceBase returns the R4 core StructureDefinition for a resource
// type. A resource type with no base StructureDefinition in the pinned R4 core
// package is not a FHIR R4 resource type.
func (s *PackageSet) ResolveResourceBase(resourceType string) (*StructureDefinition, error) {
	return s.Resolve(R4BaseCanonical(resourceType))
}

// BaseChain walks baseDefinition from sd until a StructureDefinition with no
// base, returning the chain excluding sd itself.
//
// This is what makes the R4 core package load-bearing rather than decorative.
// `us-core-observation-lab` does not derive from R4 `Observation` directly: its
// baseDefinition is `us-core-observation-clinical-result`, whose base is
// `Observation`. `us-core-heart-rate` goes through `us-core-vital-signs`.
// Resolving the chain proves both packages are internally consistent and that
// every profile the mapper asserts really is a constraint on the R4 resource it
// is attached to.
func (s *PackageSet) BaseChain(sd *StructureDefinition) ([]*StructureDefinition, error) {
	var chain []*StructureDefinition
	seen := map[string]bool{sd.URL: true}
	current := sd
	for current.BaseDefinition != "" {
		next, err := s.Resolve(current.BaseDefinition)
		if err != nil {
			return chain, fmt.Errorf("baseDefinition of %s: %w", current.URL, err)
		}
		if seen[next.URL] {
			return chain, fmt.Errorf("baseDefinition cycle at %s", next.URL)
		}
		seen[next.URL] = true
		chain = append(chain, next)
		current = next
	}
	return chain, nil
}

// StructuralOptions configures ValidateStructuralJSON.
type StructuralOptions struct {
	// ReportMustSupport adds an information-severity issue for every
	// must-support element a resource leaves unpopulated.
	//
	// It is off by default and never produces an error, because US Core
	// mustSupport is an obligation on the *system* ("must be able to populate
	// it when the data exists"), not on every instance. Turning a
	// mustSupport-and-absent element into a failure would fail every
	// conformant resource in the IG's own example set. What it is good for is
	// a regression signal: the set a fixture leaves unpopulated is stable, so
	// a change to it is a mapper change somebody should have to look at.
	ReportMustSupport bool
}

// ValidateStructuralJSON validates a FHIR JSON payload against the pinned
// packages. It accepts the same three shapes as ValidateJSON — a single
// resource, an array of resources, or a Bundle with entry[].resource — and
// returns an OperationOutcome the caller grades.
//
// For each non-Bundle resource it checks, in order:
//
//  1. `resourceType` is present and names a StructureDefinition in the pinned
//     R4 core package.
//  2. Every `meta.profile` canonical resolves in the pinned packages under the
//     5.1a version policy, constrains this resource type, and has a
//     baseDefinition chain that terminates in the R4 core package.
//  3. Cardinality from the R4 base snapshot: every element with `min >= 1` is
//     present and non-empty; every element with `max` of 1 is not an array;
//     every element with `max` of 0 is absent.
//  4. Cardinality from each resolved profile's snapshot, which is where US Core
//     tightens R4 — `Patient.identifier` is `0..*` in R4 and `1..*` in US Core.
//
// Cardinality is applied at every depth by walking the snapshot paths against
// the JSON in parallel, so `Coverage.payor` and `Encounter.class` are both
// checked and so is a required child of a present optional parent. Sliced
// elements are skipped (see the file comment).
func ValidateStructuralJSON(data []byte, set *PackageSet, opts StructuralOptions) (*OperationOutcome, error) {
	if set == nil {
		return nil, errors.New("structural validation requires a loaded PackageSet")
	}

	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	var issues []OperationOutcomeIssue
	switch v := raw.(type) {
	case map[string]any:
		issues = append(issues, structurallyValidateResourceOrBundle(v, set, opts, "")...)
	case []any:
		for i, item := range v {
			obj, ok := item.(map[string]any)
			if !ok {
				issues = append(issues, issueError("structure", "array element is not an object", []string{fmt.Sprintf("[%d]", i)}))
				continue
			}
			issues = append(issues, structurallyValidateResourceOrBundle(obj, set, opts, fmt.Sprintf("[%d]", i))...)
		}
	default:
		issues = append(issues, issueError("structure", "expected JSON object or array", nil))
	}

	return &OperationOutcome{ResourceType: "OperationOutcome", Issue: issues}, nil
}

func structurallyValidateResourceOrBundle(obj map[string]any, set *PackageSet, opts StructuralOptions, basePath string) []OperationOutcomeIssue {
	resourceType, ok := obj["resourceType"].(string)
	if !ok || resourceType == "" {
		return []OperationOutcomeIssue{issueError("required", "missing resourceType", at(basePath, "resourceType"))}
	}

	var issues []OperationOutcomeIssue
	if resourceType == "Bundle" {
		// A Bundle is itself a resource with cardinality rules (Bundle.type is
		// 1..1), so it is validated and then descended into.
		issues = append(issues, structurallyValidateResource(obj, resourceType, set, opts, basePath)...)
		issues = append(issues, structurallyValidateBundleEntries(obj, set, opts, basePath)...)
		return issues
	}
	return structurallyValidateResource(obj, resourceType, set, opts, basePath)
}

func structurallyValidateBundleEntries(obj map[string]any, set *PackageSet, opts StructuralOptions, basePath string) []OperationOutcomeIssue {
	entries, ok := obj["entry"].([]any)
	if !ok {
		return nil // Bundle.entry cardinality is already graded by the snapshot walk.
	}
	var issues []OperationOutcomeIssue
	for i, entry := range entries {
		entryObj, ok := entry.(map[string]any)
		if !ok {
			issues = append(issues, issueError("structure", "Bundle.entry item must be an object", at(basePath, fmt.Sprintf("entry[%d]", i))))
			continue
		}
		resObj, ok := entryObj["resource"].(map[string]any)
		if !ok {
			continue // an entry may legitimately carry request/response only.
		}
		issues = append(issues, structurallyValidateResourceOrBundle(
			resObj, set, opts, atStr(basePath, fmt.Sprintf("entry[%d].resource", i)))...)
	}
	return issues
}

func structurallyValidateResource(obj map[string]any, resourceType string, set *PackageSet, opts StructuralOptions, basePath string) []OperationOutcomeIssue {
	var issues []OperationOutcomeIssue

	base, err := set.ResolveResourceBase(resourceType)
	if err != nil {
		return []OperationOutcomeIssue{issueError("not-supported",
			fmt.Sprintf("%s is not a resource type in the pinned %s package", resourceType, PinnedR4Core),
			at(basePath, "resourceType"))}
	}
	issues = append(issues, checkSnapshot(obj, base, opts, basePath)...)

	for _, canonical := range declaredProfiles(obj) {
		profile, err := set.Resolve(canonical)
		if err != nil {
			issues = append(issues, issueError("not-found", err.Error(), at(basePath, "meta.profile")))
			continue
		}
		if profile.Type != resourceType {
			issues = append(issues, issueError("value",
				fmt.Sprintf("profile %s constrains %s, not %s", profile.URL, profile.Type, resourceType),
				at(basePath, "meta.profile")))
			continue
		}
		chain, err := set.BaseChain(profile)
		if err != nil {
			issues = append(issues, issueError("not-found", err.Error(), at(basePath, "meta.profile")))
			continue
		}
		if !chainReachesR4Base(chain, resourceType) {
			issues = append(issues, issueError("value",
				fmt.Sprintf("profile %s does not derive from %s", profile.URL, R4BaseCanonical(resourceType)),
				at(basePath, "meta.profile")))
			continue
		}
		issues = append(issues, checkSnapshot(obj, profile, opts, basePath)...)
	}

	return issues
}

func chainReachesR4Base(chain []*StructureDefinition, resourceType string) bool {
	want := R4BaseCanonical(resourceType)
	for _, sd := range chain {
		if sd.URL == want {
			return true
		}
	}
	return false
}

// declaredProfiles returns the meta.profile canonicals in declaration order,
// skipping non-string entries (their shape is graded by the snapshot walk).
func declaredProfiles(obj map[string]any) []string {
	meta, _ := obj["meta"].(map[string]any)
	profiles, _ := meta["profile"].([]any)
	out := make([]string, 0, len(profiles))
	for _, p := range profiles {
		if s, ok := p.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// checkSnapshot applies one StructureDefinition's snapshot to one resource.
//
// The snapshot is a flat, depth-ordered list of `Type.a.b.c` paths. Walking it
// in order means a parent is always graded before its children, so a child rule
// can be skipped when its parent is absent — which is the correct reading of
// FHIR cardinality: `Coverage.payor.reference` being 1..1 says nothing about a
// Coverage with no payor beyond what `Coverage.payor` already said.
func checkSnapshot(resource map[string]any, sd *StructureDefinition, opts StructuralOptions, basePath string) []OperationOutcomeIssue {
	var issues []OperationOutcomeIssue

	for _, element := range sd.Snapshot.Element {
		if element.IsSlice() || element.ContentReference != "" {
			continue
		}
		relative, ok := strings.CutPrefix(element.Path, sd.Type+".")
		if !ok {
			continue // the root element, whose cardinality is about the resource itself.
		}
		segments := strings.Split(relative, ".")
		parents := collectParents(resource, segments[:len(segments)-1])
		field := segments[len(segments)-1]
		fieldPath := sd.Type + "." + relative

		for _, parent := range parents {
			issues = append(issues, checkElement(parent.value, field, element, fieldPath, sd, opts, joinIssuePath(basePath, parent.path))...)
		}
	}

	return issues
}

// located is a JSON object reached by walking a snapshot path, with the
// dotted-and-indexed path that reached it.
type located struct {
	value map[string]any
	path  string
}

// collectParents walks segments from the resource root, expanding arrays, and
// returns every object at that path. An absent or non-object parent yields
// nothing, which is how child rules become vacuous for absent parents.
func collectParents(resource map[string]any, segments []string) []located {
	current := []located{{value: resource, path: ""}}
	for _, segment := range segments {
		var next []located
		for _, node := range current {
			for _, key := range matchingKeys(node.value, segment) {
				switch child := node.value[key].(type) {
				case map[string]any:
					next = append(next, located{value: child, path: joinJSONPath(node.path, key)})
				case []any:
					for i, item := range child {
						obj, ok := item.(map[string]any)
						if !ok {
							continue
						}
						next = append(next, located{value: obj, path: fmt.Sprintf("%s[%d]", joinJSONPath(node.path, key), i)})
					}
				}
			}
		}
		current = next
		if len(current) == 0 {
			return nil
		}
	}
	return current
}

func joinJSONPath(base, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}

// joinIssuePath joins the prefix that located a resource (empty at the top
// level, `entry[1].resource` inside a Bundle) with the path walked inside that
// resource (empty for a top-level element).
//
// Either half can be empty, and `atStr` from validate.go handles only the first
// case, so using it here rendered a Bundle's top-level findings as
// `entry[1].resource..type`. validate.go is left alone: it only ever calls
// atStr with a non-empty field.
func joinIssuePath(basePath, inner string) string {
	switch {
	case basePath == "":
		return inner
	case inner == "":
		return basePath
	default:
		return basePath + "." + inner
	}
}

// matchingKeys returns the JSON keys a snapshot path segment names.
//
// For an ordinary segment that is the segment itself. For a choice-type segment
// (`value[x]`, `onset[x]`) FHIR JSON renames the property to the chosen type —
// `valueQuantity`, `onsetDateTime` — so every key with the stem as a prefix and
// an upper-case type suffix matches. FHIR R4 §2.24.0.2 guarantees at most one
// is present, but returning all of them is what makes a `max` of 1 violation on
// a choice visible instead of silently ignored.
func matchingKeys(obj map[string]any, segment string) []string {
	stem, isChoice := strings.CutSuffix(segment, "[x]")
	if !isChoice {
		if _, present := obj[segment]; present {
			return []string{segment}
		}
		return nil
	}

	var keys []string
	for key := range obj {
		if len(key) <= len(stem) || !strings.HasPrefix(key, stem) {
			continue
		}
		suffix := key[len(stem):]
		if suffix[0] >= 'A' && suffix[0] <= 'Z' {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys) // deterministic issue order.
	return keys
}

func checkElement(parent map[string]any, field string, element ElementDefinition,
	fieldPath string, sd *StructureDefinition, opts StructuralOptions, parentPath string,
) []OperationOutcomeIssue {
	keys := matchingKeys(parent, field)
	present := len(keys) > 0
	minimum := element.MinOrZero()

	location := at(parentPath, strings.TrimSuffix(field, "[x]"))
	if present {
		location = at(parentPath, keys[0])
	}

	switch {
	case element.MaxIsZero() && present:
		return []OperationOutcomeIssue{issueError("value",
			fmt.Sprintf("%s is prohibited by %s (max 0)", fieldPath, sd.URL), location)}

	case minimum >= 1 && !present:
		return []OperationOutcomeIssue{issueError("required",
			fmt.Sprintf("%s is required by %s (min %d)", fieldPath, sd.URL, minimum), location)}

	case minimum >= 1 && present:
		if empty, why := isEmptyValue(parent[keys[0]]); empty {
			return []OperationOutcomeIssue{issueError("value",
				fmt.Sprintf("%s is required by %s (min %d) but %s", fieldPath, sd.URL, minimum, why), location)}
		}
		if count := arrayLen(parent[keys[0]]); count >= 0 && count < minimum {
			return []OperationOutcomeIssue{issueError("value",
				fmt.Sprintf("%s has %d entries, %s requires at least %d", fieldPath, count, sd.URL, minimum), location)}
		}
	}

	if present {
		if element.MaxIsOne() && len(keys) > 1 {
			return []OperationOutcomeIssue{issueError("value",
				fmt.Sprintf("%s is single-valued in %s but %d choice properties are present", fieldPath, sd.URL, len(keys)), location)}
		}

		count := arrayLen(parent[keys[0]])

		// Whether a property serialises as a JSON array is fixed by the BASE
		// resource definition, not by a profile. FHIR R4 §2.24.2 (JSON
		// representation) derives array-ness from the element's definition in
		// the specialization, and a profile constraining `max` from `*` to `1`
		// narrows the permitted *count* without changing the wire shape. So
		// `Coverage.payor` is `1..*` in R4 and `1..1` in US Core, and a
		// conformant instance still writes `"payor": [ … ]` with one entry —
		// which an earlier revision of this function reported as a violation.
		// Array-ness is therefore graded against specializations only; count is
		// graded against every StructureDefinition in play.
		if sd.Derivation == derivationSpecialization && element.MaxIsOne() && count >= 0 {
			return []OperationOutcomeIssue{issueError("value",
				fmt.Sprintf("%s is single-valued in %s but is a JSON array", fieldPath, sd.URL), location)}
		}

		if upper, bounded := upperBound(element.Max); bounded && count > upper {
			return []OperationOutcomeIssue{issueError("value",
				fmt.Sprintf("%s has %d entries, %s allows at most %d", fieldPath, count, sd.URL, upper), location)}
		}
	}

	if opts.ReportMustSupport && element.MustSupport && !present {
		return []OperationOutcomeIssue{{
			Severity:    "information",
			Code:        "informational",
			Diagnostics: fmt.Sprintf("%s is must-support in %s and is not populated", fieldPath, sd.URL),
			Location:    location,
		}}
	}

	return nil
}

// isEmptyValue reports a present-but-vacuous value. FHIR treats an empty string
// and an empty array as absent (R4 §2.24.0.1: "elements are not present if they
// have no value"), so a min-1 element carrying either is a violation, not a
// pass.
func isEmptyValue(value any) (bool, string) {
	switch v := value.(type) {
	case nil:
		return true, "is null"
	case string:
		if strings.TrimSpace(v) == "" {
			return true, "is an empty string"
		}
	case []any:
		if len(v) == 0 {
			return true, "is an empty array"
		}
	case map[string]any:
		if len(v) == 0 {
			return true, "is an empty object"
		}
	}
	return false, ""
}

// arrayLen returns the length of a JSON array, or -1 for a non-array.
func arrayLen(value any) int {
	if array, ok := value.([]any); ok {
		return len(array)
	}
	return -1
}

// derivationSpecialization is the StructureDefinition.derivation value that
// marks a base resource definition rather than a profile constraining one.
const derivationSpecialization = "specialization"

// upperBound parses ElementDefinition.max. FHIR R4 writes an unbounded upper
// cardinality as "*"; anything else is a non-negative integer.
func upperBound(elementMax string) (int, bool) {
	if elementMax == "" || elementMax == "*" {
		return 0, false
	}
	value, err := strconv.Atoi(elementMax)
	if err != nil {
		return 0, false
	}
	return value, true
}

// StructuralErrors filters an OperationOutcome down to its error-severity
// issues, which is the set that fails a fixture. Warnings and information
// issues are reported and not graded.
func StructuralErrors(outcome *OperationOutcome) []OperationOutcomeIssue {
	if outcome == nil {
		return nil
	}
	var errs []OperationOutcomeIssue
	for _, issue := range outcome.Issue {
		if issue.Severity == "error" || issue.Severity == "fatal" {
			errs = append(errs, issue)
		}
	}
	return errs
}
