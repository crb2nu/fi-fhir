//go:build !structuredloggingleak

package workflow

import "fmt"

// Publish records that a message was published. It deliberately records no
// message content.
//
// `log` is the only queue driver this binary registers (queue.go's init), so
// whatever this method prints is what every queue action in every workflow
// prints. The message key is extracted from an event field path — `key:
// data.mrn` is a realistic configuration (queueAction) — and the value is the
// whole serialized event. Both are message content, so neither is written; the
// line carries their sizes instead. Header values are author-configured
// constants rather than event data, but the name set is all a reader needs to
// confirm routing, so the values stay out too.
//
// The rendered topic is retained: it is the routing destination, and a
// publisher line without one is not worth writing. A workflow author who
// templates a patient identifier into a topic name has put message content into
// a routing identifier, which is an authoring defect this driver cannot repair.
//
// See `.loom/40-decisions.md` 2026-08-09 ("redaction precedes conversion").
//
// The pre-slice behaviour lives in queue_publish_leak.go behind the
// `structuredloggingleak` build tag. It is this slice's negative control: with
// the tag set, the PHI assertion in
// `TestStructuredLogging_CorrelatedAndPHIFree` must FAIL. A control that passes
// means the sentinel scan is vacuous, which is exactly the failure 4.2a's
// negative control caught.
func (p *LogQueuePublisher) Publish(topic string, key []byte, value []byte, headers map[string]string) error {
	fmt.Printf("[Queue:%s] topic=%s key_bytes=%d header_names=%s value_bytes=%d\n",
		p.name, topic, len(key), redactedHeaderNames(headers), len(value))
	return nil
}
