package mllp

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestSourceRevisionRoundTripAndContentAddressing(t *testing.T) {
	revision := testSource(t)
	encoded, err := json.Marshal(revision)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSourceRevision(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode source: %v", err)
	}
	if decoded.Digest != revision.Digest || decoded.Reference() != revision.Reference() {
		t.Fatalf("round trip changed identity: %#v", decoded)
	}

	reordered := revision
	reordered.Clients.AllowedCIDRs = []string{"10.0.0.0/8", "127.0.0.0/8"}
	first, err := reordered.semanticDigest()
	if err != nil {
		t.Fatal(err)
	}
	reordered.Clients.AllowedCIDRs = []string{"127.0.0.0/8", "10.0.0.0/8"}
	second, err := reordered.semanticDigest()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("CIDR order must not change the semantic digest")
	}
}

func TestDecodeSourceRevisionRejectsNonCanonicalOrTamperedDocuments(t *testing.T) {
	revision := testSource(t)
	encoded, _ := json.Marshal(revision)
	cases := map[string][]byte{
		"unknown field":  bytes.Replace(encoded, []byte(`"digest"`), []byte(`"unexpected":true,"digest"`), 1),
		"duplicate key":  bytes.Replace(encoded, []byte(`"source_id":"hospital-a"`), []byte(`"source_id":"hospital-a","source_id":"hospital-b"`), 1),
		"trailing value": append(append([]byte(nil), encoded...), []byte(` {}`)...),
		"tampered":       bytes.Replace(encoded, []byte(`"max_connections":4`), []byte(`"max_connections":5`), 1),
		"empty":          nil,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeSourceRevision(bytes.NewReader(raw)); !errors.Is(err, ErrInvalidSourceRevision) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestSourceRevisionValidationBoundaries(t *testing.T) {
	base := SourceRevisionInput{
		ArtifactID: "source", RevisionID: "v1", SourceID: "sender", ListenAddress: "127.0.0.1:2575", Encoding: "utf-8",
		Framing: FramingPolicy{11, 28, 13}, Timeouts: TimeoutPolicy{1, 1, 1, 1}, TLS: TLSPolicy{Mode: TLSModeDisabled},
		Clients: ClientPolicy{AllowedCIDRs: []string{"127.0.0.0/8"}}, Acknowledgements: AcknowledgementPolicy{Mode: AcknowledgementModeApplication},
		MaxMessageBytes: 1, MaxConnections: 1,
	}
	cases := map[string]func(*SourceRevisionInput){
		"encoding":             func(v *SourceRevisionInput) { v.Encoding = "utf-16" },
		"framing collision":    func(v *SourceRevisionInput) { v.Framing.EndByte = v.Framing.StartByte },
		"non canonical CIDR":   func(v *SourceRevisionInput) { v.Clients.AllowedCIDRs = []string{"127.0.0.1/8"} },
		"zero port":            func(v *SourceRevisionInput) { v.ListenAddress = "127.0.0.1:0" },
		"missing TLS bindings": func(v *SourceRevisionInput) { v.TLS = TLSPolicy{Mode: TLSModeMutual} },
		"invalid ACK mode":     func(v *SourceRevisionInput) { v.Acknowledgements.Mode = "enhanced" },
		"oversize":             func(v *SourceRevisionInput) { v.MaxMessageBytes = maxMLLPMessageBytes + 1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			input := base
			mutate(&input)
			if _, err := NewSourceRevision(input); !errors.Is(err, ErrInvalidSourceRevision) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestSourceRevisionValidateAgainstExactBinding(t *testing.T) {
	source := testSource(t)
	binding := testBinding(source)
	if err := source.ValidateAgainst(binding); err != nil {
		t.Fatalf("expected matching binding: %v", err)
	}

	mismatched := binding
	mismatched.IntegrationRevision = integration.ArtifactRevisionRef{}
	if err := source.ValidateAgainst(mismatched); !errors.Is(err, ErrSourceMismatch) {
		t.Fatalf("got %v", err)
	}
}

func TestCheckedInMLLPSourceRevision(t *testing.T) {
	file, err := os.Open(filepath.Join("..", "..", "..", "testdata", "golden", "integration", "adt-mllp", "source-revision.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	revision, err := DecodeSourceRevision(file)
	if err != nil {
		t.Fatalf("checked-in source revision: %v", err)
	}
	if revision.Digest != "sha256:1d9517bcdec7c6617fcd2191777db477cf27adc777a27a260ca552b3320a6399" {
		t.Fatalf("source digest = %s", revision.Digest)
	}
}
