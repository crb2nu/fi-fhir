package eventsourcing

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	"github.com/lib/pq"
)

const (
	postgresIdentifierMaxBytes  = 63
	postgresIdentifierHashBytes = 12
)

func quotePostgresIdentifier(identifier string) string {
	return pq.QuoteIdentifier(normalizePostgresIdentifier(identifier))
}

func quotePostgresIndexIdentifier(tableName, suffix string) string {
	return quotePostgresIdentifier("idx_" + tableName + "_" + suffix)
}

// normalizePostgresIdentifier preserves ordinary identifiers and gives names
// containing NUL or exceeding PostgreSQL's length limit a stable hash suffix.
// This avoids both lib/pq's NUL truncation and PostgreSQL's silent truncation
// of names that share a long prefix.
func normalizePostgresIdentifier(identifier string) string {
	if len(identifier) <= postgresIdentifierMaxBytes && !strings.ContainsRune(identifier, '\x00') {
		return identifier
	}

	digest := sha256.Sum256([]byte(identifier))
	hash := hex.EncodeToString(digest[:postgresIdentifierHashBytes])
	prefixBytes := postgresIdentifierMaxBytes - 1 - len(hash)
	prefixSource := strings.ReplaceAll(identifier, "\x00", "_")
	if len(prefixSource) > prefixBytes {
		prefixSource = prefixSource[:prefixBytes]
	}
	prefix := prefixSource
	for !utf8.ValidString(prefix) {
		prefix = prefix[:len(prefix)-1]
	}

	return prefix + "_" + hash
}
