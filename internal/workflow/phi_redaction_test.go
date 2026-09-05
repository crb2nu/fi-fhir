package workflow

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

// Slice 4.4d, commit 1 of Lane S5-C: the three print sites in the legacy engine
// that carry message content, closed before any print statement anywhere is
// converted to structured logging.
//
// The ordering is the point. `LogQueuePublisher` is the only registered queue
// driver (`queue.go` init), so a mechanical `fmt.Printf` → structured-log
// conversion applied before this commit would move a whole serialized event out
// of an ad-hoc terminal write and into a stream that log aggregators index and
// retain. See `.loom/40-decisions.md` 2026-08-09, decision 2.
//
// The day-1 gate
// `TestStructuredLogging_ServeEmitsNoStructuredLogAndTheQueueDriverPrintsPayloads`
// asserts the pre-slice behaviour these tests invert; the two must not both
// pass after this commit lands.

// redactionSentinel stands in for a patient identifier. It is deliberately
// distinctive so a substring match cannot succeed by accident.
const redactionSentinel = "MRN4404DREDACT"

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	func() {
		defer func() {
			os.Stdout = original
			_ = w.Close()
		}()
		fn()
	}()

	captured := <-done
	_ = r.Close()
	return captured
}

func TestLogQueuePublisherRedactsMessageContent(t *testing.T) {
	publisher, err := NewLogQueuePublisher(map[string]string{})
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}

	// The key is extracted from an event field path, so `key: data.mrn` puts a
	// patient identifier here. The value is the whole serialized event. Both
	// are message content.
	key := []byte(redactionSentinel)
	value := []byte(`{"id":"evt-1","data":{"mrn":"` + redactionSentinel + `","family":"SENTINELFAMILY"}}`)
	headers := map[string]string{
		"x-source":      "unit-test",
		"x-patient-mrn": redactionSentinel,
	}

	output := captureStdout(t, func() {
		if err := publisher.Publish("canonical-events", key, value, headers); err != nil {
			t.Errorf("publish: %v", err)
		}
	})

	// Anti-vacuity: the driver actually wrote a line. Without this, deleting the
	// print statement outright would pass every assertion below.
	if !strings.Contains(output, "[Queue:log]") {
		t.Fatalf("anti-vacuity: the publisher wrote no recognisable line; got %q", output)
	}

	for _, forbidden := range []string{redactionSentinel, "SENTINELFAMILY", `"mrn"`} {
		if strings.Contains(output, forbidden) {
			t.Errorf("published line leaks %q: %s", forbidden, output)
		}
	}

	// What it must still say, so the line remains worth writing.
	for _, required := range []string{
		"topic=canonical-events",
		fmt.Sprintf("key_bytes=%d", len(key)),
		fmt.Sprintf("value_bytes=%d", len(value)),
		"header_names=[x-patient-mrn,x-source]",
	} {
		if !strings.Contains(output, required) {
			t.Errorf("published line is missing %q: %s", required, output)
		}
	}
}

func TestLogQueuePublisherRendersEmptyHeaderSet(t *testing.T) {
	publisher, err := NewLogQueuePublisher(map[string]string{"name": "probe"})
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	output := captureStdout(t, func() {
		if err := publisher.Publish("t", nil, nil, nil); err != nil {
			t.Errorf("publish: %v", err)
		}
	})
	if !strings.Contains(output, "[Queue:probe]") {
		t.Errorf("publisher name is not rendered: %s", output)
	}
	if !strings.Contains(output, "header_names=[]") {
		t.Errorf("empty header set should render as []: %s", output)
	}
	if !strings.Contains(output, "key_bytes=0") || !strings.Contains(output, "value_bytes=0") {
		t.Errorf("zero sizes should still be reported: %s", output)
	}
}

func TestQueueActionThroughTheRegisteredDriverLeaksNoPayload(t *testing.T) {
	// The end-to-end shape a workflow author gets: the `queue` action, the
	// globally registered `log` driver, and an event carrying a sentinel.
	event := map[string]interface{}{
		"id":     "evt-1",
		"type":   "patient.admit",
		"source": "redaction-test",
		"data": map[string]interface{}{
			"mrn":    redactionSentinel,
			"family": "SENTINELFAMILY",
		},
	}
	config := map[string]string{
		"driver": "log",
		"topic":  "admissions",
		"key":    "data.mrn",
	}

	output := captureStdout(t, func() {
		if err := queueAction(event, config); err != nil {
			t.Errorf("queue action: %v", err)
		}
	})

	if !strings.Contains(output, "[Queue:log]") {
		t.Fatalf("anti-vacuity: the queue action never reached the driver; got %q", output)
	}
	if strings.Contains(output, redactionSentinel) {
		t.Errorf("the queue action leaks the message key verbatim: %s", output)
	}
	if strings.Contains(output, "SENTINELFAMILY") {
		t.Errorf("the queue action leaks the payload: %s", output)
	}
	// The key was extracted and is non-empty — the redaction is of a real value,
	// not of an absent one.
	if strings.Contains(output, "key_bytes=0") {
		t.Errorf("anti-vacuity: no message key was extracted, so redacting it proves nothing: %s", output)
	}
}

func TestLogActionAtDebugLevelRecordsShapeNotValues(t *testing.T) {
	event := map[string]interface{}{
		"id":     "evt-1",
		"type":   "patient.admit",
		"source": "redaction-test",
		"data": map[string]interface{}{
			"mrn": redactionSentinel,
		},
	}
	config := map[string]string{
		"level":   "debug",
		"message": "Processed admission",
	}

	output := captureStdout(t, func() {
		if err := logAction(event, config); err != nil {
			t.Errorf("log action: %v", err)
		}
	})

	if !strings.Contains(output, "Processed admission") {
		t.Fatalf("anti-vacuity: the log action wrote nothing recognisable; got %q", output)
	}
	if !strings.Contains(output, "Event: <redacted:") {
		t.Errorf("debug level should still describe the event: %s", output)
	}
	if strings.Contains(output, redactionSentinel) {
		t.Errorf("debug level leaks the event payload: %s", output)
	}
	if !strings.Contains(output, "4 top-level fields") {
		t.Errorf("the shape summary should report the field count: %s", output)
	}
}

func TestDescribeEventShapeHandlesUnserializableAndNonObjectEvents(t *testing.T) {
	// A channel cannot be marshalled; the summary must not panic or leak.
	if got := describeEventShape(make(chan int)); !strings.HasPrefix(got, "<redacted:") {
		t.Errorf("unserializable event: got %q", got)
	}
	// A non-object event has no top-level fields but still has a size.
	got := describeEventShape([]string{redactionSentinel})
	if strings.Contains(got, redactionSentinel) {
		t.Errorf("array event leaks its contents: %q", got)
	}
	if !strings.Contains(got, "0 top-level fields") {
		t.Errorf("array event should report zero top-level fields: %q", got)
	}
}

func TestBoundedErrorTextCapsAnUnboundedError(t *testing.T) {
	if got := boundedErrorText(nil); got != "<nil>" {
		t.Errorf("nil error: got %q", got)
	}

	short := errors.New("connection refused")
	if got := boundedErrorText(short); got != "connection refused" {
		t.Errorf("short error should pass through unchanged: %q", got)
	}

	// A DLQ implementation quoting the record it could not write.
	long := fmt.Errorf("insert failed for record: %s", strings.Repeat("A", 4096)+redactionSentinel)
	got := boundedErrorText(long)
	if strings.Contains(got, redactionSentinel) {
		t.Errorf("a 4KB error still reached the tail of the line: %q", got)
	}
	if len(got) > maxLoggedErrorBytes+64 {
		t.Errorf("bounded text is %d bytes, want <= %d plus the truncation note",
			len(got), maxLoggedErrorBytes)
	}
	if !strings.Contains(got, "bytes truncated") {
		t.Errorf("truncation must be visible to the reader: %q", got)
	}

	// Truncation lands on a rune boundary: a multi-byte error must stay valid
	// UTF-8 so a log parser downstream does not choke on a split rune.
	multibyte := errors.New(strings.Repeat("é", 4096))
	bounded := boundedErrorText(multibyte)
	if !isValidUTF8(bounded) {
		t.Errorf("truncation split a rune: %q", bounded)
	}
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}
