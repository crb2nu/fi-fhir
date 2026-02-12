# Event Contract Matrix

Generated: 2026-02-12T15:02:52Z

## Inputs

- Canonical: `pkg/events/events.go`
- GraphQL: `internal/api/graphql/schema.graphql`
- OpenAPI: `api/openapi.yaml`

## Summary

- Canonical count: 36
- GraphQL count: 36
- OpenAPI count: 36
- Missing in GraphQL (vs canonical): 0
- Extra in GraphQL (not in canonical): 0
- Missing in OpenAPI (vs canonical): 0
- Extra in OpenAPI (not in canonical): 0

## Matrix

| Event Type | Canonical | GraphQL | OpenAPI |
|---|---|---|---|
| `allergy_intolerance` | yes | yes | yes |
| `appointment_cancelled` | yes | yes | yes |
| `appointment_checked_in` | yes | yes | yes |
| `appointment_modified` | yes | yes | yes |
| `appointment_noshow` | yes | yes | yes |
| `appointment_rescheduled` | yes | yes | yes |
| `appointment_scheduled` | yes | yes | yes |
| `claim_adjudicated` | yes | yes | yes |
| `claim_status_request` | yes | yes | yes |
| `claim_status_response` | yes | yes | yes |
| `claim_submitted` | yes | yes | yes |
| `condition` | yes | yes | yes |
| `document` | yes | yes | yes |
| `document_addendum` | yes | yes | yes |
| `document_edit` | yes | yes | yes |
| `document_original` | yes | yes | yes |
| `document_replacement` | yes | yes | yes |
| `document_status_change` | yes | yes | yes |
| `eligibility_inquiry` | yes | yes | yes |
| `eligibility_response` | yes | yes | yes |
| `financial_transaction` | yes | yes | yes |
| `immunization` | yes | yes | yes |
| `lab_cancelled` | yes | yes | yes |
| `lab_ordered` | yes | yes | yes |
| `lab_result` | yes | yes | yes |
| `medication_request` | yes | yes | yes |
| `patient_admit` | yes | yes | yes |
| `patient_discharge` | yes | yes | yes |
| `patient_merge` | yes | yes | yes |
| `patient_transfer` | yes | yes | yes |
| `patient_update` | yes | yes | yes |
| `prior_auth_request` | yes | yes | yes |
| `prior_auth_response` | yes | yes | yes |
| `procedure` | yes | yes | yes |
| `social_history` | yes | yes | yes |
| `vital_sign` | yes | yes | yes |

## Drift Details

### Missing In GraphQL

- none

### Extra In GraphQL

- none

### Missing In OpenAPI

- none

### Extra In OpenAPI

- none

## Notes

- GraphQL enums are normalized from uppercase to lowercase snake case for comparison.
- OpenAPI values are compared as-is from `components.schemas.Event.properties.type.enum`.
- Use `--strict` to fail fast when drift exists.
