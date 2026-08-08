package operator

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func TestCursorRoundTripPreservesSortKey(t *testing.T) {
	when := time.Date(2026, 8, 8, 12, 30, 45, 123456789, time.UTC)
	cursor := encodeCursor(when, "receipt-a")
	gotTime, gotID, err := decodeCursor(cursor)
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if !gotTime.Equal(when) || gotID != "receipt-a" {
		t.Fatalf("cursor round trip = %v/%q, want %v/%q", gotTime, gotID, when, "receipt-a")
	}
}

func TestDecodeCursorRejectsForeignAndMalformedValues(t *testing.T) {
	for name, cursor := range map[string]string{
		"not base64":     "!!!!",
		"foreign domain": "Zm9yZWlnbjpwYXlsb2Fk",
		"missing id":     encodeCursorRaw("2026-08-08T00:00:00Z"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := decodeCursor(cursor); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("decodeCursor error = %v, want %v", err, ErrInvalidRequest)
			}
		})
	}
}

func TestNormalizePageClampsCallerRequestedSize(t *testing.T) {
	for name, tc := range map[string]struct {
		first int
		want  int
	}{
		"omitted":  {first: 0, want: DefaultPageSize},
		"negative": {first: -10, want: DefaultPageSize},
		"in range": {first: 10, want: 10},
		"oversize": {first: 10_000, want: MaxPageSize},
	} {
		t.Run(name, func(t *testing.T) {
			size, _, _, err := normalizePage(PageRequest{First: tc.first})
			if err != nil {
				t.Fatalf("normalizePage: %v", err)
			}
			if size != tc.want {
				t.Fatalf("page size = %d, want %d", size, tc.want)
			}
		})
	}
}

func TestPaginateEmitsCursorOnlyWhenMoreRemains(t *testing.T) {
	key := func(value string) (time.Time, string) {
		return time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC), value
	}

	full := paginate([]string{"a", "b", "c"}, 2, key)
	if len(full.Items) != 2 || !full.HasMore || full.NextCursor == "" {
		t.Fatalf("overfetched page = %#v", full)
	}
	if _, id, err := decodeCursor(full.NextCursor); err != nil || id != "b" {
		t.Fatalf("next cursor id = %q error=%v, want %q", id, err, "b")
	}

	partial := paginate([]string{"a"}, 2, key)
	if len(partial.Items) != 1 || partial.HasMore || partial.NextCursor != "" {
		t.Fatalf("final page = %#v", partial)
	}
}

func TestFilterValidationRejectsUnknownStatusAndInvertedWindow(t *testing.T) {
	from := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	to := from.Add(-time.Hour)

	if err := (ReceiptFilter{Status: "deleted"}).validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("receipt status error = %v, want %v", err, ErrInvalidRequest)
	}
	if err := (ReceiptFilter{From: &from, To: &to}).validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("receipt window error = %v, want %v", err, ErrInvalidRequest)
	}
	if err := (AttemptFilter{Status: "retrying"}).validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("attempt status error = %v, want %v", err, ErrInvalidRequest)
	}
	if err := (AttemptFilter{Status: "failed"}).validate(); err != nil {
		t.Fatalf("valid attempt filter error = %v", err)
	}
	if err := (ReceiptFilter{}).validate(); err != nil {
		t.Fatalf("empty receipt filter error = %v", err)
	}
}

func encodeCursorRaw(value string) string {
	return base64RawURL(cursorDomain + value)
}

func base64RawURL(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}
