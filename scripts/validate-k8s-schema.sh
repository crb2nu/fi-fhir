#!/usr/bin/env bash
# validate-k8s-schema.sh — Render every shipped deployment artifact and validate
# it against the pinned Kubernetes minor's own API schemas.
#
# Slice 4.4c. Until now the only check over deploy/ was `helm lint`,
# `helm template … > /dev/null`, and scripts/validate-kustomize-preview.sh,
# which greps eight strings out of the rendered output. None of those knows what
# a Deployment is. A field that does not exist, a field spelled wrong, a field
# that exists only in a newer Kubernetes than the one this product pins — all
# three render cleanly and fail at apply time, which is the worst place to find
# out during an upgrade exercise.
#
# Two things it is deliberately strict about:
#
#   * `-strict` rejects unknown fields. A typo in a manifest is otherwise
#     silently dropped by the API server, so the property you thought you
#     declared is simply absent.
#   * The Kubernetes version is PINNED, not "latest". docs/operations/
#     SUPPORTED-1.0.md:24 fixes 1.36.x as the reference target, and version
#     targeting has teeth here: the preStop `sleep` handler both Deployments now
#     use is rejected by the 1.28 schema and accepted by 1.36. Validating
#     against whatever is newest would stop answering the question this product
#     asks, which is whether these manifests apply to the minor it supports.
#
# It also renders deploy/helm/fi-fhir/values-reference-profile.yaml. Nothing
# rendered that file before, so the environment every 1.0 performance and
# recovery measurement is supposed to use could rot without anyone noticing.
#
# The negative control runs in the same invocation: a deliberately invalid
# manifest must be REJECTED. kubeconform skips resources whose schema it cannot
# find rather than failing, so a misconfigured schema location would otherwise
# make this gate pass by validating nothing at all.
#
# Usage:
#   scripts/validate-k8s-schema.sh
#
# Environment:
#   KUBERNETES_TARGET_VERSION  override the pinned minor (default below)
#   KUBECONFORM               path to the kubeconform binary
#
# Exit codes:
#   0 — every rendered artifact validates, and the negative control was rejected
#   1 — a rendered artifact is invalid, or the negative control was accepted
#   2 — a required tool is missing

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_ROOT"

# docs/operations/SUPPORTED-1.0.md:24. Patch releases may advance within the
# minor; the minor itself is a supported-matrix change and is not made here.
KUBERNETES_TARGET_VERSION="${KUBERNETES_TARGET_VERSION:-1.36.0}"
KUBECONFORM="${KUBECONFORM:-kubeconform}"

note() { printf 'validate-k8s-schema: %s\n' "$*"; }
fail() {
	printf 'validate-k8s-schema: %s\n' "$*" >&2
	exit 1
}

require() {
	command -v "$1" >/dev/null 2>&1 || {
		printf 'validate-k8s-schema: %s is required but not on PATH\n' "$1" >&2
		exit 2
	}
}

require helm

# `kustomize build` and `kubectl kustomize` are the same renderer; the CI image
# for this gate (dtzar/helm-kubectl) ships kubectl and not the standalone
# binary, and a developer workstation usually has the reverse.
if command -v kustomize >/dev/null 2>&1; then
	kustomize_build() { kustomize build "$1"; }
elif command -v kubectl >/dev/null 2>&1; then
	kustomize_build() { kubectl kustomize "$1"; }
else
	printf 'validate-k8s-schema: neither kustomize nor kubectl is on PATH\n' >&2
	exit 2
fi

# Same posture as the integration proofs: skip on a workstation that has not
# installed the tool, fail in CI so a missing binary cannot turn a real
# regression into a green pipeline.
if ! command -v "$KUBECONFORM" >/dev/null 2>&1; then
	if [ -n "${CI:-}" ]; then
		fail "kubeconform is required in CI (the job installs a pinned release)"
	fi
	note "kubeconform not found; skipping schema validation (install: go install github.com/yannh/kubeconform/cmd/kubeconform@latest)"
	exit 0
fi

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

validate() {
	local label="$1" path="$2"
	local output
	# Secret is skipped, and only Secret. deploy/kubernetes/base/
	# fi-fhir-umls.sops.yaml is a SOPS-encrypted document: at rest it carries a
	# top-level `sops:` key that is not part of the Secret schema and is removed
	# by decryption before anything applies it. Validating it as a Kubernetes
	# Secret asserts something about the ciphertext file, not about the manifest
	# the cluster receives. The kinds that carry the properties this slice is
	# about — Deployment, Service, Ingress, PDB, PVC — are all validated.
	if ! output="$("$KUBECONFORM" \
		-kubernetes-version "$KUBERNETES_TARGET_VERSION" \
		-skip Secret \
		-strict -summary "$path" 2>&1)"; then
		printf '%s\n' "$output" >&2
		fail "$label does not validate against Kubernetes ${KUBERNETES_TARGET_VERSION}"
	fi

	# kubeconform reports "Skipped" for resources it has no schema for. A
	# rendered artifact that is entirely skipped is not evidence of anything.
	local valid
	valid="$(printf '%s' "$output" | sed -nE 's/.*Valid: ([0-9]+).*/\1/p' | head -1)"
	if [ -z "$valid" ] || [ "$valid" -eq 0 ]; then
		printf '%s\n' "$output" >&2
		fail "$label validated zero resources; the check is vacuous"
	fi
	note "$label — $output"
}

note "target Kubernetes ${KUBERNETES_TARGET_VERSION}"

helm template fi-fhir deploy/helm/fi-fhir/ >"$WORKDIR/helm-defaults.yaml"
validate "helm chart, default values" "$WORKDIR/helm-defaults.yaml"

# The reference profile. Rendering it here is the whole reason it cannot rot:
# it is referenced only by prose in docs/operations/SUPPORTED-1.0.md, and no
# job, target, or script rendered it before slice 4.4c.
helm template fi-fhir deploy/helm/fi-fhir/ \
	-f deploy/helm/fi-fhir/values-reference-profile.yaml \
	>"$WORKDIR/helm-reference-profile.yaml"
validate "helm chart, reference profile values" "$WORKDIR/helm-reference-profile.yaml"

kustomize_build deploy/kubernetes/base >"$WORKDIR/kustomize-base.yaml"
validate "kustomize base" "$WORKDIR/kustomize-base.yaml"

kustomize_build deploy/kubernetes/overlays/production >"$WORKDIR/kustomize-production.yaml"
validate "kustomize production overlay" "$WORKDIR/kustomize-production.yaml"

# ---------------------------------------------------------------------------
# Negative control
# ---------------------------------------------------------------------------
#
# Two failures this gate could have and not notice: a schema location that
# resolves to nothing (every resource "Skipped", zero invalid, exit 0), and a
# -strict flag that stopped being passed. The control below trips on both. It
# runs in the same invocation, per the sprint's rule that a new blocking job
# carries a control that must fail.
cat >"$WORKDIR/negative-control.yaml" <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: negative-control
spec:
  replicas: 1
  # Not a field of DeploymentSpec in any Kubernetes version. -strict must
  # reject it; without -strict the API server would silently drop it.
  rollingUpdateStrategyy: nonsense
  selector:
    matchLabels:
      app: negative-control
  template:
    metadata:
      labels:
        app: negative-control
    spec:
      containers:
        - name: negative-control
          image: example:latest
YAML

if "$KUBECONFORM" -kubernetes-version "$KUBERNETES_TARGET_VERSION" \
	-strict -summary "$WORKDIR/negative-control.yaml" >/dev/null 2>&1; then
	fail "NEGATIVE CONTROL PASSED, WHICH MEANS THIS GATE IS BROKEN: kubeconform accepted a
  Deployment carrying a field that exists in no Kubernetes version. Either -strict is not
  reaching the validator or no schema is being resolved, and every check above validated
  nothing."
fi
note "negative control CONFIRMED: an unknown DeploymentSpec field is rejected"

note "OK — 4 rendered artifacts validate against Kubernetes ${KUBERNETES_TARGET_VERSION}"
