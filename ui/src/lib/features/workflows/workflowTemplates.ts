export type WorkflowTemplate = {
  id: string;
  name: string;
  description: string;
  yaml: string;
};

export const WORKFLOW_TEMPLATES: WorkflowTemplate[] = [
  {
    id: 'adt-routing',
    name: 'ADT Routing',
    description: 'Route admits/discharges to FHIR and audit logging.',
    yaml: `name: adt-routing
version: "1.0"
routes:
  - name: inpatient-admits
    filter:
      event_type: PATIENT_ADMIT
    actions:
      - type: fhir
        server: https://fhir.example.com
        resource_type: Encounter
        method: POST
      - type: log
        level: info
        message: Admit routed to FHIR

  - name: discharges
    filter:
      event_type: PATIENT_DISCHARGE
    actions:
      - type: webhook
        url: https://integration.example.com/hooks/discharge
        method: POST
      - type: log
        level: info
        message: Discharge routed to downstream systems
`
  },
  {
    id: 'critical-lab-alerting',
    name: 'Critical Lab Alerting',
    description: 'Detect critical labs and fan out alerts to queue and email.',
    yaml: `name: critical-lab-alerting
version: "1.0"
routes:
  - name: critical-labs
    filter:
      event_type: LAB_RESULT
      condition: event.severity == "critical"
    actions:
      - type: queue
        broker: nats://queue.example.com:4222
        topic: lab.critical
      - type: email
        to: clinical-alerts@example.com
        subject: Critical lab result received
      - type: log
        level: warn
        message: Critical lab alert published
`
  },
  {
    id: 'claims-fanout',
    name: 'Claims Fanout',
    description: 'Fan out claim submissions to payer API, queue, and audit store.',
    yaml: `name: claims-fanout
version: "1.0"
routes:
  - name: claims-submitted
    filter:
      event_type: CLAIM_SUBMITTED
    actions:
      - type: webhook
        url: https://payer-gateway.example.com/claims
        method: POST
      - type: queue
        broker: nats://queue.example.com:4222
        topic: claims.submitted
      - type: database
        dsn: postgres://claims-user:claims-pass@db.example.com:5432/claims
        table: claim_audit
      - type: log
        level: info
        message: Claim fanout completed
`
  }
];
