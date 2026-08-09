### 2026-08-08

- What changed:
  - Landed Phase 4 Slice 4.1b2: verified MLLP client-certificate service identity
    and fail-closed submit authorization.
  - Added `clients.identities` to the immutable, content-addressed MLLP source
    revision (`internal/integration/mllp/identity.go`, `source.go`). Entries map an
    authority-scoped URI SAN and/or SPKI SHA-256 pin to one canonical service
    subject plus grants. Existing revisions keep their exact digest because the
    field is omitted from the canonical digest input when empty.
  - Resolved one `ConnectionIdentity` per connection immediately after the TLS
    handshake and before any frame read (`server.go`); zero-match and ambiguous
    multi-match certificates close without an acknowledgement.
  - Carried the verified subject/auth-method/grants into the existing
    `authorization.AuthorizeSubmission` decision and moved that decision ahead of
    capacity acquisition and envelope construction (`service.go`).
  - Wired `FI_FHIR_MLLP_REQUIRE_CLIENT_IDENTITY`, forwarded the full MLLP env
    contract through Docker Compose, and extended
    `scripts/check-runtime-config.sh` with a required MLLP compose check.
  - Extended the required `test:mllp-runtime` job and `make mllp-runtime` to
    discover and run both MLLP runtime proofs (minimal `.gitlab-ci.yml` diff: the
    `-list` regex, the `rg` pattern, the expected count, and the job comment;
    `ci/integration-ci-hardening` owns that file this sprint).
- Why:
  - Slice 4.1b1 left MLLP attributing every connection to one deployment-fixed
    principal, so a CA-valid certificate from any sender the client CA trusts was
    indistinguishable at the authorization decision.
- What's next:
  - 4.1b3: bind batch connector/workload identity and replace remote object
    modification time as trusted receipt provenance.
  - 4.1c: destination-scoped identity and secret resolution for the first durable
    HTTPS consumer, then audit and PHI controls.
- Sources:
  - [S1] Kill test: `POSTGRES_TEST_URL=... make mllp-runtime` (both
    `TestPostgresMLLPRuntime_DurableACKPauseRestart` and
    `TestPostgresMLLPRuntime_CertificateIdentityAuthorization` passed with `-race`)
  - [S2] Negative controls: silent fallback for unmatched certificates, ignored
    per-identity grants, and a deployment-fixed principal in mapped mode each
    failed the kill test at assertions 2, 3, and 1 respectively
  - [S3] `.loom/iteration-plan-phase-4-slice-4-1b2-mllp-certificate-identity.md`
  - [S4] RFC 5280 §4.2.1.6 (URI SAN), RFC 6125 §6.4.4 (common-name deprecation)
