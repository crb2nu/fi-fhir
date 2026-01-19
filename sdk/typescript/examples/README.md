# Examples

These examples assume you have `fi-fhir` available (either via `FI_FHIR_PATH`, or via a platform-specific
binary optional dependency when using the published package).

From the repo:

```bash
cd sdk/typescript
npm ci
npm run build

# Parse an HL7v2 message file
node examples/parse-hl7.mjs ../../testdata/adt_a01_sample.hl7

# Run a workflow against a JSON events file
node examples/workflow-run.mjs ../../examples/workflows/adt-to-fhir.yaml ./events.json
```

