#!/usr/bin/env bash
# performance-report.sh — render benchmark output as an archivable report.
#
# Usage:
#   scripts/performance-report.sh benchmark-profile.txt performance-report.json
#
# The report is the artifact a 1.0 support claim cites for budget 1, so its job
# is to make an uncitable number obviously uncitable. Every measurement carries
# the hardware it ran on, the runner it can be attributed to, the replica count
# it ran against, and a `certified` flag that is false unless the run happened
# on the pinned runner, was not a negative control, AND met every budget this
# job evaluates. A reader who finds `"certified": false` should read the values
# as "someone measured this once, somewhere" and nothing more.
#
# SCHEMA (version 2)
#
#   schema_version   int     2. Bump on any incompatible change.
#   generated_at     string  RFC3339 UTC.
#   certified        bool    True only when all three hold: the run happened on
#                            a runner tagged fi-fhir-perf; it was not a
#                            negative-control build; and budget 1 — the only
#                            budget this job evaluates — was met. It says
#                            nothing about budgets 2 and 3, whose entries carry
#                            their own status. A shared-pool run is always
#                            false, however good the numbers look.
#   certification    object
#     runner_tag     string  "fi-fhir-perf" if the runner that executed the job
#                            reported that tag (CI_RUNNER_TAGS), else "".
#     runner_id      string  CI_RUNNER_ID, or "" outside CI.
#     reason         string  Every reason certified is false, "; "-joined.
#                            Empty when it is true.
#   negative_control object
#     ran            bool    True when this is a negative-control build: the
#                            benchmark output carries the NEGATIVE-CONTROL
#                            marker, or FI_FHIR_PERF_BUILD_TAGS names
#                            perfregress. Such a run is never certified.
#     failed_budget  int|null When ran is true, the budget the injected
#                            regression failed. null with ran=true means the
#                            harness did not notice a 300 ms regression, so no
#                            number from it may be cited; the script exits 3.
#                            Always null when ran is false.
#   environment      object
#     cpu            string  The "cpu:" line Go printed. The k3s pool spans
#                            5.3x, so a duration without it means nothing.
#     goos, goarch   string
#     cpu_limit      string  KUBERNETES_CPU_LIMIT for the job.
#     memory_limit   string  KUBERNETES_MEMORY_LIMIT for the job.
#     replicas       int     Application replicas the measurement ran against.
#                            1 for this in-process harness; the reference
#                            profile is 2.
#     postgres       string  Server version string.
#   revision         object
#     commit         string  CI_COMMIT_SHA.
#     ref            string  CI_COMMIT_REF_NAME.
#     pipeline_url   string  CI_PIPELINE_URL.
#   benchmarks       array of objects
#     name           string
#     iterations     int
#     ns_per_op      number  A mean. Never evaluated against a budget.
#     allocs_per_op  int
#     bytes_per_op   int
#     events_per_sec number  Omitted when the benchmark reports none.
#     p50_ms, p95_ms, p99_ms
#                    number  Nearest-rank per-accept latency. Omitted when the
#                            benchmark reports none.
#   budgets          array of objects
#     id             int     1..3, matching SUPPORTED-1.0.md's budget table.
#     name           string
#     target         string  The budget as written, in its own units.
#     measured       string  The measured value, or "not measured".
#     status         string  certified | failed | harnessed | not_measured
#                              certified     met, on the pinned runner, not a
#                                            negative control
#                              failed        measured and outside the target,
#                                            wherever it ran
#                              harnessed     the path is measured but this run
#                                            cannot certify the budget
#                              not_measured  this job produces no value for it
#     gap            string  What certification still needs. Empty when
#                            certified.
#
# Version 1 set `certified` from the FI_FHIR_PERF_RUNNER project variable alone
# and evaluated no measurement, so a build that slept 300 ms per accept
# certified; budget 1 was reported as a mean although it is written as p95/p99.
# Version 2 is the fix. `blocked_on` is gone: nothing in budgets 1-3 is blocked
# on another slice any more, and `gap` says what each one does need.
#
# The budgets array is intentionally not a copy of the benchmark array. A
# benchmark measures a function; a budget is a claim about a product. Budget 1
# is derived from named percentiles of named benchmarks, by rule, below.

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

reasons=()

# The runner is identified by what the runner itself reported to the job, not
# by the project variable that makes the job visible: the variable says a
# pinned runner exists, not that this job ran on it.
runner_tag=""
pinned="false"
case "${CI_RUNNER_TAGS:-}" in
  *fi-fhir-perf*)
    runner_tag="fi-fhir-perf"
    pinned="true"
    ;;
esac
runner_id=""
if [[ "${CI_RUNNER_ID:-}" =~ ^[0-9]+$ ]]; then
  runner_id="$CI_RUNNER_ID"
fi
if [ "$pinned" != "true" ]; then
  reasons+=("not run on a pinned fi-fhir-perf runner")
fi

# A negative-control build is detected from the artifact first: the benchmark
# logs a marker when it was compiled with the injected regression, so the report
# cannot be fooled by a variable that did or did not reach the build.
negative_control="false"
if grep -q 'NEGATIVE-CONTROL build' "$input" || [[ ",${FI_FHIR_PERF_BUILD_TAGS:-}," == *,perfregress,* ]]; then
  negative_control="true"
  reasons+=("negative-control build (perfregress): its numbers describe an injected regression, not the product")
fi

# The harness is in-process: it drives ingress.Service.Submit and
# mllp.Service.Submit directly rather than a deployed replica set, so the
# replica count is 1 regardless of what any chart says.
replicas=1

benchmarks="$(awk '
  /^Benchmark/ {
    name = $1
    sub(/-[0-9]+$/, "", name)
    iterations = $2
    ns = ""; allocs = ""; bytes = ""; eps = ""; p50 = ""; p95 = ""; p99 = ""
    for (i = 3; i <= NF; i++) {
      if ($(i) == "ns/op")      ns = $(i-1)
      if ($(i) == "allocs/op")  allocs = $(i-1)
      if ($(i) == "B/op")       bytes = $(i-1)
      if ($(i) == "events/sec") eps = $(i-1)
      if ($(i) == "p50-ms")     p50 = $(i-1)
      if ($(i) == "p95-ms")     p95 = $(i-1)
      if ($(i) == "p99-ms")     p99 = $(i-1)
    }
    printf "%s{\"name\":\"%s\",\"iterations\":%s", (count++ ? "," : ""), name, iterations
    if (ns     != "") printf ",\"ns_per_op\":%s", ns
    if (allocs != "") printf ",\"allocs_per_op\":%s", allocs
    if (bytes  != "") printf ",\"bytes_per_op\":%s", bytes
    if (eps    != "") printf ",\"events_per_sec\":%s", eps
    if (p50    != "") printf ",\"p50_ms\":%s", p50
    if (p95    != "") printf ",\"p95_ms\":%s", p95
    if (p99    != "") printf ",\"p99_ms\":%s", p99
    printf "}"
  }
' "$input")"

# Budget 1: p95 <= 250 ms and p99 <= 500 ms on both authenticated entry points,
# serial and parallel. All four benchmarks must report both percentiles; a
# missing one makes the budget not_measured rather than letting three
# benchmarks stand in for four.
b1_p95_limit=250
b1_p99_limit=500
b1_eval="$(awk -v l95="$b1_p95_limit" -v l99="$b1_p99_limit" '
  BEGIN {
    nwant = split("IngressSubmit IngressSubmitParallel MLLPSubmit MLLPSubmitParallel", want, " ")
  }
  /^BenchmarkDurableAccept_/ {
    name = $1
    sub(/-[0-9]+$/, "", name)
    p95 = ""; p99 = ""
    for (i = 3; i <= NF; i++) {
      if ($(i) == "p95-ms") p95 = $(i-1)
      if ($(i) == "p99-ms") p99 = $(i-1)
    }
    if (p95 == "" || p99 == "") next
    seen[name] = 1
    if (w95 == "" || p95 + 0 > w95 + 0) { w95 = p95; w95n = name }
    if (w99 == "" || p99 + 0 > w99 + 0) { w99 = p99; w99n = name }
    if (p95 + 0 > l95 + 0 || p99 + 0 > l99 + 0) {
      over = over (over == "" ? "" : ", ") sprintf("%s p95 %s ms / p99 %s ms", name, p95, p99)
    }
  }
  END {
    missing = ""
    for (k = 1; k <= nwant; k++) {
      r = "BenchmarkDurableAccept_" want[k]
      if (!(r in seen)) missing = missing (missing == "" ? "" : " ") r
    }
    if (missing != "") { printf "not_measured\tmissing p95/p99 for: %s\n", missing; exit }
    measured = sprintf("worst p95 %s ms (%s), worst p99 %s ms (%s), across 4 benchmarks", w95, w95n, w99, w99n)
    if (over != "") { printf "failed\t%s\t%s\n", measured, over; exit }
    printf "met\t%s\n", measured
  }
' "$input")"

b1_outcome="$(printf '%s\n' "$b1_eval" | cut -f1)"
b1_detail="$(printf '%s\n' "$b1_eval" | cut -f2)"
b1_over="$(printf '%s\n' "$b1_eval" | cut -f3)"
b1_gap=""
case "$b1_outcome" in
  not_measured)
    b1_status="not_measured"
    b1_measured="not measured"
    b1_gap="$b1_detail"
    reasons+=("budget 1 not measured: $b1_detail")
    ;;
  failed)
    b1_status="failed"
    b1_measured="$b1_detail"
    b1_gap="over target: $b1_over"
    reasons+=("budget 1 failed: $b1_over")
    ;;
  met)
    b1_measured="$b1_detail"
    if [ "$pinned" = "true" ] && [ "$negative_control" = "false" ]; then
      b1_status="certified"
    else
      b1_status="harnessed"
      b1_gap="met, but this run cannot certify it (see certification.reason)"
    fi
    ;;
  *)
    echo "performance-report: could not evaluate budget 1" >&2
    exit 1
    ;;
esac

certified="false"
if [ "$b1_status" = "certified" ]; then
  certified="true"
fi

failed_budget="null"
if [ "$negative_control" = "true" ] && [ "$b1_status" = "failed" ]; then
  failed_budget=1
fi

reason=""
if [ "$certified" != "true" ] && [ "${#reasons[@]}" -gt 0 ]; then
  reason="$(printf '%s; ' "${reasons[@]}")"
  reason="${reason%; }"
fi

cat > "$output" <<JSON
{
  "schema_version": 2,
  "generated_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "certified": ${certified},
  "certification": {
    "runner_tag": "${runner_tag}",
    "runner_id": "${runner_id}",
    "reason": "${reason}"
  },
  "negative_control": {
    "ran": ${negative_control},
    "failed_budget": ${failed_budget}
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
      "target": "p95 <= ${b1_p95_limit} ms, p99 <= ${b1_p99_limit} ms",
      "measured": "${b1_measured}",
      "status": "${b1_status}",
      "gap": "${b1_gap}"
    },
    {
      "id": 2,
      "name": "one-hour steady-state throughput on the reference profile",
      "target": "declared 250 2-KiB msg/s sustained for one hour",
      "measured": "not measured",
      "status": "harnessed",
      "gap": "this job runs 300 accepts per benchmark in one process; the budget needs a one-hour run at the declared rate against two replicas of the reference profile"
    },
    {
      "id": 3,
      "name": "1-GiB batch import peak memory above idle",
      "target": "peak RSS <= 512 MiB above idle, and restart from the last durable checkpoint",
      "measured": "not measured",
      "status": "not_measured",
      "gap": "no workload in internal/integration/perf imports a batch; perf.HeapSampler runs only in a 32 MiB unit test and reads HeapAlloc, not the cgroup RSS the budget is written in"
    }
  ]
}
JSON

echo "performance-report: wrote $output (certified=${certified}, budget1=${b1_status}, negative_control=${negative_control})"

# A negative control that fails nothing means the harness cannot see a 300 ms
# regression. The report is written either way; the job goes red so that no one
# reads its sibling runs as evidence.
if [ "$negative_control" = "true" ] && [ "$failed_budget" = "null" ]; then
  echo "performance-report: NEGATIVE CONTROL NOT DETECTED — the injected regression failed no budget; no number from this harness may be cited" >&2
  exit 3
fi
