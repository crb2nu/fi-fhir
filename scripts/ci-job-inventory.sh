#!/usr/bin/env bash
#
# ci-job-inventory.sh — a machine-readable inventory of every CI job this
# repository defines locally.
#
# Sprint 5 splits `.gitlab-ci.yml` into per-lane `include: local:` files so six
# lanes stop appending to one 2700-line file. The risk of that move is not a
# merge conflict — it is a job that is silently dropped, or that lands in the
# wrong stage and is never noticed because the pipeline is green. This script
# makes the job list a text file, so the split's acceptance is a diff rather
# than a reviewer's eye, and so a later lane cannot delete a required proof
# without the diff going red.
#
# Output is one line per job, sorted by name:
#
#     name<TAB>stage<TAB>allow_failure
#
# It is deliberately position-independent: moving a job from `.gitlab-ci.yml`
# into `ci/<name>.yml` must not change a byte of it. Use --with-source when you
# want to see where a job is actually declared.
#
# Scope and honest limits:
#   * Only locally-declared jobs are inventoried: `.gitlab-ci.yml` plus every
#     `- local: <path>` include. Jobs that arrive from `- project:` includes
#     are not parsed and do not appear.
#   * `stage` and `allow_failure` are resolved through `extends:` within the
#     local files. A job whose only source is a remote hidden job reports `-`
#     for a value it does not declare itself (`radar-scan` is the one today).
#   * `rules:` are NOT fingerprinted. Rule equivalence across an
#     alias-to-`extends:` conversion is a behavioural property; prove it by
#     diffing the rendered job list of two pipelines, not with this script.
#
# Usage:
#   scripts/ci-job-inventory.sh                 # print the inventory
#   scripts/ci-job-inventory.sh --with-source   # append <TAB>file:line
#   scripts/ci-job-inventory.sh --check         # diff against ci/job-inventory.txt
#   scripts/ci-job-inventory.sh --write         # regenerate ci/job-inventory.txt

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GOLDEN="${ROOT}/ci/job-inventory.txt"
ROOT_CI="${ROOT}/.gitlab-ci.yml"

mode="print"
with_source="0"
for arg in "$@"; do
	case "${arg}" in
	--check) mode="check" ;;
	--write) mode="write" ;;
	--with-source) with_source="1" ;;
	-h | --help)
		sed -n '2,42p' "${BASH_SOURCE[0]}"
		exit 0
		;;
	*)
		echo "ci-job-inventory.sh: unknown argument ${arg}" >&2
		exit 2
		;;
	esac
done

if [ ! -f "${ROOT_CI}" ]; then
	echo "ci-job-inventory.sh: ${ROOT_CI} not found" >&2
	exit 1
fi

# Collect the root file plus every `- local:` include, in declaration order.
collect_files() {
	printf '%s\n' "${ROOT_CI}"
	awk '
		/^include:/ { inside = 1; next }
		inside && /^[^[:space:]#]/ { inside = 0 }
		inside && match($0, /-[[:space:]]+local:[[:space:]]*/) {
			path = substr($0, RSTART + RLENGTH)
			gsub(/^["'"'"']|["'"'"']$/, "", path)
			gsub(/^\/+/, "", path)
			sub(/[[:space:]]+$/, "", path)
			if (path != "") print path
		}
	' "${ROOT_CI}" | while IFS= read -r rel; do
		if [ -f "${ROOT}/${rel}" ]; then
			printf '%s\n' "${ROOT}/${rel}"
		else
			echo "ci-job-inventory.sh: include local:${rel} does not exist" >&2
			exit 1
		fi
	done
}

inventory() {
	# shellcheck disable=SC2016
	awk -v root="${ROOT}/" -v with_source="${with_source}" '
		function flush(  ) {
			if (name == "") return
			decl_stage[name] = stage
			decl_allow[name] = allow
			decl_extends[name] = ext
			if (substr(name, 1, 1) != ".") {
				jobs[++njobs] = name
				where[name] = src
			}
			name = ""; stage = ""; allow = ""; ext = ""; src = ""
		}
		FNR == 1 { file = FILENAME; sub(root, "", file); flush() }
		# A top-level key: column 0, not a comment, not a list item.
		/^[A-Za-z0-9_.][A-Za-z0-9_.:-]*:([[:space:]]|$)/ {
			flush()
			key = $0
			sub(/:([[:space:]].*)?$/, "", key)
			if (key == "include" || key == "stages" || key == "variables" ||
			    key == "default" || key == "workflow" || key == "image" ||
			    key == "before_script" || key == "after_script" || key == "cache" ||
			    key == "services" || key == "types") { next }
			name = key
			src = file ":" FNR
			next
		}
		name == "" { next }
		# Exactly two spaces of indent: a job-level keyword, never a nested one.
		/^  stage:[[:space:]]/ { stage = trim(substr($0, 9)); next }
		/^  allow_failure:[[:space:]]/ { allow = trim(substr($0, 17)); next }
		/^  extends:[[:space:]]/ { ext = ext " " trim(substr($0, 11)); next }
		/^  extends:[[:space:]]*$/ { in_ext = 1; next }
		in_ext && /^    -[[:space:]]/ { ext = ext " " trim(substr($0, 6)); next }
		in_ext { in_ext = 0 }
		function trim(s) {
			sub(/[[:space:]]+#.*$/, "", s)
			gsub(/^[[:space:]]+|[[:space:]]+$/, "", s)
			gsub(/^["'"'"']|["'"'"']$/, "", s)
			return s
		}
		END {
			flush()
			for (i = 1; i <= njobs; i++) {
				job = jobs[i]
				s = resolve(job, "stage")
				a = resolve(job, "allow_failure")
				if (s == "") s = "-"
				if (a == "") a = "false"   # GitLab default
				line = job "\t" s "\t" a
				if (with_source == "1") line = line "\t" where[job]
				print line
			}
		}
		function resolve(job, field,   n, i, parts, v, depth) {
			v = (field == "stage") ? decl_stage[job] : decl_allow[job]
			if (v != "") return v
			for (depth = 0; depth < 4; depth++) {
				n = split(decl_extends[job], parts, " ")
				for (i = 1; i <= n; i++) {
					if (parts[i] == "") continue
					v = (field == "stage") ? decl_stage[parts[i]] : decl_allow[parts[i]]
					if (v != "") return v
					job = parts[i]
				}
				if (n == 0) break
			}
			return ""
		}
	' "$@" | LC_ALL=C sort
}

# shellcheck disable=SC2046
generated="$(inventory $(collect_files))"

case "${mode}" in
print)
	printf '%s\n' "${generated}"
	;;
write)
	mkdir -p "$(dirname "${GOLDEN}")"
	printf '%s\n' "${generated}" >"${GOLDEN}"
	echo "ci-job-inventory.sh: wrote $(printf '%s\n' "${generated}" | wc -l | tr -d ' ') jobs to ci/job-inventory.txt"
	;;
check)
	if [ ! -f "${GOLDEN}" ]; then
		echo "ci-job-inventory.sh: ${GOLDEN} is missing; run scripts/ci-job-inventory.sh --write" >&2
		exit 1
	fi
	if ! diff -u "${GOLDEN}" <(printf '%s\n' "${generated}"); then
		cat >&2 <<'EOF'

ci-job-inventory.sh: the CI job inventory changed and ci/job-inventory.txt was not updated.

  A line removed here is a required proof that no longer runs. A changed stage
  or allow_failure is a gate that stopped gating. Neither is visible in a green
  pipeline, which is why this is a blocking check rather than a comment.

  If the change is intended:  scripts/ci-job-inventory.sh --write
  and include ci/job-inventory.txt in the same commit, so a reviewer sees the
  job list change alongside the reason for it.

  Convention (AGENTS.md, "CI job layout"): a new required proof adds
  ci/test-<name>.yml plus one `- local:` line in .gitlab-ci.yml. Never append a
  job to the root file.
EOF
		exit 1
	fi
	echo "ci-job-inventory.sh: $(wc -l <"${GOLDEN}" | tr -d ' ') jobs match ci/job-inventory.txt"
	;;
esac
