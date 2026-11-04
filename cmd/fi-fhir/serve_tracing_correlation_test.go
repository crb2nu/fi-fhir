//go:build !tracingleak

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
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
	raw := bytes.Repeat([]byte{0xfb, 0xff}, 16)
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"hex", hex.EncodeToString(raw), false},
		{"standard padded", base64.StdEncoding.EncodeToString(raw), false},
		{"standard unpadded", base64.RawStdEncoding.EncodeToString(raw), false},
		{"URL padded", base64.URLEncoding.EncodeToString(raw), false},
		{"URL unpadded", base64.RawURLEncoding.EncodeToString(raw), false},
		{"trim whitespace", "  " + hex.EncodeToString(raw) + "\n", false},
		{"passphrase", strings.Repeat("k", 32), true},
		{"invalid encoding", strings.Repeat("!", 32), true},
		{"31 decoded bytes", hex.EncodeToString(raw[:31]), true},
	}
	for _, tt := range tests {
		for _, file := range []bool{false, true} {
			name := tt.name + "/env"
			if file {
				name = tt.name + "/file"
			}
			t.Run(name, func(t *testing.T) {
				t.Setenv(envTelemetryCorrelationKey, "")
				t.Setenv(envTelemetryCorrelationKeyFile, "")
				if file {
					path := filepath.Join(t.TempDir(), "key")
					if err := os.WriteFile(path, []byte(strings.TrimRight(tt.value, "\r\n")+"\n"), 0600); err != nil {
						t.Fatal(err)
					}
					t.Setenv(envTelemetryCorrelationKeyFile, path)
				} else {
					t.Setenv(envTelemetryCorrelationKey, tt.value)
				}
				key, err := loadTelemetryCorrelationKey()
				if tt.wantErr {
					if err == nil {
						t.Fatal("expected an error")
					}
					if strings.Contains(err.Error(), tt.value) {
						t.Fatal("error exposes key")
					}
					if !strings.Contains(err.Error(), "hex or base64 encoding of at least 32 random bytes") {
						t.Fatalf("missing format guidance: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(key, raw) {
					t.Fatal("key does not match decoded bytes")
				}
			})
		}
	}
	for _, value := range []string{"", "  \n"} {
		t.Run("unset "+value, func(t *testing.T) {
			t.Setenv(envTelemetryCorrelationKey, value)
			t.Setenv(envTelemetryCorrelationKeyFile, "")
			key, err := loadTelemetryCorrelationKey()
			if err != nil || key != nil {
				t.Fatalf("unset key = %v, %v", key, err)
			}
		})
	}
	t.Run("both sources", func(t *testing.T) {
		value := hex.EncodeToString(raw)
		path := filepath.Join(t.TempDir(), "secret-path")
		t.Setenv(envTelemetryCorrelationKey, value)
		t.Setenv(envTelemetryCorrelationKeyFile, path)
		_, err := loadTelemetryCorrelationKey()
		if err == nil || !strings.Contains(err.Error(), "configure exactly one") {
			t.Fatalf("got %v", err)
		}
		if strings.Contains(err.Error(), value) || strings.Contains(err.Error(), path) {
			t.Fatal("error exposes source values")
		}
	})
	for _, content := range []string{"missing", "", "\n", "   \n"} {
		t.Run("invalid file "+content, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "key")
			if content != "missing" {
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv(envTelemetryCorrelationKey, "")
			t.Setenv(envTelemetryCorrelationKeyFile, path)
			if _, err := loadTelemetryCorrelationKey(); err == nil {
				t.Fatal("expected file error")
			}
		})
	}
}
