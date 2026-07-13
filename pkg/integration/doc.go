// Package integration defines the stable, dependency-light contracts shared by
// fi-fhir ingress, processing, persistence, APIs, and authoring tools.
//
// It deliberately contains no parser, workflow, storage, or delivery runtime.
// IntegrationDefinitionRevision binds those future adapters to exact immutable
// artifacts, while ProcessRequest and ProcessResult enforce tenant, identity,
// PHI-retention, correlation, and preview/production safety invariants.
package integration
