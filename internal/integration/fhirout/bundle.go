package fhirout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
)

// ConditionalReference is the FHIR conditional reference form
// `<Type>?identifier=<system>|<value>`, which a transaction-capable server
// resolves against its own store. It is used both as an entry's request URL
// and as a reference to a resource that is not in the bundle.
func ConditionalReference(resourceType string, key fhir.Identifier) string {
	// FHIR token escaping precedes URL encoding; generated observation keys
	// contain #, which otherwise becomes a URL fragment.
	escape := strings.NewReplacer(`\`, `\\`, `,`, `\,`, `$`, `\$`, `|`, `\|`)
	token := escape.Replace(key.System) + "|" + escape.Replace(key.Value)
	return resourceType + "?identifier=" + url.QueryEscape(token)
}

// CreateConditionalTransactionBundle builds the transaction Bundle a
// `fhir`-transport destination receives.
//
// It differs from pkg/fhir.CreateTransactionBundle in exactly the ways a
// redelivery needs:
//
//   - every entry is a conditional update on the resource's key (see
//     entryRequest), never a POST, so the at-least-once outbox cannot create a
//     second Patient on a lease reclaim;
//   - every entry carries a deterministic fullUrl, and every reference between
//     projected resources points at that fullUrl, so the server resolves them
//     inside the transaction instead of against ids it has never issued;
//   - a reference to a resource outside the bundle is a conditional reference
//     keyed the same way;
//   - the key is guaranteed to be present among the resource's identifiers, so
//     the server's match on the next delivery finds this write;
//   - a mapper-assigned positional id (`obs-1`) is dropped: a conditional
//     update must not carry an id the server did not issue.
//
// The mapper's own output (Resource.Resource) is not modified; the rewrite is
// applied to an encoded copy.
func CreateConditionalTransactionBundle(projection Projection) (*fhir.Bundle, error) {
	if len(projection.Resources) == 0 {
		return nil, fmt.Errorf("%w: projection has no resources", ErrInvalidPayload)
	}
	bundle := &fhir.Bundle{
		ResourceType: "Bundle",
		Type:         "transaction",
		Entry:        make([]fhir.BundleEntry, 0, len(projection.Resources)),
	}
	for _, resource := range projection.Resources {
		if _, err := usableKey(resource.Key); err != nil {
			return nil, fmt.Errorf("%s: %w", resource.Type, err)
		}
		encoded, err := encodeForBundle(resource, projection.references)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", resource.Type, err)
		}
		bundle.Entry = append(bundle.Entry, fhir.BundleEntry{
			FullURL:  resource.FullURL,
			Resource: encoded,
			Request:  entryRequest(resource),
		})
	}
	return bundle, nil
}

// encodeForBundle encodes one resource with its key ensured, its intra-bundle
// references rewritten, and any mapper-assigned id removed. resourceType is
// written first so the encoded resource reads the way every FHIR example does.
func encodeForBundle(resource Resource, references map[string]string) (json.RawMessage, error) {
	raw, err := json.Marshal(resource.Resource)
	if err != nil {
		return nil, fmt.Errorf("%w: encode resource: %w", ErrInvalidPayload, err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("%w: resource is not a JSON object: %w", ErrInvalidPayload, err)
	}
	declared, _ := body["resourceType"].(string)
	if declared != resource.Type {
		return nil, fmt.Errorf("%w: resource declares %q, projection says %q", ErrInvalidPayload, declared, resource.Type)
	}
	delete(body, "id")
	delete(body, "resourceType")
	if resource.Key.System != "" && resource.Key.Value != "" {
		body["identifier"] = ensureIdentifier(body["identifier"], resource.Key)
	}
	rewriteReferences(body, references)

	rest, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: encode resource: %w", ErrInvalidPayload, err)
	}
	var encoded bytes.Buffer
	encoded.WriteString(`{"resourceType":`)
	typeJSON, _ := json.Marshal(resource.Type)
	encoded.Write(typeJSON)
	if len(rest) > 2 {
		encoded.WriteByte(',')
		encoded.Write(rest[1:])
	} else {
		encoded.WriteByte('}')
	}
	return json.RawMessage(encoded.Bytes()), nil
}

// ensureIdentifier makes the key one of the resource's identifiers. A bare
// identifier with the key's value gains the key's system rather than being
// duplicated; a matching one is left alone; otherwise the key is appended.
func ensureIdentifier(existing any, key fhir.Identifier) []any {
	identifiers, _ := existing.([]any)
	for _, raw := range identifiers {
		identifier, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		value, _ := identifier["value"].(string)
		if value != key.Value {
			continue
		}
		system, _ := identifier["system"].(string)
		switch system {
		case key.System:
			return identifiers
		case "":
			identifier["system"] = key.System
			return identifiers
		}
	}
	return append(identifiers, map[string]any{"system": key.System, "value": key.Value})
}

// rewriteReferences replaces every `reference` member whose literal value the
// projection mapped, anywhere in the resource.
func rewriteReferences(value any, references map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "reference" {
				if literal, ok := child.(string); ok {
					if replacement, mapped := references[literal]; mapped {
						typed[key] = replacement
					}
				}
				continue
			}
			rewriteReferences(child, references)
		}
	case []any:
		for _, child := range typed {
			rewriteReferences(child, references)
		}
	}
}
