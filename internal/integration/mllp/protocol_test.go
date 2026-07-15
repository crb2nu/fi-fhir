package mllp

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"testing"
	"time"
)

type oneByteReader struct{ reader io.Reader }

type timeoutReader struct{}

func (timeoutReader) Read([]byte) (int, error) { return 0, timeoutError{} }

type timeoutError struct{}

func (timeoutError) Error() string   { return "deadline exceeded" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func (r oneByteReader) Read(buffer []byte) (int, error) {
	if len(buffer) > 1 {
		buffer = buffer[:1]
	}
	return r.reader.Read(buffer)
}

func TestReadFrameHandlesArbitraryPacketSplitsAndMultipleFrames(t *testing.T) {
	source := testSource(t)
	first, _ := framePayload(testHL7("CTRL1"), source.Framing)
	second, _ := framePayload(testHL7("CTRL2"), source.Framing)
	reader := bufio.NewReader(oneByteReader{reader: bytes.NewReader(append(first, second...))})
	for index, expected := range [][]byte{testHL7("CTRL1"), testHL7("CTRL2")} {
		actual, err := readFrame(reader, source.Framing, source.MaxMessageBytes)
		if err != nil {
			t.Fatalf("frame %d: %v", index, err)
		}
		if !bytes.Equal(actual, expected) {
			t.Fatalf("frame %d changed payload", index)
		}
	}
}

func TestReadFrameRejectsMalformedAndOversizedInput(t *testing.T) {
	policy := FramingPolicy{11, 28, 13}
	cases := []struct {
		name string
		raw  []byte
		max  int64
		want error
	}{
		{"wrong start", []byte{1, 'x', 28, 13}, 10, ErrFrameInvalid},
		{"nested start", []byte{11, 'x', 11, 28, 13}, 10, ErrFrameInvalid},
		{"wrong trailer", []byte{11, 'x', 28, 10}, 10, ErrFrameInvalid},
		{"empty", []byte{11, 28, 13}, 10, ErrFrameInvalid},
		{"oversized", []byte{11, 'x', 'y', 28, 13}, 1, ErrFrameTooLarge},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := readFrame(bufio.NewReader(bytes.NewReader(test.raw)), policy, test.max)
			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestReadFrameClassifiesNetworkTimeout(t *testing.T) {
	_, err := readFrame(bufio.NewReader(timeoutReader{}), FramingPolicy{11, 28, 13}, 10)
	if !errors.Is(err, ErrFrameTimeout) {
		t.Fatalf("got %v", err)
	}
}

func TestParseHeaderAndBuildSafeAcknowledgements(t *testing.T) {
	source := testSource(t)
	header, err := parseMessageHeader(testHL7("CTRL-123"), source.Framing)
	if err != nil {
		t.Fatal(err)
	}
	if header.controlID != "CTRL-123" || header.sendingApplication != "SEND" || header.receivingApplication != "RECV" {
		t.Fatalf("unexpected header: %#v", header)
	}
	for _, test := range []struct {
		mode    AcknowledgementMode
		outcome acknowledgementOutcome
		want    string
	}{
		{AcknowledgementModeApplication, acknowledgementAccepted, "AA"},
		{AcknowledgementModeApplication, acknowledgementTransientError, "AE"},
		{AcknowledgementModeApplication, acknowledgementPermanentReject, "AR"},
		{AcknowledgementModeCommit, acknowledgementAccepted, "CA"},
		{AcknowledgementModeCommit, acknowledgementTransientError, "CE"},
		{AcknowledgementModeCommit, acknowledgementPermanentReject, "CR"},
	} {
		policy := AcknowledgementPolicy{Mode: test.mode, IncludeErrorSegment: true}
		payload, err := buildAcknowledgement(header, policy, test.outcome, "SUBMISSION_UNAVAILABLE", time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), "ACK-1")
		if err != nil {
			t.Fatal(err)
		}
		if got := acknowledgementCodeFromPayload(t, payload); got != test.want {
			t.Fatalf("got %s, want %s", got, test.want)
		}
		if bytes.Contains(payload, []byte("PID|")) || bytes.Contains(payload, []byte("12345")) {
			t.Fatalf("ACK leaked source payload: %q", payload)
		}
	}
}

func TestParseHeaderRejectsUnsafeControlIdentifiers(t *testing.T) {
	source := testSource(t)
	for _, payload := range [][]byte{
		[]byte("PID|1\r"),
		[]byte("MSH|^~\\&|SEND|FAC|RECV|RFAC|20260715120000||ADT^A01|BAD^ID|P|2.5\r"),
		[]byte("MSH|^~\\&|SEND|FAC|RECV|RFAC|20260715120000||ADT^A01||P|2.5\r"),
	} {
		if _, err := parseMessageHeader(payload, source.Framing); !errors.Is(err, ErrMessageHeader) {
			t.Fatalf("got %v for %q", err, payload)
		}
	}
}
