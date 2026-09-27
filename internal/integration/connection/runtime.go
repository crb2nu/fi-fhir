package connection

import (
	"errors"
	"fmt"
	"net/url"
)

// The engine runtime description (.loom/38, "Engine properties").
//
// `serve` composes its adapters from the environment once, at startup, and
// only cmd/fi-fhir knows what it mounted. This is the PHI-free, secret-free
// record of that composition: cmd/ fills it at the end of runServe, after every
// adapter is decided, and the resolver returns it. Nothing here reads the
// environment. Properties are a closed allowlist cmd/ owns, and a secret
// property's value is rendered only as "set" or "unset" (NewRuntimeProperty).

// Roles a mounted revision can play on this replica.
const (
	RuntimeRoleMLLPListener     = "mllp-listener"
	RuntimeRoleBatchRunner      = "batch-runner"
	RuntimeRoleHTTPIngress      = "http-ingress"
	RuntimeRoleDeliveryRegistry = "delivery-registry"
)

// Adapter kinds, in the fixed order RuntimeDescription.Adapters holds them.
const (
	AdapterHTTP     = "http"
	AdapterMLLP     = "mllp"
	AdapterBatch    = "batch"
	AdapterDelivery = "delivery"
)

// AdapterOrder is the one order the four adapter rows are reported in.
var AdapterOrder = [4]string{AdapterHTTP, AdapterMLLP, AdapterBatch, AdapterDelivery}

// Property sources and the two values a secret property may take.
const (
	PropertySourceEnv     = "env"
	PropertySourceDefault = "default"
	PropertyValueSet      = "set"
	PropertyValueUnset    = "unset"
)

// maxPropertyValueBytes bounds one rendered property value.
const maxPropertyValueBytes = 512

// RuntimeDescription is what this replica composed at startup.
type RuntimeDescription struct {
	Version             string
	TenantID            string
	ReplicaID           string
	AuthMode            string
	TrustedNetwork      bool
	AccessIdentity      bool
	ControlPlane        bool
	IntegrationSessions bool
	Streaming           bool
	RetentionPurge      bool
	LLMConfigured       bool
	Registry            RuntimeRegistry
	// Adapters holds exactly four rows, enabled or not, in AdapterOrder.
	Adapters [4]RuntimeAdapter
	// DestinationIdentity is nil when no delivery identity registry is loaded.
	DestinationIdentity *RuntimeDestinationIdentity
	Ledgers             []RuntimeLedger
	Properties          []RuntimeProperty
}

// RuntimeRegistry is the static integration registry this replica loaded.
type RuntimeRegistry struct {
	IntegrationCount int
	Integrations     []RuntimeRegistryIntegration
}

// RuntimeRegistryIntegration is one registry binding: the public integration
// ID and the exact definition revision it resolves to.
type RuntimeRegistryIntegration struct {
	IntegrationID string
	DefinitionID  string
	RevisionID    string
	Digest        string
	SourceID      string
	Format        string
}

// RuntimeAdapter is one ingress or egress adapter. Every field but Kind and
// Enabled is empty (nil) when the adapter is disabled or the field does not
// apply to its kind; a reader renders that as absent, never as a default.
type RuntimeAdapter struct {
	Kind                    string
	Enabled                 bool
	DefinitionID            string
	IntegrationID           string
	SourceID                string
	SourceRevisionID        string
	SourceDigest            string
	ListenAddress           string
	Path                    string
	AuthMode                string
	TLSMode                 string
	Provider                string
	PollSeconds             *int64
	MaxConnections          *int64
	MaxMessageBytes         *int64
	MaxBodyBytes            *int64
	RequireClientIdentity   *bool
	RequireWorkloadIdentity *bool
	QueueDriver             string
	MaxAttempts             *int64
	WorkerID                string
}

// RuntimeDestinationIdentity is the loaded delivery identity registry.
type RuntimeDestinationIdentity struct {
	Mode         string
	Destinations []RuntimeDestination
}

// RuntimeDestination is one destination revision in the delivery registry.
// EndpointAdvisory has no credentials, query, or fragment (EndpointAdvisory).
type RuntimeDestination struct {
	ArtifactID       string
	RevisionID       string
	Digest           string
	Transport        string
	Class            string
	EndpointAdvisory string
}

// RuntimeLedger is one forward-only migration ledger and the version this
// binary expects of it.
type RuntimeLedger struct {
	Name    string
	Version int
}

// RuntimeProperty is one allowlisted process property.
type RuntimeProperty struct {
	Key    string
	Value  string
	Secret bool
	Source string
}

// NewRuntimeProperty renders one allowlisted property. present reports
// whether the environment set it to a non-empty value. A secret property's
// value is "set" or "unset" and nothing else; a non-secret property shows the
// value it has, or its documented default ("" when it has none), and never
// shows credentials embedded in a URL.
func NewRuntimeProperty(key string, secret bool, value string, present bool, defaultValue string) RuntimeProperty {
	property := RuntimeProperty{Key: key, Secret: secret, Source: PropertySourceDefault}
	if present {
		property.Source = PropertySourceEnv
	}
	switch {
	case secret && present:
		property.Value = PropertyValueSet
	case secret:
		property.Value = PropertyValueUnset
	case present:
		property.Value = boundedPropertyValue(stripURLCredentials(value))
	default:
		property.Value = boundedPropertyValue(defaultValue)
	}
	return property
}

func boundedPropertyValue(value string) string {
	if len(value) <= maxPropertyValueBytes {
		return value
	}
	return value[:maxPropertyValueBytes] + "…"
}

// stripURLCredentials removes userinfo from a value that parses as an
// absolute URL. A documented setting never needs credentials inline, and a
// property value must not be the way one leaks.
func stripURLCredentials(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User == nil {
		return value
	}
	parsed.User = nil
	return parsed.String()
}

// EndpointAdvisory reduces a destination's declared endpoint to scheme, host,
// and path. A Kafka topic, which is not a URL, is returned unchanged.
func EndpointAdvisory(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsed.String()
}

// Validate refuses a description that breaks its own contract: four adapters
// in AdapterOrder, and no secret property carrying anything but set/unset.
func (d *RuntimeDescription) Validate() error {
	if d == nil {
		return errors.New("engine runtime description is required")
	}
	for index, adapter := range d.Adapters {
		if adapter.Kind != AdapterOrder[index] {
			return fmt.Errorf("engine runtime adapter %d is %q, want %q", index, adapter.Kind, AdapterOrder[index])
		}
	}
	seen := make(map[string]struct{}, len(d.Properties))
	for _, property := range d.Properties {
		if _, duplicate := seen[property.Key]; duplicate {
			return fmt.Errorf("engine runtime property %q is repeated", property.Key)
		}
		seen[property.Key] = struct{}{}
		if property.Secret && property.Value != PropertyValueSet && property.Value != PropertyValueUnset {
			return fmt.Errorf("engine runtime property %q is secret and must render as set or unset", property.Key)
		}
		if property.Source != PropertySourceEnv && property.Source != PropertySourceDefault {
			return fmt.Errorf("engine runtime property %q has source %q", property.Key, property.Source)
		}
	}
	if d.Registry.IntegrationCount != len(d.Registry.Integrations) {
		return errors.New("engine runtime registry count does not match its integrations")
	}
	return nil
}

// Clone returns a deep copy, so a reader can never mutate the description the
// process composed.
func (d *RuntimeDescription) Clone() RuntimeDescription {
	if d == nil {
		return RuntimeDescription{}
	}
	clone := *d
	clone.Registry.Integrations = append([]RuntimeRegistryIntegration(nil), d.Registry.Integrations...)
	for index := range clone.Adapters {
		clone.Adapters[index] = cloneAdapter(d.Adapters[index])
	}
	if d.DestinationIdentity != nil {
		identity := *d.DestinationIdentity
		identity.Destinations = append([]RuntimeDestination(nil), d.DestinationIdentity.Destinations...)
		clone.DestinationIdentity = &identity
	}
	clone.Ledgers = append([]RuntimeLedger(nil), d.Ledgers...)
	clone.Properties = append([]RuntimeProperty(nil), d.Properties...)
	return clone
}

func cloneAdapter(adapter RuntimeAdapter) RuntimeAdapter {
	clone := adapter
	clone.PollSeconds = cloneInt64(adapter.PollSeconds)
	clone.MaxConnections = cloneInt64(adapter.MaxConnections)
	clone.MaxMessageBytes = cloneInt64(adapter.MaxMessageBytes)
	clone.MaxBodyBytes = cloneInt64(adapter.MaxBodyBytes)
	clone.MaxAttempts = cloneInt64(adapter.MaxAttempts)
	clone.RequireClientIdentity = cloneBool(adapter.RequireClientIdentity)
	clone.RequireWorkloadIdentity = cloneBool(adapter.RequireWorkloadIdentity)
	return clone
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

// MountedDigest says what runs a mounted document on this replica.
type MountedDigest struct {
	Role   string
	Detail string
}

// MountedDigests maps every document digest this replica runs to the role
// that runs it: the MLLP listener's and batch runner's source revisions, the
// source digest named by the definition the HTTP ingress is bound to, and
// every destination revision in the delivery identity registry.
func (d *RuntimeDescription) MountedDigests() map[string]MountedDigest {
	mounted := make(map[string]MountedDigest)
	if d == nil {
		return mounted
	}
	for _, adapter := range d.Adapters {
		if !adapter.Enabled || adapter.SourceDigest == "" {
			continue
		}
		switch adapter.Kind {
		case AdapterMLLP:
			mounted[adapter.SourceDigest] = MountedDigest{
				Role:   RuntimeRoleMLLPListener,
				Detail: fmt.Sprintf("MLLP listener on %s for definition %s", adapter.ListenAddress, adapter.DefinitionID),
			}
		case AdapterBatch:
			mounted[adapter.SourceDigest] = MountedDigest{
				Role:   RuntimeRoleBatchRunner,
				Detail: fmt.Sprintf("%s batch runner for definition %s", adapter.Provider, adapter.DefinitionID),
			}
		case AdapterHTTP:
			mounted[adapter.SourceDigest] = MountedDigest{
				Role: RuntimeRoleHTTPIngress,
				Detail: fmt.Sprintf("HTTP ingress %s bound to integration %s (definition %s)",
					adapter.Path, adapter.IntegrationID, adapter.DefinitionID),
			}
		}
	}
	if d.DestinationIdentity != nil {
		for _, destination := range d.DestinationIdentity.Destinations {
			if destination.Digest == "" {
				continue
			}
			mounted[destination.Digest] = MountedDigest{
				Role: RuntimeRoleDeliveryRegistry,
				Detail: fmt.Sprintf("delivery identity registry (%s mode) destination %s",
					d.DestinationIdentity.Mode, destination.ArtifactID),
			}
		}
	}
	return mounted
}

// RevisionDigest is the part of a revision the runtime and reference
// projections need: which revision, and its digest.
type RevisionDigest struct {
	ArtifactID string
	RevisionID string
	Number     int64
	Digest     string
}

// runtimeStateFor reports the newest of a connection's revisions that this
// replica mounts. revisions must be ordered newest first.
func runtimeStateFor(revisions []RevisionDigest, mounted map[string]MountedDigest) RuntimeState {
	for _, revision := range revisions {
		if role, ok := mounted[revision.Digest]; ok {
			return RuntimeState{
				Mounted: true,
				Role:    role.Role,
				Detail:  fmt.Sprintf("revision %s: %s", revision.RevisionID, role.Detail),
			}
		}
	}
	return RuntimeState{}
}
