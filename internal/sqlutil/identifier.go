// Package sqlutil contains shared safeguards for SQL assembled from trusted
// configuration. Values still belong in query parameters; identifiers cannot
// be parameterized and must pass a strict allowlist before they are quoted.
package sqlutil

import (
	"errors"
	"regexp"
)

var postgresIdentifierPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// ValidatePostgresIdentifier accepts one unqualified, lowercase PostgreSQL
// identifier. PostgreSQL limits identifiers to 63 bytes and folds unquoted
// names to lowercase, so the allowlist avoids both injection and case drift.
func ValidatePostgresIdentifier(identifier string) error {
	if !postgresIdentifierPattern.MatchString(identifier) {
		return errors.New("must be 1-63 lowercase letters, digits, or underscores and must not start with a digit")
	}
	return nil
}
