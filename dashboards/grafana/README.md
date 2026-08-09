# fi-fhir Grafana Dashboards

Pre-built Grafana dashboards for monitoring the fi-fhir integration engine
runtime.

## Prerequisites

1. **Prometheus** collecting metrics from your fi-fhir application
2. **Grafana** 9.0+ with Prometheus data source configured

## Available Dashboards

| Dashboard | File | Description |
|-----------|------|-------------|
| Integration Engine Runtime | `workflow-overview.json` | Process/readiness state, HTTP+MLLP ingress, outbound delivery, batch ingestion, Integration Session stream, pending-autoroute sweep/notify, and Go runtime health |

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

### Integration Engine Runtime Dashboard

**Overview Row**
- Running Version(s) — `fi_fhir_build_info`
- Components Up — `fi_fhir_component_up` (graphql, mllp, delivery, batch, autoroute_sweep, autoroute_notify, session_stream)
- Readiness Dependencies — `fi_fhir_readiness_up` (submission_db, terminology_db, session_store, profile_store, workflow_lifecycle_store, event_store, mapping_store, plus background components)

**Ingress Row**
- HTTP Ingress Submissions by Outcome
- MLLP Messages by Outcome

**Delivery & Batch Row**
- Delivery Attempts by Outcome
- Batch Objects by Outcome

**Session Stream & Autoroute Row**
- Session Stream Events by Outcome
- Autoroute Sweep Rate & Expired Rows
- Autoroute Notifications by Outcome

**Process Runtime Row**
- Goroutines (`go_goroutines`)
- Process RSS (`process_resident_memory_bytes`)

## Metric Naming

Every panel queries a `fi_fhir_*` metric that
[`internal/observability/metrics.go`](../../internal/observability/metrics.go)
actually registers, served on `FI_FHIR_METRICS_PORT` (default `9090`) at
`FI_FHIR_METRICS_ENDPOINT` (default `/metrics`). See that file for the exact
metric names, types, and label allowlists — this dashboard does not invent
any query that isn't backed by a real collector.

If you rename a metric in `metrics.go`, update the matching query here in the
same change; a dashboard panel with no matching series renders an empty graph
with no error, so drift is silent until someone notices the panel is always
blank.

## Alerting

Prometheus alerting rules are available in [`../alerting/`](../alerting/):

| File | Description |
|------|-------------|
| `workflow-alerts.yaml` | Standalone Prometheus rules |
| `workflow-alerts-k8s.yaml` | PrometheusRule CRD for Kubernetes |

**10 alerts across 4 categories:**
- **Availability**: scrape target down, component down, readiness dependency unhealthy
- **Ingress**: HTTP ingress error rate, MLLP rejection rate
- **Delivery**: delivery failure rate, batch failure rate, session stream error rate
- **Autoroute**: expiry sweep failures, review notification failures

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
  "name": "component",
  "type": "query",
  "query": "label_values(fi_fhir_component_up, component)",
  "refresh": 1
}
```

Then use `${component}` in queries.
