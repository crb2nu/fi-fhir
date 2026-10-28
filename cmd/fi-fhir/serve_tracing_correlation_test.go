//go:build !tracingleak

package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestSpanCorrelationIDKeyedDigest(t *testing.T) {
	otherKey := []byte("fedcba9876543210fedcba9876543210")
	keyed := regexp.MustCompile(`^hmac-sha256:[0-9a-f]{32}$`)
	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{"same key and header join", func(t *testing.T) {
			if a, b := spanCorrelationID(testCorrelationKey, "MSH10-0001"), spanCorrelationID(testCorrelationKey, "MSH10-0001"); a != b {
				t.Fatalf("same key and header rendered %q and %q", a, b)
			}
		}},
		{"different header differs", func(t *testing.T) {
			if spanCorrelationID(testCorrelationKey, "MSH10-0001") == spanCorrelationID(testCorrelationKey, "MSH10-0002") {
				t.Fatal("different headers rendered the same digest")
			}
		}},
		{"different key differs", func(t *testing.T) {
			if spanCorrelationID(testCorrelationKey, "MSH10-0001") == spanCorrelationID(otherKey, "MSH10-0001") {
				t.Fatal("different keys rendered the same digest")
			}
		}},
		{"keyed form is a truncated hmac", func(t *testing.T) {
			got := spanCorrelationID(testCorrelationKey, "MSH10-0001")
			if !keyed.MatchString(got) {
				t.Fatalf("keyed digest %q is not hmac-sha256:<32 hex>", got)
			}
			// HMAC-SHA256("0123456789abcdef0123456789abcdef", "MSH10-0001"), first 16 bytes.
			if want := "hmac-sha256:ec8e0270db2be345ba1fa2963c6de107"; got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		}},
		{"nil key keeps the unkeyed digest", func(t *testing.T) {
			got := spanCorrelationID(nil, "MSH10-0001")
			if !strings.HasPrefix(got, "sha256:") || len(got) != len("sha256:")+64 {
				t.Fatalf("unkeyed digest %q is not sha256:<64 hex>", got)
			}
		}},
		{"digest never carries the header", func(t *testing.T) {
			for _, key := range [][]byte{nil, testCorrelationKey} {
				if got := spanCorrelationID(key, "PHI_HEADER_SENTINEL"); strings.Contains(got, "PHI_HEADER_SENTINEL") {
					t.Fatalf("digest %q carries the raw header", got)
				}
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}

func TestLoadTelemetryCorrelationKey(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "unset keeps the unkeyed digest", value: ""},
		{name: "whitespace only is unset", value: "  \n"},
		{name: "short key is refused", value: "too-short", wantErr: true},
		{name: "31 bytes is refused", value: strings.Repeat("k", 31), wantErr: true},
		{name: "32 bytes is accepted", value: strings.Repeat("k", 32), want: strings.Repeat("k", 32)},
		{name: "surrounding whitespace is trimmed", value: " " + strings.Repeat("k", 64) + "\n", want: strings.Repeat("k", 64)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envTelemetryCorrelationKey, tt.value)
			key, err := loadTelemetryCorrelationKey()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				if strings.Contains(err.Error(), tt.value) {
					t.Fatalf("error quotes the key: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(key) != tt.want {
				t.Fatalf("key = %q, want %q", key, tt.want)
			}
			if tt.want == "" && key != nil {
				t.Fatal("unset key must be nil so the unkeyed digest is kept")
			}
		})
	}
}
