#!/usr/bin/env bash
# pgdump-roundtrip.sh — Take a logical backup of a fi-fhir database and restore
# it into another one, using the method docs/operations/PRODUCTION-HARDENING.md
# documents for disaster recovery.
#
# This script exists so the documented recovery procedure and the procedure a
# test exercises are the same text. A restore proof that hand-rolls its own
# copy proves something about the test, not about the runbook.
#
# Two things here are not cosmetic:
#
#   * `--no-owner --no-privileges` — a restore into a database owned by a
#     different role must not fail on ownership statements. Recovery commonly
#     lands in a freshly provisioned instance with a new role name.
#   * `psql -v ON_ERROR_STOP=1` — without it psql continues past errors and
#     exits 0. A restore that dropped half the triggers would look like a
#     success. The runbook's original command omitted this; see slice 4.4a.
#
# It also refuses to run on a client/server major-version mismatch, which is a
# real and silent way to produce an unrestorable backup. pg_dump 17 and later
# emit `SET transaction_timeout = 0` in the dump preamble; PostgreSQL 16 has no
# such setting and rejects it. So an operator who runs pg_dump from a
# workstation with newer client tools against a supported PostgreSQL 16 server
# gets a dump that completes successfully, exits 0, and cannot be restored into
# the very server it came from. The failure surfaces during recovery, which is
# the worst possible moment to discover it. Set FI_FHIR_PG_BIN_DIR to point at
# matching client binaries when the default PATH has the wrong ones.
#
# What it does NOT do, stated so nobody mistakes it for more than it is: a
# periodic logical dump cannot bound data loss to the product spec's RPO of 5
# minutes. Anything written between the last dump and the failure is gone.
# Meeting that target needs WAL archiving / point-in-time recovery, which no
# chart or manifest in this repository configures yet. See
# docs/operations/PRODUCTION-HARDENING.md "Recovery objectives, honestly".
#
# Usage:
#   scripts/pgdump-roundtrip.sh --source-url <url> --target-db <name> [--archive <path>]
#
# Exit codes:
#   0 — dump and restore both succeeded
#   1 — bad usage
#   2 — dump or restore failed

set -euo pipefail

SOURCE_URL=""
TARGET_DB=""
ARCHIVE=""

usage() {
	sed -n '2,32p' "$0" | sed 's/^# \{0,1\}//'
}

while [ $# -gt 0 ]; do
	case "$1" in
	--source-url)
		SOURCE_URL="${2:-}"
		shift 2
		;;
	--target-db)
		TARGET_DB="${2:-}"
		shift 2
		;;
	--archive)
		ARCHIVE="${2:-}"
		shift 2
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "pgdump-roundtrip: unknown argument: $1" >&2
		usage >&2
		exit 1
		;;
	esac
done

if [ -z "$SOURCE_URL" ] || [ -z "$TARGET_DB" ]; then
	echo "pgdump-roundtrip: --source-url and --target-db are required" >&2
	usage >&2
	exit 1
fi

# The target database name is interpolated into SQL, so constrain it to an
# identifier rather than quoting after the fact.
if ! printf '%s' "$TARGET_DB" | grep -Eq '^[a-z_][a-z0-9_]*$'; then
	echo "pgdump-roundtrip: --target-db must be a lowercase SQL identifier, got: $TARGET_DB" >&2
	exit 1
fi

# FI_FHIR_PG_BIN_DIR lets a caller select client binaries whose major version
# matches the server when the ones first on PATH do not.
PG_BIN_DIR="${FI_FHIR_PG_BIN_DIR:-}"
PG_DUMP="pg_dump"
PSQL="psql"
if [ -n "$PG_BIN_DIR" ]; then
	PG_DUMP="${PG_BIN_DIR}/pg_dump"
	PSQL="${PG_BIN_DIR}/psql"
fi

for binary in "$PG_DUMP" "$PSQL"; do
	command -v "$binary" >/dev/null 2>&1 || {
		echo "pgdump-roundtrip: $binary not found; install the PostgreSQL client tools matching the server major version" >&2
		exit 2
	}
done

# Derive the maintenance and target URLs from the source by swapping the
# database path segment. Everything else — host, port, credentials, sslmode —
# is preserved exactly, so the restore lands on the same server the operator
# already authenticated against.
url_without_query="${SOURCE_URL%%\?*}"
query=""
if [ "$url_without_query" != "$SOURCE_URL" ]; then
	query="?${SOURCE_URL#*\?}"
fi
url_prefix="${url_without_query%/*}"
ADMIN_URL="${url_prefix}/postgres${query}"
TARGET_URL="${url_prefix}/${TARGET_DB}${query}"

# Refuse a client/server major mismatch rather than producing an unrestorable
# archive. See the header for why this is not pedantry.
SERVER_MAJOR="$("$PSQL" -tAX "$SOURCE_URL" -c 'SHOW server_version_num' 2>/dev/null | head -1)"
if [ -z "$SERVER_MAJOR" ]; then
	echo "pgdump-roundtrip: could not read the server version from --source-url" >&2
	exit 2
fi
SERVER_MAJOR=$((SERVER_MAJOR / 10000))
CLIENT_MAJOR="$("$PG_DUMP" --version | sed -E 's/^pg_dump \(PostgreSQL\) ([0-9]+).*/\1/')"

if [ "$CLIENT_MAJOR" != "$SERVER_MAJOR" ]; then
	echo "pgdump-roundtrip: client/server major version mismatch: pg_dump ${CLIENT_MAJOR} against PostgreSQL ${SERVER_MAJOR}." >&2
	echo "  A newer pg_dump writes SET commands an older server rejects (transaction_timeout, added in 17)," >&2
	echo "  so the dump succeeds and the restore fails. An older pg_dump refuses to read a newer server." >&2
	echo "  Use client tools matching the server, or set FI_FHIR_PG_BIN_DIR to a directory that has them." >&2
	exit 2
fi

CLEANUP_ARCHIVE=0
if [ -z "$ARCHIVE" ]; then
	ARCHIVE="$(mktemp -t fi-fhir-dump.XXXXXX).sql.gz"
	CLEANUP_ARCHIVE=1
fi
cleanup() {
	[ "$CLEANUP_ARCHIVE" -eq 1 ] && rm -f "$ARCHIVE"
	return 0
}
trap cleanup EXIT

echo "pgdump-roundtrip: dumping to ${ARCHIVE}"
if ! "$PG_DUMP" --no-owner --no-privileges "$SOURCE_URL" | gzip >"$ARCHIVE"; then
	echo "pgdump-roundtrip: pg_dump failed" >&2
	exit 2
fi

echo "pgdump-roundtrip: recreating target database ${TARGET_DB}"
if ! "$PSQL" -v ON_ERROR_STOP=1 -q "$ADMIN_URL" \
	-c "DROP DATABASE IF EXISTS ${TARGET_DB} WITH (FORCE)" \
	-c "CREATE DATABASE ${TARGET_DB}"; then
	echo "pgdump-roundtrip: could not recreate ${TARGET_DB}" >&2
	exit 2
fi

echo "pgdump-roundtrip: restoring into ${TARGET_DB}"
if ! gunzip -c "$ARCHIVE" | "$PSQL" -v ON_ERROR_STOP=1 -q "$TARGET_URL" >/dev/null; then
	echo "pgdump-roundtrip: restore failed" >&2
	exit 2
fi

echo "pgdump-roundtrip: restored ${TARGET_DB} from ${ARCHIVE}"
