package main

import (
	"net"
	"os"
	"strconv"

	integrationbatch "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	integrationdelivery "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	integrationdestination "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	integrationingress "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/ingress"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/mllp"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/registry"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/config"
)

// The engine runtime description (.loom/38 C-0, "Engine properties").
//
// runServe is the only place that knows what this replica mounted, so it
// builds the description once, at the end of composition, and hands the same
// value to the resolver and the connection catalog. Nothing below reads the
// environment generically: adapters are described from what
// loadIntegrationRuntimeFromEnv actually built, and process properties are
// the closed allowlist serveProperties, which TestServePropertiesAreExactlyTheDocumentedKeys
// pins to the keys `serve --help` documents.

// runtimeComposition records what loadIntegrationRuntimeFromEnv mounted. It
// holds identifiers, digests, and bounds — never a credential and never
// message content — and exists only to describe the runtime.
type runtimeComposition struct {
	graphQLAuthMode string
	registry        *registry.StaticRegistry
	http            *httpIngressFacts
	mllp            *mllpFacts
	batch           *batchFacts
	delivery        *deliveryFacts
}

type httpIngressFacts struct {
	integrationID string
	authMode      string
	maxBodyBytes  int64
}

type mllpFacts struct {
	source                mllp.SourceRevision
	definitionID          string
	requireClientIdentity bool
}

type batchFacts struct {
	source                  integrationbatch.SourceRevision
	definitionID            string
	workerID                string
	requireWorkloadIdentity bool
}

type deliveryFacts struct {
	workerID     string
	maxAttempts  int
	queueDriver  string
	identityMode integrationdestination.Mode
	identity     *integrationdestination.Registry
}

// engineRuntimeFacts are the process-level facts runServe decides after the
// integration runtime is loaded.
type engineRuntimeFacts struct {
	version             string
	host                string
	port                int
	controlPlane        bool
	integrationSessions bool
	streaming           bool
	retentionPurge      bool
	llmConfigured       bool
}

// serveProperty is one allowlisted process property: its environment key,
// whether its value is secret, and the default the code applies when it is
// unset ("" when there is none). A secret property renders only as set/unset.
type serveProperty struct {
	key          string
	secret       bool
	defaultValue string
}

// serveProperties is the closed allowlist of process properties the engine
// runtime reports: exactly the environment keys `serve --help` documents, with
// `[_FILE]` expanded to both keys and the documented FI_FHIR_DATABASE_* family
// expanded to the members the durable database opener reads. Every key whose
// name says it carries a credential — or the path to one — is secret.
func serveProperties() []serveProperty {
	databaseDefaults := config.Default().Database
	deliveryDefaults := integrationdelivery.DefaultConfig()
	return []serveProperty{
		{key: "FI_FHIR_DEPLOYMENT_TENANT_ID"},
		{key: "FI_FHIR_GRAPHQL_AUTH_MODE", defaultValue: graphqlAuthModeStatic},
		{key: "FI_FHIR_GRAPHQL_ALLOWED_ORIGINS"},
		{key: "FI_FHIR_INTEGRATION_REGISTRY_PATH"},

		{key: "FI_FHIR_GRAPHQL_PRINCIPAL_ID"},
		{key: "FI_FHIR_GRAPHQL_ROLES"},
		{key: "FI_FHIR_GRAPHQL_BEARER_TOKEN", secret: true},
		{key: "FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE", secret: true},
		{key: "FI_FHIR_GRAPHQL_SERVICE_BEARER_TOKEN_FILE", secret: true},
		{key: "FI_FHIR_GRAPHQL_SERVICE_PRINCIPAL_ID"},

		{key: "FI_FHIR_GRAPHQL_OIDC_ISSUER_URL"},
		{key: "FI_FHIR_GRAPHQL_OIDC_AUDIENCE"},
		{key: "FI_FHIR_GRAPHQL_OIDC_TENANT_CLAIM", defaultValue: "tenant_id"},
		{key: "FI_FHIR_GRAPHQL_OIDC_ROLES_CLAIM", defaultValue: "roles"},
		{key: "FI_FHIR_GRAPHQL_OIDC_SIGNING_ALGS", defaultValue: "RS256"},
		{key: "FI_FHIR_GRAPHQL_TRUSTED_CIDRS"},

		{key: "FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED", defaultValue: "false"},

		{key: "FI_FHIR_HTTP_INGRESS_AUTH_MODE"},
		{key: "FI_FHIR_HTTP_INGRESS_PRINCIPAL_ID"},
		{key: "FI_FHIR_HTTP_INGRESS_INTEGRATION_ID"},
		{key: "FI_FHIR_HTTP_INGRESS_SECRET", secret: true},
		{key: "FI_FHIR_HTTP_INGRESS_SECRET_FILE", secret: true},
		{key: "FI_FHIR_HTTP_INGRESS_OAUTH_ISSUER_URL"},
		{key: "FI_FHIR_HTTP_INGRESS_OAUTH_AUDIENCE"},
		{key: "FI_FHIR_HTTP_INGRESS_OAUTH_TENANT_CLAIM", defaultValue: connection.DefaultHTTPOAuthTenantClaim},
		{key: "FI_FHIR_HTTP_INGRESS_OAUTH_ROLES_CLAIM", defaultValue: connection.DefaultHTTPOAuthRolesClaim},
		{key: "FI_FHIR_HTTP_INGRESS_OAUTH_CLIENT_ID_CLAIM", defaultValue: connection.DefaultHTTPOAuthClientIDClaim},
		{key: "FI_FHIR_HTTP_INGRESS_OAUTH_SIGNING_ALGS", defaultValue: connection.DefaultHTTPOAuthSigningAlg},
		{key: "FI_FHIR_HTTP_INGRESS_OAUTH_ALLOWED_CLIENT_IDS"},
		{key: "FI_FHIR_HTTP_INGRESS_MAX_BODY_BYTES", defaultValue: strconv.FormatInt(integrationingress.DefaultMaxBodyBytes, 10)},

		{key: "FI_FHIR_DATABASE_DRIVER", defaultValue: databaseDefaults.Driver},
		{key: "FI_FHIR_DATABASE_HOST"},
		{key: "FI_FHIR_DATABASE_PORT", defaultValue: strconv.Itoa(databaseDefaults.Port)},
		{key: "FI_FHIR_DATABASE_NAME"},
		{key: "FI_FHIR_DATABASE_USERNAME"},
		{key: "FI_FHIR_DATABASE_USER"},
		{key: "FI_FHIR_DATABASE_PASSWORD", secret: true},
		{key: "FI_FHIR_DATABASE_SSL_MODE", defaultValue: databaseDefaults.SSLMode},
		{key: "FI_FHIR_DATABASE_MAX_OPEN_CONNS", defaultValue: strconv.Itoa(databaseDefaults.MaxOpenConns)},
		{key: "FI_FHIR_DATABASE_MAX_IDLE_CONNS", defaultValue: strconv.Itoa(databaseDefaults.MaxIdleConns)},
		{key: "FI_FHIR_DATABASE_CONN_MAX_LIFETIME", defaultValue: databaseDefaults.ConnMaxLifetime.String()},

		{key: "FI_FHIR_MLLP_SOURCE_CONFIG_PATH"},
		{key: "FI_FHIR_MLLP_DEFINITION_ID"},
		{key: "FI_FHIR_MLLP_PRINCIPAL_ID"},
		{key: "FI_FHIR_MLLP_REQUIRE_CLIENT_IDENTITY", defaultValue: "false"},
		{key: "FI_FHIR_MLLP_TLS_CERT_FILE"},
		{key: "FI_FHIR_MLLP_TLS_KEY_FILE", secret: true},
		{key: "FI_FHIR_MLLP_TLS_CLIENT_CA_FILE"},

		{key: "FI_FHIR_BATCH_SOURCE_CONFIG_PATH"},
		{key: "FI_FHIR_BATCH_DEFINITION_ID"},
		{key: "FI_FHIR_BATCH_PRINCIPAL_ID"},
		{key: "FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY", defaultValue: "false"},
		{key: "FI_FHIR_BATCH_WORKER_ID"},
		{key: "FI_FHIR_BATCH_S3_ACCESS_KEY", secret: true},
		{key: "FI_FHIR_BATCH_S3_ACCESS_KEY_FILE", secret: true},
		{key: "FI_FHIR_BATCH_S3_SECRET_KEY", secret: true},
		{key: "FI_FHIR_BATCH_S3_SECRET_KEY_FILE", secret: true},
		{key: "FI_FHIR_BATCH_SFTP_KNOWN_HOSTS_FILE"},
		{key: "FI_FHIR_BATCH_SFTP_PASSWORD", secret: true},
		{key: "FI_FHIR_BATCH_SFTP_PASSWORD_FILE", secret: true},
		{key: "FI_FHIR_BATCH_SFTP_PRIVATE_KEY_FILE", secret: true},

		{key: "FI_FHIR_DELIVERY_WORKER_ENABLED", defaultValue: "false"},
		{key: "FI_FHIR_DELIVERY_WORKER_ID"},
		{key: "FI_FHIR_QUEUE_DRIVER"},
		{key: "FI_FHIR_QUEUE_BROKERS"},
		{key: "FI_FHIR_QUEUE_CLIENT_ID"},
		{key: "FI_FHIR_QUEUE_TLS", defaultValue: "false"},
		{key: "FI_FHIR_QUEUE_TLS_ROOT_CA_FILE"},
		{key: "FI_FHIR_QUEUE_USERNAME"},
		{key: "FI_FHIR_QUEUE_PASSWORD", secret: true},
		{key: "FI_FHIR_QUEUE_PASSWORD_FILE", secret: true},
		{key: "FI_FHIR_DELIVERY_MAX_ATTEMPTS", defaultValue: strconv.Itoa(deliveryDefaults.MaxAttempts)},
		{key: "FI_FHIR_DELIVERY_RETRY_BASE_DELAY", defaultValue: deliveryDefaults.RetryBaseDelay.String()},
		{key: "FI_FHIR_DELIVERY_RETRY_MAX_DELAY", defaultValue: deliveryDefaults.RetryMaxDelay.String()},

		{key: "TEMPORAL_ADDRESS"},
		{key: "TEMPORAL_NAMESPACE", defaultValue: "terminology-mapping"},
	}
}

// describeServeProperties renders the allowlist against the environment. It
// reads exactly the allowlisted keys, one by one.
func describeServeProperties() []connection.RuntimeProperty {
	allowlist := serveProperties()
	properties := make([]connection.RuntimeProperty, 0, len(allowlist))
	for _, property := range allowlist {
		value := os.Getenv(property.key)
		properties = append(properties, connection.NewRuntimeProperty(
			property.key, property.secret, value, value != "", property.defaultValue,
		))
	}
	return properties
}

// buildEngineRuntimeDescription describes what this replica composed. It is
// called once, after every adapter is decided.
func buildEngineRuntimeDescription(runtime *previewRuntime, facts engineRuntimeFacts) (*connection.RuntimeDescription, error) {
	replicaID, err := replicaHolderID()
	if err != nil {
		replicaID = "unavailable"
	}
	composition := runtime.composition
	description := &connection.RuntimeDescription{
		Version:             facts.version,
		TenantID:            runtime.tenantID,
		ReplicaID:           replicaID,
		AuthMode:            composition.graphQLAuthMode,
		TrustedNetwork:      runtime.trustedNetwork != nil,
		AccessIdentity:      runtime.accessIdentity != nil,
		ControlPlane:        facts.controlPlane,
		IntegrationSessions: facts.integrationSessions,
		Streaming:           facts.streaming,
		RetentionPurge:      facts.retentionPurge,
		LLMConfigured:       facts.llmConfigured,
		Properties:          describeServeProperties(),
	}
	summaries := composition.registry.Integrations()
	for _, summary := range summaries {
		description.Registry.Integrations = append(description.Registry.Integrations, connection.RuntimeRegistryIntegration{
			IntegrationID: summary.IntegrationID,
			DefinitionID:  summary.IntegrationRevision.ArtifactID,
			RevisionID:    summary.IntegrationRevision.RevisionID,
			Digest:        summary.IntegrationRevision.Digest,
			SourceID:      summary.Source.SourceID,
			Format:        string(summary.Format),
		})
	}
	description.Registry.IntegrationCount = len(description.Registry.Integrations)

	description.Adapters = [4]connection.RuntimeAdapter{
		describeHTTPAdapter(runtime, summaries, net.JoinHostPort(facts.host, strconv.Itoa(facts.port))),
		describeMLLPAdapter(runtime),
		describeBatchAdapter(runtime),
		describeDeliveryAdapter(runtime),
	}
	if delivery := composition.delivery; delivery != nil && delivery.identity != nil && runtime.deliveryWorker != nil {
		identity := &connection.RuntimeDestinationIdentity{Mode: string(delivery.identityMode)}
		for _, revision := range delivery.identity.Destinations() {
			identity.Destinations = append(identity.Destinations, connection.RuntimeDestination{
				ArtifactID:       revision.ArtifactID,
				RevisionID:       revision.RevisionID,
				Digest:           revision.Digest,
				Transport:        string(revision.Transport),
				Class:            string(revision.Class),
				EndpointAdvisory: connection.EndpointAdvisory(revision.EndpointAdvisory()),
			})
		}
		description.DestinationIdentity = identity
	}
	for _, ledger := range schemaLedgers() {
		description.Ledgers = append(description.Ledgers, connection.RuntimeLedger{Name: ledger.Name, Version: ledger.Version})
	}
	if err := description.Validate(); err != nil {
		return nil, err
	}
	return description, nil
}

// describeHTTPAdapter reports the HTTP ingress. It mounts no source document of
// its own: its source revision is the one named by the definition its bound
// integration resolves to in the static registry.
func describeHTTPAdapter(runtime *previewRuntime, summaries []registry.IntegrationSummary, listenAddress string) connection.RuntimeAdapter {
	adapter := connection.RuntimeAdapter{Kind: connection.AdapterHTTP}
	facts := runtime.composition.http
	if runtime.ingressHandler == nil || facts == nil {
		return adapter
	}
	adapter.Enabled = true
	adapter.IntegrationID = facts.integrationID
	adapter.Path = runtime.ingressPath
	adapter.AuthMode = facts.authMode
	adapter.ListenAddress = listenAddress
	adapter.MaxBodyBytes = int64Pointer(facts.maxBodyBytes)
	for _, summary := range summaries {
		if summary.IntegrationID != facts.integrationID {
			continue
		}
		adapter.DefinitionID = summary.IntegrationRevision.ArtifactID
		adapter.SourceID = summary.Source.SourceID
		adapter.SourceRevisionID = summary.Source.RevisionID
		adapter.SourceDigest = summary.Source.Digest
	}
	return adapter
}

func describeMLLPAdapter(runtime *previewRuntime) connection.RuntimeAdapter {
	adapter := connection.RuntimeAdapter{Kind: connection.AdapterMLLP}
	facts := runtime.composition.mllp
	if runtime.mllpServer == nil || facts == nil {
		return adapter
	}
	source := facts.source
	adapter.Enabled = true
	adapter.DefinitionID = facts.definitionID
	adapter.SourceID = source.SourceID
	adapter.SourceRevisionID = source.RevisionID
	adapter.SourceDigest = source.Digest
	adapter.ListenAddress = source.ListenAddress
	adapter.TLSMode = string(source.TLS.Mode)
	adapter.MaxConnections = int64Pointer(int64(source.MaxConnections))
	adapter.MaxMessageBytes = int64Pointer(source.MaxMessageBytes)
	adapter.RequireClientIdentity = boolPointer(facts.requireClientIdentity)
	return adapter
}

func describeBatchAdapter(runtime *previewRuntime) connection.RuntimeAdapter {
	adapter := connection.RuntimeAdapter{Kind: connection.AdapterBatch}
	facts := runtime.composition.batch
	if runtime.batchRunner == nil || facts == nil {
		return adapter
	}
	source := facts.source
	adapter.Enabled = true
	adapter.DefinitionID = facts.definitionID
	adapter.SourceID = source.SourceID
	adapter.SourceRevisionID = source.RevisionID
	adapter.SourceDigest = source.Digest
	adapter.Provider = string(source.Provider)
	adapter.PollSeconds = int64Pointer(source.PollSeconds)
	adapter.MaxMessageBytes = int64Pointer(source.MaxMessageBytes)
	adapter.RequireWorkloadIdentity = boolPointer(facts.requireWorkloadIdentity)
	adapter.WorkerID = facts.workerID
	return adapter
}

func describeDeliveryAdapter(runtime *previewRuntime) connection.RuntimeAdapter {
	adapter := connection.RuntimeAdapter{Kind: connection.AdapterDelivery}
	facts := runtime.composition.delivery
	if runtime.deliveryWorker == nil || facts == nil {
		return adapter
	}
	adapter.Enabled = true
	adapter.QueueDriver = facts.queueDriver
	adapter.MaxAttempts = int64Pointer(int64(facts.maxAttempts))
	adapter.WorkerID = facts.workerID
	return adapter
}

func int64Pointer(value int64) *int64 { return &value }

func boolPointer(value bool) *bool { return &value }
