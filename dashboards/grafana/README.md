# fi-fhir Grafana Dashboards

Pre-built Grafana dashboards for monitoring the fi-fhir workflow engine.

## Prerequisites

1. **Prometheus** collecting metrics from your fi-fhir application
2. **Grafana** 9.0+ with Prometheus data source configured

## Available Dashboards

| Dashboard | File | Description |
|-----------|------|-------------|
| Workflow Overview | `workflow-overview.json` | Comprehensive view of event processing, actions, HTTP requests, resilience patterns, and DLQ |

## Import Instructions

### Option 1: Grafana UI

1. Open Grafana and navigate to **Dashboards** > **Import**
2. Click **Upload JSON file**
3. Select the dashboard JSON file (e.g., `workflow-overview.json`)
4. Select your Prometheus data source
5. Click **Import**

### Option 2: Grafana Provisioning

Add to your Grafana provisioning configuration:

```yaml
# /etc/grafana/provisioning/dashboards/fi-fhir.yaml
apiVersion: 1
providers:
  - name: fi-fhir
    type: file
    folder: fi-fhir
    options:
      path: /path/to/dashboards/grafana
```

### Option 3: Kubernetes ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-dashboards-fi-fhir
  labels:
    grafana_dashboard: "1"
data:
  workflow-overview.json: |
    # paste dashboard JSON here
```

## Dashboard Panels

### Workflow Overview Dashboard

**Overview Row**
- Events Processed (total count)
- Error Rate (percentage)
- Processing Latency (p99)
- DLQ Depth (current)

**Event Processing Row**
- Event Processing Rate by Type
- Event Processing Latency (p50/p95/p99)
- Event Success/Failure Rate
- Events Routed by Route

**Actions Row**
- Action Execution Rate by Type
- Action Latency by Type (p95/p99)
- Action Retries
- Action Error Rate by Type

**HTTP Requests Row**
- HTTP Request Rate by Method
- HTTP Request Latency (p50/p95/p99)
- HTTP Response Status Codes

**Resilience Row**
- Circuit Breaker State Changes
- Circuit Breaker Rejections
- Rate Limiting Activity
- Rate Limit Wait Duration

**Dead Letter Queue Row**
- DLQ Depth Over Time
- DLQ Push Rate by Error Type
- DLQ Reprocessing Rate

## Metric Naming

Dashboards expect metrics with the default Prometheus configuration:
- Namespace: `fi_fhir`
- Subsystem: `workflow`

If you customized the metric names, update the queries in the dashboard JSON.

## Alerting

Pre-built Prometheus alerting rules are available in [`../alerting/`](../alerting/):

| File | Description |
|------|-------------|
| `workflow-alerts.yaml` | Standalone Prometheus rules |
| `workflow-alerts-k8s.yaml` | PrometheusRule CRD for Kubernetes |

**18 alerts across 5 categories:**
- **Availability**: Error rate, action failures, no-event detection
- **Latency**: p99 for events, actions, HTTP requests
- **Resilience**: Circuit breaker, retry rate, rate limiting
- **DLQ**: Queue depth, push rate, reprocessing failures
- **HTTP**: 5xx/4xx error rates, authentication failures

See [Alerting README](../alerting/README.md) for installation and customization.

## Customization

### Changing Time Ranges

Edit the `time` field in the JSON:
```json
"time": { "from": "now-6h", "to": "now" }
```

### Adding Variables

Add to the `templating.list` array:
```json
{
  "name": "event_type",
  "type": "query",
  "query": "label_values(fi_fhir_workflow_events_processed_total, event_type)",
  "refresh": 1
}
```

Then use `${event_type}` in queries.

### Custom Metric Prefix

If you changed the namespace/subsystem, update queries:
- Find: `fi_fhir_workflow_`
- Replace: `your_namespace_your_subsystem_`
