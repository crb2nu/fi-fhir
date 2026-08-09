#!/usr/bin/env bash
# performance-report.sh — render benchmark output as an archivable report.
#
# Usage:
#   scripts/performance-report.sh benchmark-profile.txt performance-report.json
#
# The report is the artifact a future 1.0 support claim would cite for budgets 1
# and 2, so its job is to make an uncitable number obviously uncitable. Every
# measurement carries the hardware it ran on, the replica count it ran against,
# and a `certified` flag that is false unless the run happened on the pinned
# runner. A reader who finds `"certified": false` should read the value as
# "someone measured this once, somewhere" and nothing more.
#
# SCHEMA (version 1)
#
#   schema_version   int     1. Bump on any incompatible change.
#   generated_at     string  RFC3339 UTC.
#   certified        bool    True only when the run happened on a runner tagged
#                            fi-fhir-perf AND the reference profile was applied.
#                            Nothing else may set it. A shared-pool run is
#                            always false, however good the numbers look.
#   certification    object
#     runner_tag     string  The tag the job actually ran on, or "" if untagged.
#     reason         string  Why certified is false, empty when it is true.
#   environment      object
#     cpu            string  The "cpu:" line Go printed. The single most
#                            important field in the file: the k3s pool spans
#                            5.3x, so a duration without it means nothing.
#     goos, goarch   string
#     cpu_limit      string  KUBERNETES_CPU_LIMIT for the job.
#     memory_limit   string  KUBERNETES_MEMORY_LIMIT for the job.
#     replicas       int     Application replicas the measurement ran against.
#                            1 for an in-process harness; the reference profile
#                            is 2, and budget 2 is not measurable at 2 until
#                            slice 4.4e makes the MLLP bucket per-deployment.
#     postgres       string  Server version string.
#   revision         object
#     commit         string  CI_COMMIT_SHA.
#     ref            string  CI_COMMIT_REF_NAME.
#     pipeline_url   string  CI_PIPELINE_URL.
#   benchmarks       array of objects
#     name           string
#     iterations     int
#     ns_per_op      number
#     allocs_per_op  int
#     bytes_per_op   int
#     events_per_sec number  Omitted when the benchmark reports none.
#   budgets          array of objects
#     id             int     1..7, matching SUPPORTED-1.0.md's budget table.
#     name           string
#     target         string  The budget as written, in its own units.
#     measured       string  The measured value, or "not measured".
#     status         string  certified | harnessed | blocked | not_measured
#     blocked_on     string  Empty unless status is blocked.
#
# The budgets array is intentionally not derived from the benchmark array. A
# benchmark measures a function; a budget is a claim about a product. Keeping
# them separate stops a green benchmark from being silently promoted into a met
# budget, which is the failure this whole slice exists to prevent.

set -euo pipefail

input="${1:?usage: performance-report.sh <benchmark.txt> <report.json>}"
output="${2:?usage: performance-report.sh <benchmark.txt> <report.json>}"

if [ ! -f "$input" ]; then
  echo "performance-report: $input does not exist" >&2
  exit 1
fi

cpu="$(sed -n 's/^cpu: *//p' "$input" | head -1)"
goos="$(sed -n 's/^goos: *//p' "$input" | head -1)"
goarch="$(sed -n 's/^goarch: *//p' "$input" | head -1)"

# certified is false unless this ran on the pinned runner. The runner tag is not
# exposed to the job directly, so the project variable that gates the job is the
# proxy — and it is set only alongside a registered fi-fhir-perf runner.
runner_tag=""
certified="false"
reason="not run on a pinned fi-fhir-perf runner"
if [ "${FI_FHIR_PERF_RUNNER:-}" = "1" ]; then
  runner_tag="fi-fhir-perf"
  certified="true"
  reason=""
fi

# The harness is in-process: it drives ingress.Service.Submit directly rather
# than a deployed replica set, so the replica count is 1 regardless of what any
# chart says. Recording it honestly is what keeps budget 2 from looking met.
replicas=1

benchmarks="$(awk '
  /^Benchmark/ {
    name = $1
    sub(/-[0-9]+$/, "", name)
    iterations = $2
    ns = ""; allocs = ""; bytes = ""; eps = ""
    for (i = 3; i <= NF; i++) {
      if ($(i) == "ns/op")      ns = $(i-1)
      if ($(i) == "allocs/op")  allocs = $(i-1)
      if ($(i) == "B/op")       bytes = $(i-1)
      if ($(i) == "events/sec") eps = $(i-1)
    }
    printf "%s{\"name\":\"%s\",\"iterations\":%s", (count++ ? "," : ""), name, iterations
    if (ns     != "") printf ",\"ns_per_op\":%s", ns
    if (allocs != "") printf ",\"allocs_per_op\":%s", allocs
    if (bytes  != "") printf ",\"bytes_per_op\":%s", bytes
    if (eps    != "") printf ",\"events_per_sec\":%s", eps
    printf "}"
  }
' "$input")"

cat > "$output" <<JSON
{
  "schema_version": 1,
  "generated_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "certified": ${certified},
  "certification": {
    "runner_tag": "${runner_tag}",
    "reason": "${reason}"
  },
  "environment": {
    "cpu": "${cpu}",
    "goos": "${goos}",
    "goarch": "${goarch}",
    "cpu_limit": "${KUBERNETES_CPU_LIMIT:-}",
    "memory_limit": "${KUBERNETES_MEMORY_LIMIT:-}",
    "replicas": ${replicas},
    "postgres": "${POSTGRES_VERSION:-16}"
  },
  "revision": {
    "commit": "${CI_COMMIT_SHA:-}",
    "ref": "${CI_COMMIT_REF_NAME:-}",
    "pipeline_url": "${CI_PIPELINE_URL:-}"
  },
  "benchmarks": [${benchmarks}],
  "budgets": [
    {
      "id": 1,
      "name": "authenticated MLLP and HTTP durable-accept latency",
      "target": "p95 <= 250 ms, p99 <= 500 ms",
      "measured": "see benchmarks[].ns_per_op",
      "status": "harnessed",
      "blocked_on": ""
    },
    {
      "id": 2,
      "name": "one-hour steady-state throughput on the reference profile",
      "target": "sustained declared msg/s for one hour",
      "measured": "not measured",
      "status": "blocked",
      "blocked_on": "slice 4.4e (per-deployment MLLP token bucket); this harness is single-process, so a two-replica reference-profile run would admit twice the declared rate"
    },
    {
      "id": 3,
      "name": "1-GiB batch import peak memory above idle",
      "target": "bounded peak RSS above idle",
      "measured": "not measured",
      "status": "harnessed",
      "blocked_on": "perf.HeapSampler reports HeapAlloc, not RSS; a true RSS figure must be read from the cgroup by this job"
    }
  ]
}
JSON

echo "performance-report: wrote $output (certified=${certified})"
