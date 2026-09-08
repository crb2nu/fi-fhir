# Pinned FHIR IG packages

Two offline FHIR implementation-guide packages, in the npm-style `.tgz` form the
FHIR package registry publishes. They are the resolution source for the Go
structural validator in `pkg/fhir/structural.go` (Slice 5.1b, Option C).

| File | Package | Version | Bytes | SHA-256 (verified by `TestFHIRStructural_PinnedPackagesMatchTheirRecordedDigests`) | SHA-1 (`dist.shasum` published by the registry) |
|---|---|---|---|---|---|
| `hl7.fhir.r4.core-4.0.1.tgz` | `hl7.fhir.r4.core` | 4.0.1 | 12,815,597 | `ebd7731df7d36b5b7d39d5fb6c9d77b44bb7fe5742f1a2e87f164738c3289d44` | `0e4b8d99f7918587557682c8b47df605424547a5` |
| `hl7.fhir.us.core-9.0.0.tgz` | `hl7.fhir.us.core` | 9.0.0 | 2,749,959 | `d7b54d2ec2a48cea94ffea5d939ad67a681f80b94d69594a08cebac36da9e059` | `bc9955981955135d84e3348b6ef43c76ea95e1be` |

`SHA256SUMS` is the machine-readable form of column 5 and is what the test
reads. Regenerate it with `shasum -a 256 *.tgz > SHA256SUMS` only when a pin
deliberately moves.

## Provenance

Downloaded 2026-09-08 from the FHIR package registry:

```sh
curl -L -o hl7.fhir.r4.core-4.0.1.tgz  https://packages.fhir.org/hl7.fhir.r4.core/4.0.1
curl -L -o hl7.fhir.us.core-9.0.0.tgz  https://packages.fhir.org/hl7.fhir.us.core/9.0.0
```

`packages.fhir.org` redirects to `packages.simplifier.net`. The registry
publishes an npm-style `dist.shasum` (SHA-1) per version; both files reproduce
theirs byte for byte, which is the independent half of the provenance check —
the SHA-256 column proves the bytes have not changed since *we* fetched them,
the SHA-1 column proves they are the bytes *upstream* published.

## Why the whole archives and not an extracted subset

`.loom/34-sprint6-execution-specs.md` named "the package `.tgz` files can live
in the tree" as this lane's riskiest assumption, with partial vendoring by
extraction script as the fallback if a size or scan gate rejected them. Every
gate was checked and none rejects them, so the whole archives are pinned:

- **No size gate.** There is no `.gitattributes`, no Git LFS configuration, and
  no file-size lint in `scripts/` or `.gitlab-ci.yml`. 15.5 MB across two blobs
  that never change again is a one-time cost paid on clone.
- **`security:trivy` (filesystem scan) is inert on them.** Trivy 0.63.0 — the
  pinned `TRIVY_VERSION` — does not decompress `.tgz`. Probed with the exact CI
  commands against a directory holding only these two files:
  `trivy fs --scanners vuln --exit-code 1 --severity CRITICAL .` reported
  `Number of language-specific files num=0` and exited 0, and
  `trivy fs --scanners secret --exit-code 1 --severity HIGH,CRITICAL .` reported
  no issues and exited 0. No `.trivyignore` entry and no `--skip-dirs` addition
  is needed; adding one would have suppressed findings nobody had shown existed.
- **`security:trivy-image` never sees them.** `.dockerignore:20` is `testdata/`,
  so the builder stage's `COPY . .` excludes this directory, and the runtime
  stage is `gcr.io/distroless/static-debian12:nonroot` carrying only the
  binary. This is the ratified confinement half of the 2026-08-08 validation
  decision: nothing from an IG package enters the shipped image.

Keeping the archives whole also keeps the packages *citable*. An extracted
subset would make `FHIR-CONFORMANCE-MATRIX.md` §4's external denominator (the
US Core profile inventory) a claim about our extraction script rather than
about the IG.

## Layout the loader depends on

Both archives use the FHIR package layout: every resource is a JSON file
directly under `package/`, with `package/package.json` carrying `name` and
`version`. The US Core archive additionally has `package/example/`,
`package/openapi/`, `package/other/` and `package/xml/` subdirectories, and the
R4 core archive has sibling top-level `openapi/` and `other/` directories.
`pkg/fhir/structural.go` reads only `package/package.json` and
`package/StructureDefinition-*.json`, so examples and non-JSON representations
are never parsed.

| Archive | Entries | `package/StructureDefinition-*.json` |
|---|---|---|
| `hl7.fhir.r4.core-4.0.1.tgz` | 5,046 | 658 |
| `hl7.fhir.us.core-9.0.0.tgz` | 521 | 70 |
