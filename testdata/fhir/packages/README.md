# Pinned FHIR IG packages

Twenty-three offline FHIR packages, in the npm-style `.tgz` form the FHIR
package registry publishes. The first two are the resolution source for the Go
structural validator in `pkg/fhir/structural.go` (Slice 5.1b, Option C). All
twenty-three are the package cache the HL7 validator runs against in
`scripts/fhir-official-validate.sh` (Slice 5.1c-β, Option A) — see "The
official validator's closure" below.

| File | Package | Version | Bytes | SHA-256 (verified by `TestFHIRStructural_PinnedPackagesMatchTheirRecordedDigests`) | SHA-1 (`dist.shasum` published by the registry) |
|---|---|---|---|---|---|
| `hl7.fhir.r4.core-4.0.1.tgz` | `hl7.fhir.r4.core` | 4.0.1 | 12,815,597 | `ebd7731df7d36b5b7d39d5fb6c9d77b44bb7fe5742f1a2e87f164738c3289d44` | `0e4b8d99f7918587557682c8b47df605424547a5` |
| `hl7.fhir.us.core-9.0.0.tgz` | `hl7.fhir.us.core` | 9.0.0 | 2,749,959 | `d7b54d2ec2a48cea94ffea5d939ad67a681f80b94d69594a08cebac36da9e059` | `bc9955981955135d84e3348b6ef43c76ea95e1be` |

`SHA256SUMS` is the machine-readable form of the SHA-256 column of both tables
and is what the test and the official-validator script read. Every archive in
this directory must be recorded there and every recorded archive must be here
(`TestFHIRStructural_PinnedPackagesMatchTheirRecordedDigests`). When a pin
deliberately moves, update the line for that archive only.

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
  pinned `TRIVY_VERSION` — does not decompress `.tgz`. Probed twice. Against a
  directory holding only these two files,
  `trivy fs --scanners vuln --exit-code 1 --severity CRITICAL .` reported
  `Number of language-specific files num=0` and exited 0, and
  `trivy fs --scanners secret --exit-code 1 --severity HIGH,CRITICAL .` reported
  no issues and exited 0. Then against the whole branch tree with both archives
  in place, using the job's full command line including its `--skip-dirs` flags:
  both gates exited 0 again, and the archives contributed no scan target. No
  `.trivyignore` entry and no `--skip-dirs` addition is needed; adding one would
  have suppressed findings nobody had shown existed.
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

## The official validator's closure (Slice 5.1c-β, 2026-09-24)

`validator_cli.jar` 6.10.4 will not validate against the two archives above
alone. Run with `--network none` and only those two in its package cache, it
stops at `Unable to resolve package id hl7.fhir.r4.core#4.0.1`; with the core
pre-placed, at `hl7.fhir.xver-extensions#0.1.0`; and so on. It demands two
kinds of package:

- **Its own defaults**, loaded before any IG (`ValidationService.java` at the
  6.10.4 tag): `hl7.fhir.xver-extensions#0.1.0`, the THO and extensions
  "working versions" `hl7.terminology.r4#6.2.0` and
  `hl7.fhir.uv.extensions.r4#5.2.0`, and then *unversioned*
  `hl7.terminology` and `hl7.fhir.uv.extensions` — "latest". Online, latest
  is the registry's latest; offline, the validator takes the newest version in
  its cache. The two are pinned at the registry's latest on 2026-09-24
  (`7.4.0`, `5.3.0`), so the offline run reproduces an online run of that
  date, and they only move when a human moves them.
- **US Core 9.0.0's declared dependencies, transitively.** The validator loads
  every non-core dependency of every package it loads. `hl7.fhir.r5.core` is
  declared (by the R5-flavoured THO and extensions packages) but never loaded —
  the validator skips core packages as dependencies — so it is not pinned.

Every one was downloaded from `packages.fhir.org` on 2026-09-24 and reproduces
the registry's `dist.shasum` byte for byte. None is licence-gated (CC0-1.0,
CC-BY-4.0, HHS open data). `us.nlm.vsac` is **not** a US Core 9.0.0 dependency
and is never demanded; VSAC value sets (`cts.nlm.nih.gov`) surface in the
findings ledger as `ValueSet … not found` warnings.

| File | Package | Bytes | Demanded by | Licence | SHA-256 | SHA-1 (`dist.shasum`) |
|---|---|---|---|---|---|---|
| `hl7.fhir.r4.examples-4.0.1.tgz` | `hl7.fhir.r4.examples#4.0.1` | 20,535,196 | SDC 4.0.0 | CC0-1.0 | `e18b31e7a52145a31f1f3f409cf6847583b08b7306cc4e9460a95a7b5efba930` | `ef0ea43649d05ff246b471cad4583b662f1f27b5` |
| `hl7.fhir.uv.extensions-5.1.0-snapshot1.tgz` | `hl7.fhir.uv.extensions#5.1.0-snapshot1` | 2,987,110 | smart-app-launch 2.2.0 | CC0-1.0 | `8a1f92c1e77535cb2ae21b69028de81a21ea27bd24d0a76c43c7224b8f46b35c` | `c9b500af54cea20afb742c9773a57ba422557376` |
| `hl7.fhir.uv.extensions-5.3.0.tgz` | `hl7.fhir.uv.extensions#5.3.0` | 1,298,568 | validator ("latest" extensions, pinned 2026-09-24) | CC0-1.0 | `6e3a3f9929e05d2b813a3f98fa1ad2f5122fb6b2819fa73e6c5520002fb2c5b0` | `f0dda54c204935cd971854e7ce3d091fdcd934a5` |
| `hl7.fhir.uv.extensions.r4-1.0.0.tgz` | `hl7.fhir.uv.extensions.r4#1.0.0` | 3,014,049 | hl7.terminology#5.5.0 | CC0-1.0 | `b270eb5b0ea3d015398a5af6c86cab2718070cbb850e366ef050ce6d6eb4a9d3` | `d4f95213cd55173fb5bce87c02eba3ecdace3cb2` |
| `hl7.fhir.uv.extensions.r4-5.2.0.tgz` | `hl7.fhir.uv.extensions.r4#5.2.0` | 1,302,452 | validator (extensions working version) | CC0-1.0 | `b406e75575f05676559d0759770c5939d023ee72fb2ef38e0b3259328487720a` | `2c7e21436dfa35ba7209696d63247538b851968a` |
| `hl7.fhir.uv.extensions.r4-5.3.0-ballot-tc1.tgz` | `hl7.fhir.uv.extensions.r4#5.3.0-ballot-tc1` | 776,066 | SDC 4.0.0 | CC0-1.0 | `5f5d1e88052d615453e6b7a1eb4f885df36590f14b52974e5860485d16819bd9` | `4890812026915c293a938df552cd69eaa67520bd` |
| `hl7.fhir.uv.extensions.r4-5.3.0.tgz` | `hl7.fhir.uv.extensions.r4#5.3.0` | 811,403 | US Core 9.0.0 | CC0-1.0 | `dfbc3ac95df91ed845cc6b60920d2875f679361fa0e16f227cee93c4a9ab2104` | `e03679fe759094ed670b637e6e376717659fa03b` |
| `hl7.fhir.uv.extensions.r5-5.2.0.tgz` | `hl7.fhir.uv.extensions.r5#5.2.0` | 1,301,013 | hl7.terminology.r5#7.1.0 | CC0-1.0 | `e02fc6eff0f37c1611a35aa93ef9aaa3b55ab7371a5c96f4d2a5834106a13170` | `90d77a988dd6a75452961a187783714d02265a90` |
| `hl7.fhir.uv.sdc-4.0.0.tgz` | `hl7.fhir.uv.sdc#4.0.0` | 1,196,457 | US Core 9.0.0 | CC0-1.0 | `d785be8474c7ec7988e32e326430d9ca4aeb2cac4a1f195022e4d6f7dc5c5291` | `d3f58dd5761e6728a4312c523011e2bc3096d29c` |
| `hl7.fhir.uv.smart-app-launch-2.2.0.tgz` | `hl7.fhir.uv.smart-app-launch#2.2.0` | 100,390 | US Core 9.0.0 | CC-BY-4.0 | `0bb498dd09677684239f24d279b6da711cd814449f2f473c2970fbe881ec9c70` | `2e5b7caacf87523082507f6af2efdf25c24ad3d8` |
| `hl7.fhir.uv.tools.r4-1.1.2.tgz` | `hl7.fhir.uv.tools.r4#1.1.2` | 154,902 | SDC 4.0.0 | CC0-1.0 | `a1f166f8808629a40c4acabc16a4fbfd164d9f38f9db95b6f4b38bb69155dfe4` | `8cabc086f7bc00fe180fd564a27b397827b4e8f1` |
| `hl7.fhir.uv.xver-r5.r4-0.1.0.tgz` | `hl7.fhir.uv.xver-r5.r4#0.1.0` | 12,317,273 | US Core 9.0.0 | CC0-1.0 | `7ee6f04d78ced803dd567559a0d178bafbd2d3b71db61bbbf6b15c796a1a664d` | `ee9f685550128619aac197f07f2a1c34311d4e95` |
| `hl7.fhir.xver-extensions-0.1.0.tgz` | `hl7.fhir.xver-extensions#0.1.0` | 183,877 | validator (core load) | CC0-1.0 | `f3bb9fa2083402e88a02b41f433655274e8a1cca563211c8f7ba6fd0badf537a` | `19669673a5fa2934fbd5fe9ce394d002ae3e5f04` |
| `hl7.terminology-5.5.0.tgz` | `hl7.terminology#5.5.0` | 5,038,371 | smart-app-launch 2.2.0 | CC0-1.0 | `6d0947f9be882c06c29c693b784b751e8bd8700acdc0cf042bf4ce48d6b16698` | `db33c3b3204dd21e5c9d392aaa3e6c16e2d90ac1` |
| `hl7.terminology-7.4.0.tgz` | `hl7.terminology#7.4.0` | 5,615,088 | validator ("latest" THO, pinned 2026-09-24) | CC0-1.0 | `b321ad4ee1c18798abd8692f45bc86550cffac72ae97413125cfb82f0112c72d` | `b1a16b9e65716821c5ef576bf78906f139456b89` |
| `hl7.terminology.r4-6.2.0.tgz` | `hl7.terminology.r4#6.2.0` | 5,661,217 | validator (THO working version) | CC0-1.0 | `79404c9cc95491fc0155627cd039c401a6eb4748175328131e91b709a41300e2` | `c445f8807f49c9d4dbaf106816fb97ca005494f1` |
| `hl7.terminology.r4-6.5.0.tgz` | `hl7.terminology.r4#6.5.0` | 5,018,208 | hl7.fhir.uv.extensions.r4#5.3.0-ballot-tc1 | CC0-1.0 | `a28b638483a11df696ed92198276236d759e41c7a3a8960c9e7e7d0a1185bd06` | `8a7a096866b9b6e96e288ce8d72bf99aa31714a3` |
| `hl7.terminology.r4-7.1.0.tgz` | `hl7.terminology.r4#7.1.0` | 4,709,916 | US Core 9.0.0 | CC0-1.0 | `1cb0cd5601972925fcd04f2c175d9cc63a9ecd9346a91a6e735f1a865dc5fba1` | `19a681d8bd598b5c0da72e058616d78ae70940c0` |
| `hl7.terminology.r5-5.3.0.tgz` | `hl7.terminology.r5#5.3.0` | 3,600,291 | hl7.fhir.uv.extensions#5.1.0-snapshot1 | CC0-1.0 | `5366fbaf1b8a3d0bb69099a659d0a01377a541df284146d71f3a7dbea889d6c3` | `6dba119329face8e46c2b49327fd032cf20fc541` |
| `hl7.terminology.r5-7.1.0.tgz` | `hl7.terminology.r5#7.1.0` | 4,719,976 | hl7.fhir.uv.extensions#5.3.0 | CC0-1.0 | `473303108b5607aad7b910581739e8bbf3e61a625ed9e739c66e0ca597d4aefe` | `e68789e8b2d0d537434efffbf79d8bb6a2276336` |
| `us.cdc.phinvads-0.12.0.tgz` | `us.cdc.phinvads#0.12.0` | 18,905,258 | US Core 9.0.0 | HHS/Open | `cf565971a6bcca3193de4d7ff71f5c443fc4e00adaeef8c88416d21c778e3a3f` | `cb6aacb99bb21d52a30ff1dc5b71a0629c951027` |

**99,247,081 bytes across 21 archives**, dominated by `hl7.fhir.r4.examples`
(20.5 MB, reached only through SDC 4.0.0), `us.cdc.phinvads` (18.9 MB) and
`hl7.fhir.uv.xver-r5.r4` (12.3 MB). Minimal by construction: with exactly these
23 archives in its cache, the validator's own `Package Summary` lists all 23 —
none is spare — and `scripts/fhir-official-validate.sh` fails if that ever
stops being true in either direction.

**`security:trivy` is still inert on them.** Trivy 0.63.0 against a directory
holding only the 23 archives, with the job's exact `--skip-dirs` flags:
`--scanners vuln --exit-code 1 --severity CRITICAL` reported
`Number of language-specific files num=0` and exited 0; `--scanners secret
--exit-code 1 --severity HIGH,CRITICAL` reported no issues and exited 0. It
still does not decompress `.tgz`.

Provenance, for any one of them:

```sh
curl -L -o <name>-<version>.tgz https://packages.fhir.org/<name>/<version>
curl -s https://packages.fhir.org/<name> | jq -r '.versions["<version>"].dist.shasum'   # compare with: shasum -a 1 <file>
```
