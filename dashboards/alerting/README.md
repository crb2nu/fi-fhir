# Alerting Rules for the fi-fhir Integration Engine

This directory contains Prometheus alerting rules for monitoring the fi-fhir
integration engine runtime — the process started by `fi-fhir serve`, not the
legacy `internal/workflow` engine.

## Files

| File | Description |
|------|-------------|
| `workflow-alerts.yaml` | Standalone Prometheus alerting rules |
| `workflow-alerts-k8s.yaml` | PrometheusRule CRD for Kubernetes/Prometheus Operator |

Both files define the same 10 rules; `workflow-alerts-k8s.yaml` wraps them in
a `PrometheusRule` CRD instead of a bare `rule_files` document. The filenames
predate this rewrite and are kept as-is so existing `rule_files` and
`kubectl apply` references do not break.

## Alert Categories

Every expression below queries a metric
[`internal/observability/metrics.go`](../../internal/observability/metrics.go)
actually registers. The previous rule set (32 rules) targeted
`fi_fhir_workflow_*` metric names nothing in this codebase ever emitted, so
none of them could fire; that rule set has been fully replaced, not extended.

| Alert | Severity | Description |
|-------|----------|-------------|
| `FiFhirScrapeTargetDown` | Critical | Prometheus has not scraped the fi-fhir target in 5 minutes |
| `FiFhirComponentDown` | Critical | A configured background component (`fi_fhir_component_up`) stopped running |
| `FiFhirReadinessComponentUnhealthy` | Critical | A readiness dependency (`fi_fhir_readiness_up`) is reporting unhealthy — `/ready` is returning 503 |
| `FiFhirIngressErrorRate` | Warning | HTTP ingress infrastructure error rate > 5% |
| `FiFhirMLLPRejectionRate` | Warning | MLLP frame rejection/error rate > 10% |
| `FiFhirDeliveryFailureRate` | Critical | Outbound delivery terminal-failure/error rate > 5% (closest available signal to a DLQ backlog; fi-fhir has no DLQ-depth metric) |
| `FiFhirBatchFailureRate` | Warning | Batch object failure rate > 10% |
| `FiFhirSessionStreamErrorRate` | Warning | Integration Session stream drop/error rate > 5% |
| `FiFhirAutorouteSweepFailureRate` | Warning | Pending-autoroute expiry sweep erroring |
| `FiFhirAutorouteNotificationFailureRate` | Warning | Pending-autoroute review notifications failing to send |

### Rules removed in this rewrite

The following 22 rules from the prior version were deleted because their
metric (`fi_fhir_workflow_events_processed_total`,
`fi_fhir_workflow_actions_executed_total`,
`fi_fhir_workflow_events_processed_duration_seconds_bucket`,
`fi_fhir_workflow_actions_executed_duration_seconds_bucket`,
`fi_fhir_workflow_http_requests_duration_seconds_bucket`,
`fi_fhir_workflow_circuit_breaker_state_changes_total`,
`fi_fhir_workflow_circuit_breaker_rejections_total`,
`fi_fhir_workflow_action_retries_total`,
`fi_fhir_workflow_rate_limit_waits_total`,
`fi_fhir_workflow_rate_limit_rejections_total`,
`fi_fhir_workflow_dlq_depth`, `fi_fhir_workflow_dlq_pushed_total`,
`fi_fhir_workflow_dlq_popped_total`, `fi_fhir_workflow_http_requests_total`)
has no emitter anywhere in this codebase: `WorkflowHighErrorRate`,
`WorkflowActionFailures`, `WorkflowNoEventsProcessed`,
`WorkflowHighEventLatency`, `WorkflowHighActionLatency`,
`WorkflowHighHTTPLatency`, `WorkflowCircuitBreakerOpen`,
`WorkflowCircuitBreakerRejections`, `WorkflowHighRetryRate`,
`WorkflowRateLimitingActive`, `WorkflowRateLimitRejections`,
`WorkflowDLQGrowing`, `WorkflowDLQCritical`, `WorkflowDLQHighPushRate`,
`WorkflowDLQReprocessingFailures`, `WorkflowHTTP5xxErrors`,
`WorkflowHTTP4xxErrors`, `WorkflowAuthFailures`. Circuit breakers, rate
limiting, and per-route/action metrics do not exist in the current
integration engine; if that capability returns, add its metric to
`internal/observability/metrics.go` first, then a rule here — never the other
way around.

## Installation

### Standalone Prometheus

Add to your `prometheus.yml`:

```yaml
rule_files:
  - /etc/prometheus/rules/workflow-alerts.yaml
```

Then reload Prometheus:
```bash
curl -X POST http://prometheus:9090/-/reload
```

### Prometheus Operator (Kubernetes)

Apply the PrometheusRule CRD:

```bash
# Update namespace if needed
kubectl apply -f workflow-alerts-k8s.yaml -n monitoring
```

Ensure your Prometheus is configured to select the rule:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: Prometheus
metadata:
  name: main
spec:
  ruleSelector:
    matchLabels:
      prometheus: main
      role: alert-rules
```

### Helm Chart

The chart's own `deploy/helm/fi-fhir/templates/prometheusrule.yaml` renders
whatever list you pass as `.Values.prometheusRule.rules` (empty by default,
and gated behind `.Values.prometheusRule.enabled`). To use these rules via
Helm, copy the `groups` from `workflow-alerts.yaml` into that values key
instead of applying `workflow-alerts-k8s.yaml` directly:

```yaml
# values.yaml
prometheusRule:
  enabled: true
  rules:
    # Copy groups from workflow-alerts.yaml
```

## Customization

### Adjusting Thresholds

Common adjustments based on traffic patterns:

```yaml
# Low-traffic environments
- alert: FiFhirIngressErrorRate
  expr: |
    (
      sum(rate(fi_fhir_http_ingress_submissions_total{outcome="error"}[15m]))
      /
      sum(rate(fi_fhir_http_ingress_submissions_total[15m]))
    ) > 0.05
  for: 15m  # Longer window smooths low-volume noise

# High-traffic environments
- alert: FiFhirDeliveryFailureRate
  expr: |
    (
      sum(rate(fi_fhir_delivery_attempts_total{outcome=~"failed|error"}[5m]))
      /
      sum(rate(fi_fhir_delivery_attempts_total[5m]))
    ) > 0.01  # Tighter threshold (1%)
  for: 5m  # Faster alert
```

### Adding Labels

Add environment or team labels:

```yaml
labels:
  severity: critical
  service: fi-fhir
  team: healthcare-integration  # Add team
  environment: production       # Add environment
```

### Alertmanager Routing

Route alerts based on labels:

```yaml
# alertmanager.yml
route:
  receiver: 'default'
  routes:
    - match:
        service: fi-fhir
        severity: critical
      receiver: 'pagerduty-critical'
    - match:
        service: fi-fhir
        severity: warning
      receiver: 'slack-warnings'
    - match:
        service: fi-fhir
        category: delivery
      receiver: 'delivery-oncall'

receivers:
  - name: 'pagerduty-critical'
    pagerduty_configs:
      - service_key: ${PAGERDUTY_KEY}
  - name: 'slack-warnings'
    slack_configs:
      - channel: '#healthcare-alerts'
        api_url: ${SLACK_WEBHOOK_URL}
  - name: 'delivery-oncall'
    webhook_configs:
      - url: 'http://delivery-oncall/trigger'
```

## Runbook Templates

Create runbooks for each alert. Example structure:

### FiFhirReadinessComponentUnhealthy

**Symptoms:**
- `GET /ready` returns 503
- Alert fires after 5 minutes of sustained unhealthy status on one component

**Investigation:**
1. Check `GET /ready` directly for the unhealthy component's message field
2. Check the dependency's own health (database, session store, etc.)
3. Review recent deployments or configuration changes

**Resolution:**
```bash
# Check the readiness report directly
curl http://fi-fhir:8080/ready

# Check recent errors in logs
kubectl logs -l app.kubernetes.io/name=fi-fhir --tail=100 | grep -i error
```

### FiFhirDeliveryFailureRate

**Symptoms:**
- Elevated `fi_fhir_delivery_attempts_total{outcome="failed"}` or `{outcome="error"}`
- Outbound delivery to the downstream endpoint is not keeping up

**Investigation:**
1. Check Grafana's Delivery & Batch row for the failure trend
2. Check the downstream endpoint's own availability
3. Review dispatcher logs for the specific error

**Resolution:**
```bash
# Check recent dispatcher errors in logs
kubectl logs -l app.kubernetes.io/name=fi-fhir --tail=200 | grep -i delivery
```

## Testing Alerts

Use Prometheus's built-in testing:

```yaml
# test-alerts.yaml
rule_files:
  - workflow-alerts.yaml

evaluation_interval: 1m

tests:
  - interval: 1m
    input_series:
      - series: 'fi_fhir_http_ingress_submissions_total{outcome="accepted"}'
        values: '0+10x10'
      - series: 'fi_fhir_http_ingress_submissions_total{outcome="error"}'
        values: '0+1x10'
    alert_rule_test:
      - eval_time: 10m
        alertname: FiFhirIngressErrorRate
        exp_alerts:
          - exp_labels:
              severity: warning
              service: fi-fhir
```

Run tests:
```bash
promtool test rules test-alerts.yaml
```

## Integration with Grafana

Link alerts to dashboard panels:

1. In Grafana, go to Alerting → Alert rules
2. Create alert rule linked to the Prometheus rule
3. Add dashboard UID and panel ID to annotations:

```yaml
annotations:
  dashboard_uid: "fi-fhir-integration-runtime"
  panel_id: "3"
```

## See Also

- [Grafana Dashboard](../grafana/README.md)
- [Prometheus Metrics](../../internal/observability/metrics.go)
