package connection

import (
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// CheckSecretBindingReferences is the catalog's own reference check — the
// count (MaxSecretBindings), each name, provider, key and version token, and
// that none of them carries certificate or key material — exported for the
// definition editor (.loom/42 E-1), whose revisions are append-only too.
// Problem paths are `secret_bindings[i].<field>`; no message repeats a value.
func CheckSecretBindingReferences(bindings []integration.SecretBinding) []Problem {
	c := &checker{}
	checkBindingReferences(bindings, c)
	return c.problems
}

// CheckSecretReference checks one bare reference (an encryption key, say) at
// path with the same rules, without a binding name.
func CheckSecretReference(path string, reference integration.SecretReference) []Problem {
	c := &checker{}
	for _, field := range []struct{ name, value string }{
		{"key", reference.Key}, {"version", reference.Version},
	} {
		if strings.Contains(field.value, pemMarker) {
			c.add(CodeSecretValueForbidden, path+"."+field.name,
				"a reference names a secret; it never carries certificate or key material")
		}
	}
	switch reference.Provider {
	case integration.SecretProviderEnvironment, integration.SecretProviderFile, integration.SecretProviderVault,
		integration.SecretProviderAWSSSM, integration.SecretProviderKubernetes:
	case "":
		c.add(CodeRequired, path+".provider", "provider is required")
	default:
		c.add(CodeInvalidEnum, path+".provider", "provider must be env, file, vault, aws-ssm, or k8s")
	}
	switch {
	case reference.Key == "":
		c.add(CodeRequired, path+".key", "key is required")
	case !validSecretToken(reference.Key):
		c.add(CodeInvalidValue, path+".key", "key must be at most 256 characters with no whitespace")
	}
	if reference.Version != "" && !validSecretToken(reference.Version) {
		c.add(CodeInvalidValue, path+".version", "version must be at most 256 characters with no whitespace")
	}
	return c.problems
}
