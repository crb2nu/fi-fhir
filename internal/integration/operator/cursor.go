package operator

import (
	"encoding/base64"
	"strings"
	"time"
	"unicode"
)

const cursorDomain = "fi-fhir/operator-cursor/v1:"

// encodeCursor produces an opaque, stable forward cursor for keyset paging.
// The cursor is the exact sort key of the last returned row, so a page never
// skips or repeats a record when new rows arrive between requests.
func encodeCursor(sortTime time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString(
		[]byte(cursorDomain + sortTime.UTC().Format(time.RFC3339Nano) + "\x00" + id),
	)
}

func decodeCursor(cursor string) (time.Time, string, error) {
	if cursor == "" {
		return time.Time{}, "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", ErrInvalidRequest
	}
	decoded := string(raw)
	if !strings.HasPrefix(decoded, cursorDomain) {
		return time.Time{}, "", ErrInvalidRequest
	}
	parts := strings.SplitN(strings.TrimPrefix(decoded, cursorDomain), "\x00", 2)
	if len(parts) != 2 || !validToken(parts[1], 256) {
		return time.Time{}, "", ErrInvalidRequest
	}
	sortTime, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", ErrInvalidRequest
	}
	return sortTime.UTC(), parts[1], nil
}

// normalizePage clamps a caller page request to the server-owned bounds and
// resolves its cursor. A caller can never widen the window.
func normalizePage(request PageRequest) (int, time.Time, string, error) {
	size := request.First
	switch {
	case size <= 0:
		size = DefaultPageSize
	case size > MaxPageSize:
		size = MaxPageSize
	}
	cursorTime, cursorID, err := decodeCursor(request.Cursor)
	if err != nil {
		return 0, time.Time{}, "", err
	}
	return size, cursorTime, cursorID, nil
}

func validToken(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func optionalToken(value string, maxBytes int) bool {
	return value == "" || validToken(value, maxBytes)
}

func optionalTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}
