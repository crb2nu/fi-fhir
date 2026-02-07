#!/usr/bin/env bash
# docs-status.sh — Generate component status data from coverage + git history.
#
# Usage:
#   bash scripts/docs-status.sh                  # Output structured data
#   bash scripts/docs-status.sh --check-stale    # Also flag STATUS.md drift
#
# Requires: coverage.out in project root (run `make test-cover` first)

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COVERAGE_FILE="${PROJECT_ROOT}/coverage.out"
STATUS_FILE="${PROJECT_ROOT}/docs/STATUS.md"
CHECK_STALE=false

if [[ "${1:-}" == "--check-stale" ]]; then
    CHECK_STALE=true
fi

# ─── Component definitions ───────────────────────────────────────────────────
# Each line: <display_name>|<path>|<coverage_prefix>
COMPONENTS=(
    "HL7v2 Parser|internal/parser/hl7v2|internal/parser/hl7v2"
    "CSV Parser|internal/parser/csv|internal/parser/csv"
    "EDI X12 Parser|internal/parser/edi|internal/parser/edi"
    "EDI Companion Guides|internal/parser/edi/companion|internal/parser/edi/companion"
    "CDA/CCDA Parser|internal/parser/cda|internal/parser/cda"
    "FHIR Parser|internal/parser/fhir|internal/parser/fhir"
    "Events|pkg/events|pkg/events"
    "Event Sourcing|pkg/eventsourcing|pkg/eventsourcing"
    "Config|pkg/config|pkg/config"
    "Source Profiles|pkg/profile|pkg/profile"
    "Validators|pkg/validate|pkg/validate"
    "FHIR Mapper|pkg/fhir|pkg/fhir"
    "ETL Pipeline|pkg/etl|pkg/etl"
    "Storage|pkg/storage|pkg/storage"
    "Terminology (core)|pkg/terminology|pkg/terminology"
    "Terminology DB|pkg/terminology/db|pkg/terminology/db"
    "Terminology Upload|pkg/terminology/upload|pkg/terminology/upload"
    "Terminology Suggest|pkg/terminology/suggest|pkg/terminology/suggest"
    "Terminology Semantic|pkg/terminology/semantic|pkg/terminology/semantic"
    "Terminology Index|pkg/terminology/index|pkg/terminology/index"
    "Patient Matching|pkg/matching|pkg/matching"
    "LLM Client|pkg/llm|pkg/llm"
    "LLM Copilot|pkg/llm/copilot|pkg/llm/copilot"
    "Workflow Engine|internal/workflow|internal/workflow"
    "GraphQL API|internal/api/graphql|internal/api/graphql"
    "FHIR Subscriptions|internal/fhir/subscription|internal/fhir"
    "Terminology Autoroute|internal/terminology/autoroute|internal/terminology/autoroute"
    "Terminology Workflow|internal/terminology/workflow|internal/terminology/workflow"
    "LLM Explain|internal/llm/explain|internal/llm/explain"
    "LLM Extract|internal/llm/extract|internal/llm/extract"
    "LLM Quality|internal/llm/quality|internal/llm/quality"
    "CLI|cmd/fi-fhir|cmd/fi-fhir"
)

# ─── Check coverage file ─────────────────────────────────────────────────────
if [[ ! -f "${COVERAGE_FILE}" ]]; then
    echo "ERROR: ${COVERAGE_FILE} not found. Run 'make test-cover' first." >&2
    exit 1
fi

# ─── Parse coverage ──────────────────────────────────────────────────────────
# Build an associative array of package → average coverage %
declare -A PKG_COVERAGE
declare -A PKG_FUNCS

while IFS= read -r line; do
    # Skip header and total
    [[ "${line}" == mode:* ]] && continue
    [[ "${line}" == total:* ]] && continue
    [[ -z "${line}" ]] && continue

    # Extract file path and coverage from go tool cover -func output
    # Format: <full_path>:<line>:\t<func>\t<coverage%>
    file_path="${line%%	*}"
    coverage_pct="${line##*	}"
    coverage_pct="${coverage_pct%%%}"

    # Strip module prefix to get relative path
    rel_path="${file_path#*fi-fhir/}"
    rel_path="${rel_path%:*}"  # Remove :line suffix

    # Determine component by matching coverage prefix
    for comp_def in "${COMPONENTS[@]}"; do
        IFS='|' read -r _name comp_path cov_prefix <<< "${comp_def}"
        if [[ "${rel_path}" == ${cov_prefix}/* ]]; then
            # Accumulate for averaging
            current_sum="${PKG_COVERAGE[${comp_path}]:-0}"
            current_count="${PKG_FUNCS[${comp_path}]:-0}"
            PKG_COVERAGE[${comp_path}]="$(echo "${current_sum} + ${coverage_pct}" | bc 2>/dev/null || echo "${current_sum}")"
            PKG_FUNCS[${comp_path}]="$((current_count + 1))"
            break
        fi
    done
done < <(go tool cover -func="${COVERAGE_FILE}" 2>/dev/null | grep -v "^total:")

# ─── Output structured data ──────────────────────────────────────────────────
echo "# fi-fhir Component Status Data"
echo "# Generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "#"
printf "%-30s | %-35s | %8s | %12s | %5s | %5s\n" \
    "Component" "Path" "Coverage" "Last Updated" "Files" "Tests"
printf "%-30s-+-%-35s-+-%8s-+-%12s-+-%5s-+-%5s\n" \
    "------------------------------" "-----------------------------------" "--------" "------------" "-----" "-----"

stale_count=0

for comp_def in "${COMPONENTS[@]}"; do
    IFS='|' read -r name comp_path cov_prefix <<< "${comp_def}"

    # Coverage
    total="${PKG_COVERAGE[${comp_path}]:-0}"
    count="${PKG_FUNCS[${comp_path}]:-0}"
    if [[ "${count}" -gt 0 ]]; then
        avg="$(echo "scale=1; ${total} / ${count}" | bc 2>/dev/null || echo "0.0")"
        coverage="${avg}%"
    else
        coverage="—"
    fi

    # Git last updated
    full_path="${PROJECT_ROOT}/${comp_path}"
    if [[ -d "${full_path}" || -f "${full_path}" ]]; then
        last_updated="$(git -C "${PROJECT_ROOT}" log -1 --format='%ci' -- "${comp_path}" 2>/dev/null | cut -d' ' -f1)"
        [[ -z "${last_updated}" ]] && last_updated="N/A"
    else
        last_updated="N/A"
    fi

    # File and test counts
    if [[ -d "${full_path}" ]]; then
        file_count="$(find "${full_path}" -name '*.go' -not -name '*_test.go' 2>/dev/null | wc -l | tr -d ' ')"
        test_count="$(find "${full_path}" -name '*_test.go' 2>/dev/null | wc -l | tr -d ' ')"
    else
        file_count="0"
        test_count="0"
    fi

    printf "%-30s | %-35s | %8s | %12s | %5s | %5s\n" \
        "${name}" "${comp_path}" "${coverage}" "${last_updated}" "${file_count}" "${test_count}"

    # Stale check: compare with STATUS.md
    if [[ "${CHECK_STALE}" == true ]] && [[ -f "${STATUS_FILE}" ]]; then
        # Use backtick-delimited path for precise matching (e.g., `pkg/events/`)
        escaped_path="\`${comp_path}/\`"
        status_line="$(grep -F "${escaped_path}" "${STATUS_FILE}" 2>/dev/null | head -1 || true)"
        if [[ -z "${status_line}" ]]; then
            # Try without trailing slash
            escaped_path="\`${comp_path}\`"
            status_line="$(grep -F "${escaped_path}" "${STATUS_FILE}" 2>/dev/null | head -1 || true)"
        fi
        status_date="$(echo "${status_line}" | grep -oE '[0-9]{4}-[0-9]{2}-[0-9]{2}' | tail -1 || true)"
        if [[ -n "${status_date}" ]] && [[ -n "${last_updated}" ]] && [[ "${last_updated}" != "N/A" ]]; then
            if [[ "${status_date}" != "${last_updated}" ]]; then
                echo "  ⚠ STALE: STATUS.md says ${status_date}, git says ${last_updated}" >&2
                stale_count=$((stale_count + 1))
            fi
        fi
    fi
done

# ─── UI coverage (separate — uses Vitest) ────────────────────────────────────
echo ""
echo "# UI Coverage (Vitest)"
ui_coverage_dir="${PROJECT_ROOT}/ui/coverage"
if [[ -d "${ui_coverage_dir}" ]]; then
    echo "# UI coverage directory found at ${ui_coverage_dir}"
    # Look for coverage-summary.json from c8/istanbul
    if [[ -f "${ui_coverage_dir}/coverage-summary.json" ]]; then
        echo "# Coverage summary available — parse with jq if needed"
    else
        echo "# No coverage-summary.json found; run 'cd ui && npm test -- --coverage'"
    fi
else
    echo "# No UI coverage data found. Run: cd ui && npm test -- --coverage"
fi

# ─── Summary ──────────────────────────────────────────────────────────────────
echo ""
echo "# Total Go coverage: $(go tool cover -func="${COVERAGE_FILE}" 2>/dev/null | grep "^total:" | awk '{print $NF}')"

if [[ "${CHECK_STALE}" == true ]]; then
    if [[ ${stale_count} -gt 0 ]]; then
        echo ""
        echo "# ⚠ ${stale_count} component(s) have stale STATUS.md entries" >&2
    else
        echo ""
        echo "# ✅ All STATUS.md entries are up to date"
    fi
fi
