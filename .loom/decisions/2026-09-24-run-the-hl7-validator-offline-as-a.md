### 2026-09-24: Run the HL7 validator offline as a CI-only gate

- Decision:
  - **Option A lands as a CI-only, blocking, offline gate.** `validator_cli.jar`
    **6.10.4** (GitHub release 2026-09-04, Git# `1b90fb13f77b`, 200,928,617
    bytes, sha256 `1106b9d58f9e363e47bea7c4fc065841e5fc91fe9d062775c3bfdd212bd653cc`,
    reproducing the release asset's published digest) validates the 25 mapper
    fixtures and the transaction Bundle the durable `fhir` transport delivers
    for each of the 11 supported event types, against R4 4.0.1 +
    `hl7.fhir.us.core#9.0.0`. The findings are compared to
    `testdata/fhir/official/findings.ledger.txt` by **exact equality** (gate
    `scripts/fhir-official-validate.sh`, `make fhir-official`, CI
    `test:fhir-official-capture` → `test:fhir-official`).
  - **Pinning method.** The jar is pinned by version *and* sha256 in the script,
    downloaded in the job from the upstream GitHub release, verified before use
    (fail closed), and kept outside the checkout (`/tmp`, or a docker volume for
    the local recipe). It is not vendored and never enters the shipped image.
    The JRE is `eclipse-temurin:21.0.12_8-jre-resolute`, tag and digest in
    `TEMURIN_JRE_IMAGE_REF`, pulled through `DOCKERHUB_CACHE_PREFIX` like every
    other image.
  - **Transitive packages are pinned — 21 of them, in-tree.** The kill-test
    refuted "two archives are enough": with `--network none` the validator
    demands its own defaults (`hl7.fhir.xver-extensions#0.1.0`, THO
    `hl7.terminology.r4#6.2.0`, `hl7.fhir.uv.extensions.r4#5.2.0`, and
    unversioned "latest" `hl7.terminology` and `hl7.fhir.uv.extensions`) and US
    Core 9.0.0's full transitive dependency closure. Exactly the demanded set is
    pinned under `testdata/fhir/packages/` (99,247,081 bytes), each reproducing
    the registry `dist.shasum`, each recorded in `SHA256SUMS`. The two "latest"
    packages are pinned at the registry's latest on 2026-09-24 (`7.4.0`,
    `5.3.0`), because offline the validator takes the newest version in its
    cache.
  - **Terminology posture: `-tx n/a`, and it stays that way.** No terminology
    server, ever, in CI. Code-system membership is still checked where the code
    system's content ships in the pinned THO packages (it caught `LAB` in
    `observation-category`, `discharge` in `careplan-category`, `physician` in
    `practitioner-role`); everything that needs a server — LOINC, SNOMED CT,
    RxNorm, UCUM, CVX membership, and every VSAC value set — is recorded as the
    warning or information the validator emits for "could not check". None of
    the 21 packages is licence-gated, and `us.nlm.vsac` is not a US Core 9.0.0
    dependency, so nothing was obtained by unofficial means.
  - **Size trade-off, stated.** The repository pack is 565 MiB today, dominated
    by the committed `.tmp/go-mod-cache/`. Slice 5.1b added 15.5 MB of archives;
    this adds ~99 MB. The dominant single cost is `hl7.fhir.r4.examples#4.0.1`
    (20.5 MB), reached only through `hl7.fhir.uv.sdc#4.0.0`, followed by
    `us.cdc.phinvads#0.12.0` (18.9 MB) and `hl7.fhir.uv.xver-r5.r4#0.1.0`
    (12.3 MB). No size gate rejected them: there is still no `.gitattributes`,
    no LFS and no file-size lint, and `security:trivy` is inert on `.tgz`.
    **Escape hatch** if a size gate ever does reject in-tree archives: move them
    to the GitLab generic package registry on project 19 and keep in-repo
    `SHA256SUMS` verification of every download — the digest record stays in
    the tree, only the bytes move.
- Rationale:
  - The 2026-08-08 decision's ratified confinement half — CI-only, not in the
    image, never reaching `packages.fhir.org`, IG archives pinned where trivy
    scans them — is satisfied by construction and checked on every run: the
    script builds a private package cache from exactly the 23 archives (never
    `~/.fhir`), passes `-no-http-access`, fails if the validator installs
    anything, and fails if the validator's own `Package Summary` differs from
    the pinned set in either direction (a spare pin, or a package from
    somewhere else).
  - **Offline equals online.** An online control run — empty cache, network
    allowed, `-tx n/a` — resolved the identical 23-package summary and produced
    identical findings, first on `patient.json`, then on the full 36-file
    input set. Two offline runs were identical. The ledger is therefore what the
    public validator says today, not an artefact of the pinning.
  - Exact equality, for the same reason `recordedCardinalityGaps()` uses it: a
    tolerance that only ratchets down still lets a finding outlive its cause.
  - Pinning today's "latest" rather than the oldest version already in the
    closure keeps the offline run faithful to the validator's own policy
    ("always the newest THO") as of a named date, and makes moving it a
    reviewed edit instead of a silent drift.
- Alternatives considered:
  - **Run against the two archives and let the validator fetch the rest.**
    Rejected: it breaks the ratified "never reaches packages.fhir.org" rule, and
    "latest" would float under the gate.
  - **Load US Core as a plain folder of resources so no dependency is loaded.**
    Rejected: it is not how the validator is meant to be run, it would turn
    every THO/VSAC/SDC reference into an unresolved-reference finding, and the
    evidence would stop being "what the official validator says".
  - **Fetch the closure in the job from the registry, checked against in-repo
    digests.** Rejected for now (the coordinator ruled in-tree, consistent with
    5.1b); it is the named escape hatch above if a size gate changes.
  - **A terminology server (tx.fhir.org or self-hosted).** Rejected: egress of
    resource content, a moving target under a blocking gate, and licence terms
    for SNOMED/VSAC content. Not in scope for a hermetic gate.
  - **Validate only the fixtures.** Rejected: the delivered Bundles are what a
    FHIR server actually receives (conditional references, rewritten ids,
    ensured identifiers), and they carry findings the fixtures do not
    (`Reference_REF_CantMatchChoice`, the test-input OID).
- Consequences:
  - Two blocking CI jobs on `.go-mr-rules`; Java exists only in the second.
  - Every mapper change that alters a finding must regenerate the ledger in the
    same commit. Lane S7-A's cardinality fixes shrink it; whoever merges second
    rebases and regenerates.
  - The ledger is not a clean bill of health: at landing it holds 163 findings
    over 36 inputs, 41 of them errors (cardinality, invariants, slicing, local
    code-system membership, identifier/URL datatype checks). They are recorded,
    not accepted — each one is now visible and cannot silently change.
  - Moving the validator version, the JRE image or any pinned archive is a
    deliberate edit that regenerates the ledger; the ledger header carries the
    jar version, digest, invocation and package summary so a pin change without
    regeneration fails.
- Sources:
  - [S1] `.loom/35-sprint7-execution-specs.md` — Lane S7-B, the riskiest
    assumption of the sprint.
  - [S2] `.loom/decisions/2026-08-08-fhir-conformance-validation-strategy-for-slice-5.md`
    — the ratified confinement half.
  - [S3] `hapifhir/org.hl7.fhir.core` @ 6.10.4: `ValidationService.java`
    (`buildValidationEngine`, `loadIgsAndExtensions`),
    `FilesystemPackageCacheManager.java` (`getLatestVersion` →
    `getLatestVersionFromCache`), `IgLoader.java` (`loadIg`, transitive
    dependency load, core packages skipped).
  - [S4] `testdata/fhir/packages/README.md` — per-archive provenance, licence,
    "demanded by", and the trivy evidence.
  - [S5] `docs/planning/FHIR-CONFORMANCE-MATRIX.md` §5.2.
