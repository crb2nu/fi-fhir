#!/usr/bin/env bash
#
# fhir-official-validate.sh — Slice 5.1c-β: the HL7 FHIR validator
# (validator_cli.jar, "Option A") run OFFLINE over the mapper fixtures and the
# transaction Bundles the durable `fhir` transport delivers, compared by EXACT
# EQUALITY against a checked-in findings ledger.
#
# What it validates (every run, one validator invocation):
#   mapper/*.json   testdata/fhir/mapper/*.json — the mapper's own output
#   bundles/*.json  $FHIR_OFFICIAL_BUNDLES_DIR — one `<event_type>.bundle.json`
#                   per supported event type, written by
#                   TestDeliveredBundlesForEverySupportedEventType
#                   (internal/integration/fhirout) when FI_FHIR_FHIR_CAPTURE_DIR
#                   is set; byte-identical to the transport's request body.
# against FHIR R4 4.0.1 + hl7.fhir.us.core#9.0.0, with `-tx n/a` (no
# terminology server) and `-no-http-access`.
#
# Hermeticity (.loom/decisions/2026-08-08-fhir-conformance-validation-strategy-for-slice-5.md):
#   * The validator resolves EVERY package from a private package cache this
#     script builds, per run, from the 23 archives pinned under
#     testdata/fhir/packages/ (verified against SHA256SUMS first). The cache
#     lives under the work directory (`-Duser.home`), never ~/.fhir, so a
#     developer's own cache cannot change what "latest" resolves to.
#   * `-no-http-access` makes the validator refuse every HTTP(S) request. It
#     still *attempts* a latest-version lookup for the two unversioned packages
#     it always loads (hl7.terminology, hl7.fhir.uv.extensions) and logs
#     "Access to the internet is not allowed by local security policy"; it then
#     takes the newest version in the pinned cache. The script fails if the
#     validator installs anything, or if its package summary and the pinned
#     archive set differ in either direction.
#   * Locally the validation runs in a `--network none` container, so the same
#     property also holds at the socket level.
#
# The jar (~200 MB) is never vendored and never written into the checkout. It
# is downloaded by version from the upstream GitHub release, verified against
# the sha256 below (fail closed: a mismatch deletes it and exits non-zero), and
# kept OUTSIDE the repository in $FHIR_OFFICIAL_CACHE_DIR (default
# ${TMPDIR:-/tmp}/fi-fhir-official; the script refuses a cache dir inside the
# repository). In docker mode it lives in a docker volume on the build host.
#
# Modes:
#   gate              validate, then diff against testdata/fhir/official/
#                     findings.ledger.txt. A NEW finding fails; a VANISHED
#                     finding fails ("shrink the ledger in the same commit that
#                     fixes it"). The ledger can only change deliberately.
#   negative-control  the same run with Patient.name removed from a COPY of
#                     mapper/patient.json (the fixture is untouched). Passes only
#                     if the ledger diff is confined to exactly that file and
#                     includes an error naming Patient.name.
#   update            validate and rewrite the ledger (a deliberate regeneration).
#
# Environment:
#   FHIR_OFFICIAL_BUNDLES_DIR      captured bundles (required)
#   FHIR_OFFICIAL_OUT_DIR          where ledger + validator log are written
#                                  (default: a fresh temp dir)
#   FHIR_OFFICIAL_DOCKER_CONTEXT   run the JRE stage in a container on this
#                                  docker context (local recipe; e.g. 7900xtx)
#   FHIR_OFFICIAL_JRE_IMAGE        image for docker mode; keep in step with
#                                  TEMURIN_JRE_IMAGE_REF in ci/test-fhir-official.yml
#   FHIR_OFFICIAL_CACHE_DIR        jar cache outside the checkout (non-docker)
#   FHIR_OFFICIAL_JAVA_OPTS        default -Xmx2g
#
# Local recipe (no local JRE needed):
#   FHIR_OFFICIAL_DOCKER_CONTEXT=7900xtx make fhir-official
#   FHIR_OFFICIAL_DOCKER_CONTEXT=7900xtx make fhir-official-negative-control
#   FHIR_OFFICIAL_DOCKER_CONTEXT=7900xtx make fhir-official FHIR_OFFICIAL_UPDATE=1

set -euo pipefail

# ---- pins -------------------------------------------------------------------
VALIDATOR_VERSION="6.10.4"
VALIDATOR_SHA256="1106b9d58f9e363e47bea7c4fc065841e5fc91fe9d062775c3bfdd212bd653cc"
VALIDATOR_URL="https://github.com/hapifhir/org.hl7.fhir.core/releases/download/${VALIDATOR_VERSION}/validator_cli.jar"
FHIR_VERSION="4.0.1"
FHIR_IG="hl7.fhir.us.core#9.0.0"
DEFAULT_JRE_IMAGE="eclipse-temurin:21.0.12_8-jre-resolute@sha256:7dd2336d132aaf34c980e4277a1828a7da68b67684bff3d5ff1877f52c117656"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LEDGER="${ROOT}/testdata/fhir/official/findings.ledger.txt"
PACKAGES_DIR="${ROOT}/testdata/fhir/packages"
MAPPER_DIR="${ROOT}/testdata/fhir/mapper"

log() { printf 'fhir-official: %s\n' "$*" >&2; }
die() {
	printf 'fhir-official: ERROR: %s\n' "$*" >&2
	exit 1
}

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	else
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

# ---- in-container / in-JRE stage ---------------------------------------------

# validate_inputs <indir> <outdir> <jar>
# indir holds mapper/, bundles/ and packages/ (the 23 archives + SHA256SUMS).
# Writes <outdir>/findings.ledger.txt and <outdir>/validator.log.
validate_inputs() {
	local indir="$1" outdir="$2" jar="$3"
	mkdir -p "${outdir}"
	command -v java >/dev/null 2>&1 || die "java is not on PATH (set FHIR_OFFICIAL_DOCKER_CONTEXT to run in a container)"

	[ "$(sha256_of "${jar}")" = "${VALIDATOR_SHA256}" ] || die "${jar} does not have the pinned sha256 ${VALIDATOR_SHA256}"

	# 1. Every archive is exactly what SHA256SUMS records, and every archive is recorded.
	local sums="${indir}/packages/SHA256SUMS" archive name recorded
	[ -f "${sums}" ] || die "missing ${sums}"
	local count=0
	for archive in "${indir}"/packages/*.tgz; do
		[ -f "${archive}" ] || die "no .tgz archives under ${indir}/packages"
		name="$(basename "${archive}")"
		recorded="$(awk -v f="${name}" '$2 == f || $2 == "*" f {print $1}' "${sums}")"
		[ -n "${recorded}" ] || die "${name} is not recorded in SHA256SUMS"
		[ "$(sha256_of "${archive}")" = "${recorded}" ] || die "${name} does not match its SHA256SUMS digest"
		count=$((count + 1))
	done
	[ "${count}" -eq "$(awk 'NF == 2' "${sums}" | wc -l | tr -d ' ')" ] || die "SHA256SUMS records an archive that is not on disk"

	# 2. A private package cache holding exactly the pinned set, keyed the way
	#    the validator's FilesystemPackageCacheManager expects: <name>#<version>.
	#    It is the validator's user.home for this run only, and is not part of
	#    the output (extracted, it is several hundred MB).
	local home cache pj id
	home="$(mktemp -d "${TMPDIR:-/tmp}/fi-fhir-official-home.XXXXXX")"
	cache="${home}/.fhir/packages"
	mkdir -p "${cache}"
	: >"${outdir}/pinned-packages.txt"
	for archive in "${indir}"/packages/*.tgz; do
		pj="$(tar -xzOf "${archive}" package/package.json)"
		id="$(printf '%s' "${pj}" | grep -o '"name" *: *"[^"]*"' | head -1 | sed 's/.*: *"\(.*\)"/\1/')#$(printf '%s' "${pj}" | grep -o '"version" *: *"[^"]*"' | head -1 | sed 's/.*: *"\(.*\)"/\1/')"
		case "${id}" in "#"* | *"#") die "cannot read name/version from $(basename "${archive}")" ;; esac
		mkdir -p "${cache}/${id}"
		tar -xzf "${archive}" -C "${cache}/${id}"
		printf '%s\n' "${id}" >>"${outdir}/pinned-packages.txt"
	done

	# 3. One validator run over every input.
	local inputs=() f
	for f in "${indir}"/mapper/*.json "${indir}"/bundles/*.json; do
		[ -f "${f}" ] && inputs+=("${f}")
	done
	[ "${#inputs[@]}" -gt 0 ] || die "no inputs under ${indir}"
	local out_json="${outdir}/validator-output.json" java_opts
	rm -f "${out_json}"
	read -r -a java_opts <<<"${FHIR_OFFICIAL_JAVA_OPTS:--Xmx2g}"
	log "validating ${#inputs[@]} files with validator_cli.jar ${VALIDATOR_VERSION} (offline, -tx n/a)"
	set +e
	java "${java_opts[@]}" -Duser.home="${home}" -jar "${jar}" "${inputs[@]}" \
		-version "${FHIR_VERSION}" -ig "${FHIR_IG}" -tx n/a -txCache n/a -no-http-access \
		-output-style json -output "${out_json}" >"${outdir}/validator.log" 2>&1
	local status=$?
	set -e
	rm -rf "${home}"
	# The exit code reflects the findings (0 or 1), not whether the run worked;
	# the output file and the package summary are the infrastructure checks.
	if [ "${status}" -gt 1 ] || [ ! -s "${out_json}" ]; then
		tail -40 "${outdir}/validator.log" >&2 || true
		die "validator run failed (exit ${status}); full log: ${outdir}/validator.log"
	fi
	if grep -q '^Installing ' "${outdir}/validator.log"; then
		grep '^Installing \|Fetching' "${outdir}/validator.log" >&2 || true
		die "the validator tried to install a package that is not pinned — pin it under testdata/fhir/packages/ (never let it fetch)"
	fi

	# 4. The loaded set equals the pinned set, in both directions.
	local summary
	summary="$(sed -n 's/^ *Package Summary: //p' "${outdir}/validator.log" | tail -1)"
	[ -n "${summary}" ] || die "no 'Package Summary' in the validator log"
	printf '%s\n' "${summary}" | tr -d '[]' | tr ',' '\n' | sed 's/^ *//;s/ *$//' | grep -v '^$' | LC_ALL=C sort >"${outdir}/loaded-packages.txt"
	LC_ALL=C sort -o "${outdir}/pinned-packages.txt" "${outdir}/pinned-packages.txt"
	if ! diff -u "${outdir}/pinned-packages.txt" "${outdir}/loaded-packages.txt" >&2; then
		die "loaded packages (+) and pinned archives (-) differ: a spare pin, or a package loaded from somewhere else"
	fi

	# 5. Normalise to the ledger.
	normalise "${out_json}" "${indir}/" "${summary}" "${#inputs[@]}" >"${outdir}/findings.ledger.txt"
}

# normalise <validator-output.json> <input-root/> <package-summary> <input-count>
#
# The validator's JSON output (-output-style json) is a Bundle of one
# OperationOutcome per input, pretty-printed one member per line. POSIX awk
# (the JRE image has no jq or python) reads it by key. One ledger row per issue:
#
#   file <TAB> severity <TAB> code <TAB> message-id <TAB> expression <TAB> message
#
# plus one `file <TAB> (validated)` row per input, so an input that silently
# stops being validated is a vanished row, not an absence. Line and column are
# dropped (a regenerated fixture moves them without changing a finding), the
# input root is stripped, and `\"` in a message is unescaped; every other JSON
# escape is kept verbatim. Nothing else varies run to run: two runs, and an
# online run against the live registry, produced identical rows.
normalise() {
	local json="$1" root="$2" summary="$3" expected="$4" body
	body="$(awk -v root="${root}" '
		function after(line, key,   marker, i, s) {
			marker = "\"" key "\" : \""
			i = index(line, marker)
			if (i == 0) return ""
			s = substr(line, i + length(marker))
			sub(/",?[ \t\r]*$/, "", s)
			return s
		}
		function literal_replace(s, from, to,   out, i) {
			out = ""
			while ((i = index(s, from)) > 0) {
				out = out substr(s, 1, i - 1) to
				s = substr(s, i + length(from))
			}
			return out s
		}
		function dash(s) { return s == "" ? "-" : s }
		function flush() {
			if (have) print file "\t" sev "\t" dash(code) "\t" dash(mid) "\t" dash(expr) "\t" dash(literal_replace(text, root, ""))
			have = 0; sev = ""; code = ""; mid = ""; expr = ""; text = ""
		}
		index($0, "\"url\" : \"http://hl7.org/fhir/StructureDefinition/operationoutcome-file\"") { flush(); want_file = 1; next }
		want_file && index($0, "\"valueString\" : \"") {
			file = literal_replace(after($0, "valueString"), root, "")
			print file "\t(validated)"
			want_file = 0; next
		}
		index($0, "\"url\" : \"http://hl7.org/fhir/StructureDefinition/operationoutcome-message-id\"") { want_mid = 1; next }
		want_mid && index($0, "\"valueCode\" : \"") { pending_mid = after($0, "valueCode"); want_mid = 0; next }
		index($0, "\"severity\" : \"") {
			flush()
			have = 1; sev = after($0, "severity"); mid = pending_mid; pending_mid = ""; got_code = 0
			next
		}
		have && !got_code && index($0, "\"code\" : \"") { code = after($0, "code"); got_code = 1; next }
		have && text == "" && index($0, "\"text\" : \"") { text = literal_replace(after($0, "text"), "\\\"", "\""); next }
		have && expr == "" && index($0, "\"expression\" : [\"") {
			expr = substr($0, index($0, "[\"") + 2)
			sub(/"[],].*$/, "", expr)
			next
		}
		END { flush() }
	' "${json}" | LC_ALL=C sort)"

	local validated
	validated="$(printf '%s\n' "${body}" | grep -c $'\t(validated)$' || true)"
	[ "${validated}" -eq "${expected}" ] || die "validator output covers ${validated} of ${expected} inputs"

	cat <<EOF
# fi-fhir official FHIR validator findings ledger — Slice 5.1c-β. GENERATED; do not hand-edit.
# Gate: scripts/fhir-official-validate.sh (make fhir-official; CI test:fhir-official). Exact equality:
# a new row fails, a vanished row fails. Regenerate deliberately: make fhir-official FHIR_OFFICIAL_UPDATE=1
# validator: validator_cli.jar ${VALIDATOR_VERSION} sha256:${VALIDATOR_SHA256}
# invocation: -version ${FHIR_VERSION} -ig ${FHIR_IG} -tx n/a -txCache n/a -no-http-access
# packages: ${summary}
# columns: file, severity, code, message-id, expression, message (tab-separated); "(validated)" marks each input
EOF
	printf '%s\n' "${body}"
}

# ---- host stage ---------------------------------------------------------------

stage_inputs() {
	local indir="$1" mode="$2" bundles="${FHIR_OFFICIAL_BUNDLES_DIR:-}"
	[ -n "${bundles}" ] || die "FHIR_OFFICIAL_BUNDLES_DIR is not set (make fhir-official captures them first)"
	[ -d "${bundles}" ] || die "FHIR_OFFICIAL_BUNDLES_DIR=${bundles} does not exist"
	mkdir -p "${indir}/mapper" "${indir}/bundles" "${indir}/packages"
	cp "${MAPPER_DIR}"/*.json "${indir}/mapper/"
	local n=0 b
	for b in "${bundles}"/*.bundle.json; do
		[ -f "${b}" ] || continue
		cp "${b}" "${indir}/bundles/"
		n=$((n + 1))
	done
	[ "${n}" -gt 0 ] || die "no *.bundle.json under ${bundles}"
	cp "${PACKAGES_DIR}"/*.tgz "${PACKAGES_DIR}/SHA256SUMS" "${indir}/packages/"

	if [ "${mode}" = "negative-control" ]; then
		# Remove the top-level "name" array from a COPY of patient.json. The
		# fixture is json.MarshalIndent output, so the member spans from
		# `  "name": [` to the next `  ],` at the same two-space indent.
		local src="${indir}/mapper/patient.json" tmp="${indir}/patient.mutated"
		awk '
			/^  "name": \[/ { skip = 1; removed = 1; next }
			skip && /^  \],?[ \t\r]*$/ { skip = 0; next }
			!skip { print }
			END { if (!removed || skip) exit 3 }
		' "${src}" >"${tmp}" || die "negative control: could not remove Patient.name from patient.json (layout changed?)"
		mv "${tmp}" "${src}"
		if grep -q '^  "name":' "${src}"; then die "negative control: Patient.name still present after mutation"; fi
		log "negative control: Patient.name removed from a copy of mapper/patient.json"
	fi
}

ensure_local_jar() {
	local dir="${FHIR_OFFICIAL_CACHE_DIR:-${TMPDIR:-/tmp}/fi-fhir-official}"
	mkdir -p "${dir}"
	dir="$(cd "${dir}" && pwd -P)"
	case "${dir}/" in "$(cd "${ROOT}" && pwd -P)/"*) die "FHIR_OFFICIAL_CACHE_DIR must be outside the repository: the jar never enters the checkout" ;; esac
	local jar="${dir}/validator_cli-${VALIDATOR_VERSION}.jar"
	if [ -f "${jar}" ] && [ "$(sha256_of "${jar}")" = "${VALIDATOR_SHA256}" ]; then
		printf '%s\n' "${jar}"
		return
	fi
	log "downloading validator_cli.jar ${VALIDATOR_VERSION} into ${dir}"
	rm -f "${jar}" "${jar}.part"
	curl -fsSL --retry 3 --retry-delay 5 -o "${jar}.part" "${VALIDATOR_URL}" || die "download of ${VALIDATOR_URL} failed"
	local got
	got="$(sha256_of "${jar}.part")"
	if [ "${got}" != "${VALIDATOR_SHA256}" ]; then
		rm -f "${jar}.part"
		die "validator_cli.jar ${VALIDATOR_VERSION} sha256 is ${got}, pinned ${VALIDATOR_SHA256} — refusing to run it"
	fi
	mv "${jar}.part" "${jar}"
	printf '%s\n' "${jar}"
}

# Runs validate_inputs in a --network none container on $FHIR_OFFICIAL_DOCKER_CONTEXT.
run_in_docker() {
	local indir="$1" outdir="$2" ctx="${FHIR_OFFICIAL_DOCKER_CONTEXT}"
	local image="${FHIR_OFFICIAL_JRE_IMAGE:-${DEFAULT_JRE_IMAGE}}"
	local volume="fi-fhir-official-validator-${VALIDATOR_VERSION}"
	local d=(docker --context "${ctx}")
	local jar="/opt/validator/validator_cli.jar"

	"${d[@]}" volume create "${volume}" >/dev/null
	# The download is the only networked step and runs in its own container; it
	# verifies the digest before the jar is kept.
	"${d[@]}" run --rm -v "${volume}:/opt/validator" "${image}" sh -c "
		if [ -f ${jar} ] && echo '${VALIDATOR_SHA256}  ${jar}' | sha256sum -c - >/dev/null 2>&1; then exit 0; fi
		rm -f ${jar} ${jar}.part
		curl -fsSL --retry 3 --retry-delay 5 -o ${jar}.part '${VALIDATOR_URL}' || exit 1
		echo '${VALIDATOR_SHA256}  ${jar}.part' | sha256sum -c - || { rm -f ${jar}.part; exit 1; }
		mv ${jar}.part ${jar}
	" || die "could not provision a verified validator_cli.jar ${VALIDATOR_VERSION} in docker volume ${volume}"

	local name="fi-fhir-official-$$"
	"${d[@]}" rm -f "${name}" >/dev/null 2>&1 || true
	"${d[@]}" create --name "${name}" --network none -v "${volume}:/opt/validator:ro" -w /work \
		-e "FHIR_OFFICIAL_JAVA_OPTS=${FHIR_OFFICIAL_JAVA_OPTS:--Xmx2g}" \
		"${image}" bash /work/fhir-official-validate.sh validate-inputs /work/in /work/out "${jar}" >/dev/null
	"${d[@]}" cp "${BASH_SOURCE[0]}" "${name}:/work/fhir-official-validate.sh"
	"${d[@]}" cp "${indir}" "${name}:/work/in"
	local status=0
	"${d[@]}" start -a "${name}" || status=$?
	"${d[@]}" cp "${name}:/work/out/." "${outdir}/" 2>/dev/null || true
	"${d[@]}" rm -f "${name}" >/dev/null 2>&1 || true
	[ "${status}" -eq 0 ] || die "containerised validation failed (exit ${status}); see ${outdir}/validator.log"
}

run_mode() {
	local mode="$1"
	local work outdir jar
	work="$(mktemp -d "${TMPDIR:-/tmp}/fi-fhir-official-work.XXXXXX")"
	# shellcheck disable=SC2064
	trap "rm -rf '${work}'" EXIT
	# One output directory per mode, kept after the run for the validator log.
	outdir="${FHIR_OFFICIAL_OUT_DIR:-${TMPDIR:-/tmp}/fi-fhir-official-out}/${mode}"
	rm -rf "${outdir}"
	mkdir -p "${outdir}"
	log "output: ${outdir}"
	stage_inputs "${work}/in" "${mode}"

	if [ -n "${FHIR_OFFICIAL_DOCKER_CONTEXT:-}" ]; then
		run_in_docker "${work}/in" "${outdir}"
	else
		jar="$(ensure_local_jar)"
		validate_inputs "${work}/in" "${outdir}" "${jar}"
	fi
	local actual="${outdir}/findings.ledger.txt"
	[ -s "${actual}" ] || die "no ledger produced"
	summarise "${actual}"

	case "${mode}" in
	update)
		mkdir -p "$(dirname "${LEDGER}")"
		cp "${actual}" "${LEDGER}"
		log "wrote ${LEDGER#"${ROOT}"/}"
		;;
	gate)
		[ -f "${LEDGER}" ] || die "${LEDGER#"${ROOT}"/} is missing; run: make fhir-official FHIR_OFFICIAL_UPDATE=1"
		if ! diff -u "${LEDGER}" "${actual}"; then
			cat >&2 <<'EOF'

fhir-official: the HL7 validator's findings no longer equal the recorded ledger.

  "+" rows are NEW findings the ledger does not record — a regression, or a
      mapper change whose findings nobody has reviewed yet.
  "-" rows are findings the validator NO LONGER reports — usually a fix. Shrink
      the ledger in the same commit that fixes it.

  Either way the ledger changes only deliberately:
    FHIR_OFFICIAL_DOCKER_CONTEXT=7900xtx make fhir-official FHIR_OFFICIAL_UPDATE=1
  then commit testdata/fhir/official/findings.ledger.txt with the change that
  caused it. Docs: docs/planning/FHIR-CONFORMANCE-MATRIX.md section 5.2.
EOF
			exit 1
		fi
		log "OK: findings equal the recorded ledger exactly"
		;;
	negative-control)
		[ -f "${LEDGER}" ] || die "${LEDGER#"${ROOT}"/} is missing"
		local changed
		changed="$(diff "${LEDGER}" "${actual}" | sed -n 's/^[<>] //p' || true)"
		[ -n "${changed}" ] || die "negative control FAILED: the gate still passes with Patient.name removed from patient.json"
		local files
		files="$(printf '%s\n' "${changed}" | cut -f1 | LC_ALL=C sort -u | tr '\n' ' ' | sed 's/ $//')"
		if [ "${files}" != "mapper/patient.json" ]; then
			diff -u "${LEDGER}" "${actual}" >&2 || true
			die "negative control changed the WRONG rows: [${files}], want [mapper/patient.json] and nothing else"
		fi
		if ! printf '%s\n' "${changed}" | grep -q $'^mapper/patient\\.json\terror\t.*Patient\\.name'; then
			diff -u "${LEDGER}" "${actual}" >&2 || true
			die "negative control failed without an error naming Patient.name, so it is not evidence the gate is live"
		fi
		diff -u "${LEDGER}" "${actual}" >&2 || true
		log "negative control OK: removing Patient.name changes exactly mapper/patient.json, with an error naming Patient.name"
		;;
	esac
}

summarise() {
	awk -F '\t' '!/^#/ && $2 != "(validated)" { n[$2]++; t++ } $2 == "(validated)" { files++ }
		END { printf "fhir-official: %d inputs, %d findings (error %d, warning %d, information %d)\n", files, t, n["error"], n["warning"], n["information"] }' "$1" >&2
}

main() {
	local mode="${1:-gate}"
	case "${mode}" in
	gate | negative-control | update) run_mode "${mode}" ;;
	validate-inputs)
		[ "$#" -eq 4 ] || die "usage: $0 validate-inputs <indir> <outdir> <jar>"
		validate_inputs "$2" "$3" "$4"
		;;
	-h | --help) sed -n '2,70p' "${BASH_SOURCE[0]}" ;;
	*) die "unknown mode ${mode} (gate | negative-control | update)" ;;
	esac
}

main "$@"
