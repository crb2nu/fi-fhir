#!/usr/bin/env bash
# validate-docs.sh — Check documentation consistency.
#
# Validates:
# 1. Every component directory has an entry in docs/STATUS.md
# 2. CHANGELOG.md has an [Unreleased] section
# 3. AGENTS.md doesn't contain known stale patterns
#
# Exit code: 0 if all checks pass, 1 if any fail.

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STATUS_FILE="${PROJECT_ROOT}/docs/STATUS.md"
CHANGELOG_FILE="${PROJECT_ROOT}/CHANGELOG.md"
AGENTS_FILE="${PROJECT_ROOT}/AGENTS.md"

errors=0
warnings=0

# ─── Helper ───────────────────────────────────────────────────────────────────
fail() {
    echo "❌ FAIL: $1" >&2
    errors=$((errors + 1))
}

warn() {
    echo "⚠  WARN: $1" >&2
    warnings=$((warnings + 1))
}

pass() {
    echo "✅ PASS: $1"
}

# ─── Check 1: STATUS.md exists ───────────────────────────────────────────────
echo ""
echo "=== Check 1: STATUS.md existence and component coverage ==="

if [[ ! -f "${STATUS_FILE}" ]]; then
    fail "docs/STATUS.md not found"
else
    pass "docs/STATUS.md exists"

    # Check that key directories have entries in STATUS.md
    missing_components=()

    # Parser directories
    for dir in "${PROJECT_ROOT}"/internal/parser/*/; do
        [[ ! -d "${dir}" ]] && continue
        rel_path="${dir#${PROJECT_ROOT}/}"
        rel_path="${rel_path%/}"
        if ! grep -qF "${rel_path}" "${STATUS_FILE}" 2>/dev/null; then
            # Check without trailing slash variations
            base_name="$(basename "${rel_path}")"
            if ! grep -qF "internal/parser/${base_name}" "${STATUS_FILE}" 2>/dev/null; then
                missing_components+=("${rel_path}")
            fi
        fi
    done

    # pkg directories (top-level only, not sub-packages)
    for dir in "${PROJECT_ROOT}"/pkg/*/; do
        [[ ! -d "${dir}" ]] && continue
        rel_path="${dir#${PROJECT_ROOT}/}"
        rel_path="${rel_path%/}"
        if ! grep -qF "${rel_path}" "${STATUS_FILE}" 2>/dev/null; then
            missing_components+=("${rel_path}")
        fi
    done

    # Internal service directories
    for dir in "${PROJECT_ROOT}"/internal/api/ "${PROJECT_ROOT}"/internal/fhir/ "${PROJECT_ROOT}"/internal/workflow/ "${PROJECT_ROOT}"/internal/terminology/ "${PROJECT_ROOT}"/internal/llm/; do
        [[ ! -d "${dir}" ]] && continue
        rel_path="${dir#${PROJECT_ROOT}/}"
        rel_path="${rel_path%/}"
        if ! grep -qF "${rel_path}" "${STATUS_FILE}" 2>/dev/null; then
            # Also check for sub-paths (e.g., internal/api/graphql)
            found=false
            for subdir in "${dir}"*/; do
                [[ ! -d "${subdir}" ]] && continue
                sub_rel="${subdir#${PROJECT_ROOT}/}"
                sub_rel="${sub_rel%/}"
                if grep -qF "${sub_rel}" "${STATUS_FILE}" 2>/dev/null; then
                    found=true
                    break
                fi
            done
            if [[ "${found}" == false ]]; then
                missing_components+=("${rel_path}")
            fi
        fi
    done

    if [[ ${#missing_components[@]} -eq 0 ]]; then
        pass "All component directories have STATUS.md entries"
    else
        for comp in "${missing_components[@]}"; do
            fail "Missing STATUS.md entry for: ${comp}"
        done
    fi
fi

# ─── Check 2: CHANGELOG.md has [Unreleased] section ──────────────────────────
echo ""
echo "=== Check 2: CHANGELOG.md [Unreleased] section ==="

if [[ ! -f "${CHANGELOG_FILE}" ]]; then
    fail "CHANGELOG.md not found"
else
    if grep -q '## \[Unreleased\]' "${CHANGELOG_FILE}" 2>/dev/null; then
        pass "CHANGELOG.md has [Unreleased] section"

        # Check that [Unreleased] has content (not just the header)
        unreleased_lines="$(sed -n '/## \[Unreleased\]/,/## \[/p' "${CHANGELOG_FILE}" | wc -l | tr -d ' \n')"
        unreleased_lines="${unreleased_lines:-0}"
        if [[ ${unreleased_lines} -lt 5 ]]; then
            warn "CHANGELOG.md [Unreleased] section has very few entries (${unreleased_lines} lines)"
        else
            pass "CHANGELOG.md [Unreleased] section has content (${unreleased_lines} lines)"
        fi
    else
        fail "CHANGELOG.md missing [Unreleased] section"
    fi
fi

# ─── Check 3: AGENTS.md stale patterns ───────────────────────────────────────
echo ""
echo "=== Check 3: AGENTS.md stale pattern check ==="

if [[ ! -f "${AGENTS_FILE}" ]]; then
    fail "AGENTS.md not found"
else
    stale_patterns=(
        "Remaining Backlog.*Patient matching"
        "Remaining Backlog.*EDI companion"
        "Remaining Backlog.*GraphQL mutations"
        "Remaining Backlog.*WebSocket"
        "Phase 6.*Complete"
        "Phase 7.*Complete"
    )

    stale_found=false
    for pattern in "${stale_patterns[@]}"; do
        if grep -qiE "${pattern}" "${AGENTS_FILE}" 2>/dev/null; then
            fail "AGENTS.md contains stale pattern: '${pattern}'"
            stale_found=true
        fi
    done

    if [[ "${stale_found}" == false ]]; then
        pass "AGENTS.md has no known stale patterns"
    fi

    # Check for duplicate "Completed" sections
    completed_count="$(grep -c '^\*\*Completed\*\*' "${AGENTS_FILE}" 2>/dev/null || echo 0)"
    completed_count="${completed_count//[^0-9]/}"
    completed_count="${completed_count:-0}"
    if [[ ${completed_count} -gt 1 ]]; then
        fail "AGENTS.md has ${completed_count} duplicate 'Completed' sections (should have at most 1)"
    else
        pass "AGENTS.md has no duplicate 'Completed' sections"
    fi

    # Check roadmap section isn't excessively long
    roadmap_lines="$(sed -n '/## Roadmap Context/,/## /p' "${AGENTS_FILE}" | wc -l | tr -d ' \n')"
    roadmap_lines="${roadmap_lines:-0}"
    if [[ ${roadmap_lines} -gt 40 ]]; then
        warn "AGENTS.md Roadmap Context section is ${roadmap_lines} lines (target: <30)"
    else
        pass "AGENTS.md Roadmap Context section is concise (${roadmap_lines} lines)"
    fi
fi

# ─── Check 4: Documentation conventions file exists ──────────────────────────
echo ""
echo "=== Check 4: Documentation conventions ==="

if [[ -f "${PROJECT_ROOT}/docs/DOCUMENTATION-CONVENTIONS.md" ]]; then
    pass "docs/DOCUMENTATION-CONVENTIONS.md exists"
else
    warn "docs/DOCUMENTATION-CONVENTIONS.md not found (recommended)"
fi

# ─── Summary ──────────────────────────────────────────────────────────────────
echo ""
echo "════════════════════════════════════════════════════════════════"
if [[ ${errors} -eq 0 ]]; then
    echo "✅ Documentation validation passed! (${warnings} warning(s))"
    exit 0
else
    echo "❌ Documentation validation failed: ${errors} error(s), ${warnings} warning(s)"
    exit 1
fi
