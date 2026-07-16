package batch

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestMessageReaderStreamsMixedLineEndingsAndResumes(t *testing.T) {
	t.Parallel()
	first := "MSH|^~\\&|A|B|C|D|202601010000||ADT^A01|ONE|P|2.5\rPID|1||123\r"
	second := "MSH|^~\\&|A|B|C|D|202601010000||ADT^A01|TWO|P|2.5\r\nPID|1||456\n"
	raw := []byte(first + second)
	reader, err := NewMessageReader(bytes.NewReader(raw), 0, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	message1, err := reader.Next()
	if err != nil {
		t.Fatal(err)
	}
	if message1.StartOffset != 0 || message1.EndOffset != int64(len(first)) || bytes.ContainsAny(message1.Payload, "\n") {
		t.Fatalf("first message = %#v", message1)
	}
	message2, err := reader.Next()
	if err != nil {
		t.Fatal(err)
	}
	if message2.StartOffset != int64(len(first)) || message2.EndOffset != int64(len(raw)) {
		t.Fatalf("second offsets = %d..%d", message2.StartOffset, message2.EndOffset)
	}
	if _, err := reader.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("final error = %v", err)
	}

	resumed, err := NewMessageReader(bytes.NewReader(raw[message1.EndOffset:]), message1.EndOffset, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	again, err := resumed.Next()
	if err != nil || !bytes.Equal(again.Payload, message2.Payload) || again.EndOffset != int64(len(raw)) {
		t.Fatalf("resumed = %#v, %v", again, err)
	}
}

func TestMessageReaderRejectsUnframedAndOversizedInput(t *testing.T) {
	t.Parallel()
	reader, _ := NewMessageReader(bytes.NewBufferString("PID|1||123\r"), 0, 1024)
	if _, err := reader.Next(); !errors.Is(err, ErrInvalidBatchStream) {
		t.Fatalf("unframed error = %v", err)
	}
	reader, _ = NewMessageReader(bytes.NewBufferString("MSH|"+string(bytes.Repeat([]byte{'X'}, 128))+"\r"), 0, 32)
	if _, err := reader.Next(); !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("oversize error = %v", err)
	}
}
