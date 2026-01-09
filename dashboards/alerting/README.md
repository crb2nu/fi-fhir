# Alerting Rules for fi-fhir Workflow Engine

This directory contains Prometheus alerting rules for monitoring the fi-fhir workflow engine.

## Files

| File | Description |
|------|-------------|
| `workflow-alerts.yaml` | Standalone Prometheus alerting rules |
| `workflow-alerts-k8s.yaml` | PrometheusRule CRD for Kubernetes/Prometheus Operator |

## Alert Categories

### Availability Alerts

| Alert | Severity | Description |
|-------|----------|-------------|
| `WorkflowHighErrorRate` | Critical | Event processing error rate > 5% |
| `WorkflowActionFailures` | Warning | Action execution failures > 0.1/sec |
| `WorkflowNoEventsProcessed` | Warning | No events processed for 30 minutes |

### Latency Alerts

| Alert | Severity | Description |
|-------|----------|-------------|
| `WorkflowHighEventLatency` | Warning | p99 event processing > 5 seconds |
| `WorkflowHighActionLatency` | Warning | p99 action execution > 10 seconds |
| `WorkflowHighHTTPLatency` | Warning | p99 HTTP request > 5 seconds |

### Resilience Alerts

| Alert | Severity | Description |
|-------|----------|-------------|
| `WorkflowCircuitBreakerOpen` | Critical | Circuit breaker opened for an endpoint |
| `WorkflowCircuitBreakerRejections` | Warning | Circuit breaker rejecting > 1 req/sec |
| `WorkflowHighRetryRate` | Warning | > 30% of actions being retried |
| `WorkflowRateLimitingActive` | Info | Rate limiting is active |
| `WorkflowRateLimitRejections` | Warning | Rate limiter rejecting requests |

### Dead Letter Queue Alerts

| Alert | Severity | Description |
|-------|----------|-------------|
| `WorkflowDLQGrowing` | Warning | DLQ depth > 100 events |
| `WorkflowDLQCritical` | Critical | DLQ depth > 1000 events |
| `WorkflowDLQHighPushRate` | Warning | Events pushed to DLQ > 1/sec |
| `WorkflowDLQReprocessingFailures` | Warning | > 50% reprocessing failures |

### HTTP Alerts

| Alert | Severity | Description |
|-------|----------|-------------|
| `WorkflowHTTP5xxErrors` | Warning | > 10% requests returning 5xx |
| `WorkflowHTTP4xxErrors` | Info | > 20% requests returning 4xx |
| `WorkflowAuthFailures` | Warning | 401 Unauthorized responses detected |

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

If using a Helm chart, mount the rules as a ConfigMap:

```yaml
# values.yaml
prometheus:
  additionalPrometheusRulesMap:
    fi-fhir:
      groups:
        # Copy groups from workflow-alerts.yaml
```

## Customization

### Adjusting Thresholds

Common adjustments based on traffic patterns:

```yaml
# Low-traffic environments (< 1 event/sec)
- alert: WorkflowNoEventsProcessed
  expr: sum(rate(fi_fhir_workflow_events_processed_total[1h])) == 0
  for: 2h  # Longer window

# High-traffic environments (> 1000 events/sec)
- alert: WorkflowHighErrorRate
  expr: |
    (
      sum(rate(fi_fhir_workflow_events_processed_total{success="false"}[1m]))
      /
      sum(rate(fi_fhir_workflow_events_processed_total[1m]))
    ) > 0.01  # Tighter threshold (1%)
  for: 2m  # Faster alert
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
        category: dlq
      receiver: 'dlq-handler'

receivers:
  - name: 'pagerduty-critical'
    pagerduty_configs:
      - service_key: ${PAGERDUTY_KEY}
  - name: 'slack-warnings'
    slack_configs:
      - channel: '#healthcare-alerts'
        api_url: ${SLACK_WEBHOOK_URL}
  - name: 'dlq-handler'
    webhook_configs:
      - url: 'http://dlq-reprocessor/trigger'
```

## Runbook Templates

Create runbooks for each alert. Example structure:

### WorkflowHighErrorRate

**Symptoms:**
- Error rate exceeds 5% of processed events
- Alert fires after 5 minutes of sustained errors

**Investigation:**
1. Check Grafana dashboard for error breakdown by route/action
2. Review recent deployments or configuration changes
3. Check external service health (FHIR servers, webhooks, databases)

**Resolution:**
```bash
# Check recent errors in logs
kubectl logs -l app=fi-fhir --tail=100 | grep -i error

# Check circuit breaker state
curl http://fi-fhir:8080/debug/circuit-breakers

# Review DLQ for failed events
kubectl exec -it deploy/fi-fhir -- fi-fhir dlq list --limit 10
```

### WorkflowCircuitBreakerOpen

**Symptoms:**
- Circuit breaker opened, requests failing fast
- External endpoint may be unavailable

**Investigation:**
1. Check the endpoint URL in alert labels
2. Verify external service is reachable
3. Review HTTP response codes and latency trends

**Resolution:**
```bash
# Test endpoint directly
curl -v https://fhir.hospital.org/r4/metadata

# Check network connectivity from pod
kubectl exec -it deploy/fi-fhir -- nc -zv fhir.hospital.org 443

# Force circuit breaker reset (if needed)
curl -X POST http://fi-fhir:8080/debug/circuit-breakers/reset?endpoint=fhir.hospital.org
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
      - series: 'fi_fhir_workflow_events_processed_total{success="true"}'
        values: '0+10x10'
      - series: 'fi_fhir_workflow_events_processed_total{success="false"}'
        values: '0+1x10'
    alert_rule_test:
      - eval_time: 10m
        alertname: WorkflowHighErrorRate
        exp_alerts:
          - exp_labels:
              severity: critical
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
  dashboard_uid: "workflow-overview"
  panel_id: "2"
```

## See Also

- [Grafana Dashboard](../grafana/README.md)
- [Prometheus Metrics](../../internal/workflow/metrics_prometheus.go)
- [Workflow DSL Documentation](../../docs/planning/WORKFLOW-DSL.md)
