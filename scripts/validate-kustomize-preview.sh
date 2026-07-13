#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT HUP INT TERM

assert_contains() {
  file=$1
  value=$2
  if ! grep -F -q -- "$value" "$file"; then
    echo "missing rendered deployment value: $value" >&2
    exit 1
  fi
}

assert_once() {
  file=$1
  value=$2
  count=$(grep -F -c -- "$value" "$file" || true)
  if [ "$count" -ne 1 ]; then
    echo "expected one rendered deployment value, found $count: $value" >&2
    exit 1
  fi
}

base="$TMP_DIR/base.yaml"
production="$TMP_DIR/production.yaml"
kubectl kustomize "$ROOT_DIR/deploy/kubernetes/base" > "$base"
kubectl kustomize "$ROOT_DIR/deploy/kubernetes/overlays/production" > "$production"

for rendered in "$base" "$production"; do
  assert_contains "$rendered" "- serve"
  assert_contains "$rendered" "- --no-playground"
  assert_contains "$rendered" "- --no-introspection"
  assert_contains "$rendered" "defaultMode: 288"
  assert_contains "$rendered" "nginx.ingress.kubernetes.io/proxy-body-size: 1m"
  assert_contains "$rendered" "nginx.ingress.kubernetes.io/proxy-request-buffering: \"off\""
  assert_contains "$rendered" "/var/run/secrets/fi-fhir/GRAPHQL_BEARER_TOKEN"
  assert_contains "$rendered" "/app/config/registry.json"
  for variable in \
    FI_FHIR_DEPLOYMENT_TENANT_ID \
    FI_FHIR_GRAPHQL_PRINCIPAL_ID \
    FI_FHIR_GRAPHQL_ROLES \
    FI_FHIR_GRAPHQL_ALLOWED_ORIGINS \
    FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE \
    FI_FHIR_INTEGRATION_REGISTRY_PATH
  do
    assert_once "$rendered" "name: $variable"
  done
done

assert_contains "$base" "value: https://fi-fhir.example.com"
assert_contains "$production" "value: https://fi-fhir.prod.example.com"

echo "Kustomize authenticated-preview deployment validation passed"
