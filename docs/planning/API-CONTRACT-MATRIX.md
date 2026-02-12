# Event Contract Matrix

Generated: 2026-02-12T15:00:23Z

## Inputs

- Canonical: `pkg/events/events.go`
- GraphQL: `internal/api/graphql/schema.graphql`
- OpenAPI: `api/openapi.yaml`

## Summary

- Canonical count: 36
- GraphQL count: 16
- OpenAPI count: 11
- Missing in GraphQL (vs canonical): 20
- Extra in GraphQL (not in canonical): 0
- Missing in OpenAPI (vs canonical): 27
- Extra in OpenAPI (not in canonical): 2

## Matrix

| Event Type | Canonical | GraphQL | OpenAPI |
|---|---|---|---|
| `allergy_intolerance` | yes | no | no |
| `appointment_booked` | no | no | yes |
| `appointment_cancelled` | yes | yes | yes |
| `appointment_checked_in` | yes | no | no |
| `appointment_modified` | yes | no | no |
| `appointment_noshow` | yes | yes | no |
| `appointment_rescheduled` | yes | no | no |
| `appointment_scheduled` | yes | yes | no |
| `claim_adjudicated` | yes | yes | no |
| `claim_response` | no | no | yes |
| `claim_status_request` | yes | no | no |
| `claim_status_response` | yes | no | no |
| `claim_submitted` | yes | yes | yes |
| `condition` | yes | yes | no |
| `document` | yes | yes | no |
| `document_addendum` | yes | no | no |
| `document_edit` | yes | no | no |
| `document_original` | yes | no | no |
| `document_replacement` | yes | no | no |
| `document_status_change` | yes | no | no |
| `eligibility_inquiry` | yes | no | yes |
| `eligibility_response` | yes | no | yes |
| `financial_transaction` | yes | no | no |
| `immunization` | yes | yes | no |
| `lab_cancelled` | yes | no | no |
| `lab_ordered` | yes | yes | no |
| `lab_result` | yes | yes | yes |
| `medication_request` | yes | no | no |
| `patient_admit` | yes | yes | yes |
| `patient_discharge` | yes | yes | yes |
| `patient_merge` | yes | no | no |
| `patient_transfer` | yes | yes | yes |
| `patient_update` | yes | yes | yes |
| `prior_auth_request` | yes | no | no |
| `prior_auth_response` | yes | no | no |
| `procedure` | yes | yes | no |
| `social_history` | yes | no | no |
| `vital_sign` | yes | yes | no |

## Drift Details

### Missing In GraphQL

- `allergy_intolerance`
- `appointment_checked_in`
- `appointment_modified`
- `appointment_rescheduled`
- `claim_status_request`
- `claim_status_response`
- `document_addendum`
- `document_edit`
- `document_original`
- `document_replacement`
- `document_status_change`
- `eligibility_inquiry`
- `eligibility_response`
- `financial_transaction`
- `lab_cancelled`
- `medication_request`
- `patient_merge`
- `prior_auth_request`
- `prior_auth_response`
- `social_history`

### Extra In GraphQL

- none

### Missing In OpenAPI

- `allergy_intolerance`
- `appointment_checked_in`
- `appointment_modified`
- `appointment_noshow`
- `appointment_rescheduled`
- `appointment_scheduled`
- `claim_adjudicated`
- `claim_status_request`
- `claim_status_response`
- `condition`
- `document`
- `document_addendum`
- `document_edit`
- `document_original`
- `document_replacement`
- `document_status_change`
- `financial_transaction`
- `immunization`
- `lab_cancelled`
- `lab_ordered`
- `medication_request`
- `patient_merge`
- `prior_auth_request`
- `prior_auth_response`
- `procedure`
- `social_history`
- `vital_sign`

### Extra In OpenAPI

- `appointment_booked`
- `claim_response`

## Notes

- GraphQL enums are normalized from uppercase to lowercase snake case for comparison.
- OpenAPI values are compared as-is from `components.schemas.Event.properties.type.enum`.
- Use `--strict` to fail fast when drift exists.
