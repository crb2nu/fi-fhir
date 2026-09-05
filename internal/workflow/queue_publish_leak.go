//go:build structuredloggingleak

package workflow

import "fmt"

// Publish restores the pre-slice-4.4d behaviour: the entire serialized event,
// the extracted message key, and every header value written verbatim to stdout.
//
// This file exists ONLY to be the negative control for
// `TestStructuredLogging_CorrelatedAndPHIFree`. It is compiled out of every
// shipped build — no release, image, or CI job sets `structuredloggingleak` —
// and `make structured-logging-negative-control` inverts the exit status of a
// tagged run, so a zero exit there means the PHI assertion has stopped
// measuring anything.
//
// The idiom is `transportgateblanket`'s (Makefile:
// transport-gate-negative-control). It exists because 4.2a's control caught a
// sentinel scan that passed for the wrong reason: the sentinel was never in the
// captured stream to begin with, so "the sentinel does not appear" was true and
// meaningless.
//
// Do not "fix" this file. Its whole job is to leak.
func (p *LogQueuePublisher) Publish(topic string, key []byte, value []byte, headers map[string]string) error {
	fmt.Printf("[Queue:%s] Topic: %s, Key: %s, Headers: %v, Value: %s\n",
		p.name, topic, string(key), headers, string(value))
	return nil
}
