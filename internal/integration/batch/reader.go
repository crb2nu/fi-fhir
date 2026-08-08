package batch

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

var (
	ErrInvalidBatchStream = errors.New("invalid HL7v2 batch stream")
	ErrMessageTooLarge    = errors.New("batch message exceeds configured limit")
)

// Message is one normalized HL7v2 message and its exact raw byte interval.
// Raw holds the untouched source bytes covering [StartOffset, EndOffset), which
// is what the streaming content digest hashes. Payload is the normalized form
// handed to the processor and is not byte-identical to Raw.
type Message struct {
	Payload     []byte
	Raw         []byte
	StartOffset int64
	EndOffset   int64
}

// MessageReader incrementally splits concatenated HL7v2 messages on MSH
// boundaries. It buffers at most one configured message plus scanner overhead.
type MessageReader struct {
	scanner         *bufio.Scanner
	maxMessageBytes int64
	offset          int64
	pendingSegment  []byte
	pendingRaw      []byte
	pendingStart    int64
	pendingEnd      int64
	finished        bool
}

func NewMessageReader(reader io.Reader, startOffset, maxMessageBytes int64) (*MessageReader, error) {
	if reader == nil || startOffset < 0 || maxMessageBytes < 1 {
		return nil, ErrInvalidBatchStream
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), int(maxMessageBytes)+2)
	scanner.Split(splitSegmentWithDelimiter)
	return &MessageReader{scanner: scanner, maxMessageBytes: maxMessageBytes, offset: startOffset}, nil
}

func (r *MessageReader) Next() (Message, error) {
	if r == nil || r.scanner == nil || r.finished {
		return Message{}, io.EOF
	}
	var payload []byte
	var raw []byte
	var start int64
	if r.pendingSegment != nil {
		payload = append(payload, r.pendingSegment...)
		raw = append(raw, r.pendingRaw...)
		start = r.pendingStart
		r.offset = r.pendingEnd
		r.pendingSegment = nil
		r.pendingRaw = nil
	}
	for r.scanner.Scan() {
		token := append([]byte(nil), r.scanner.Bytes()...)
		tokenStart := r.offset
		r.offset += int64(len(token))
		segment := bytes.TrimRight(token, "\r\n")
		if len(segment) == 0 {
			// Blank separators carry no HL7v2 content but still occupy the raw
			// byte interval the streaming digest must cover.
			raw = append(raw, token...)
			continue
		}
		if bytes.HasPrefix(segment, []byte("MSH")) {
			if len(payload) != 0 {
				r.pendingSegment = append([]byte(nil), segment...)
				r.pendingRaw = token
				r.pendingStart = tokenStart
				r.pendingEnd = r.offset
				return Message{Payload: payload, Raw: raw, StartOffset: start, EndOffset: tokenStart}, nil
			}
			start = tokenStart
			raw = nil
		} else if len(payload) == 0 {
			return Message{}, ErrInvalidBatchStream
		}
		if len(payload) != 0 {
			payload = append(payload, '\r')
		}
		payload = append(payload, segment...)
		raw = append(raw, token...)
		if int64(len(payload)) > r.maxMessageBytes {
			return Message{}, ErrMessageTooLarge
		}
	}
	if err := r.scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return Message{}, ErrMessageTooLarge
		}
		return Message{}, err
	}
	r.finished = true
	if len(payload) == 0 {
		return Message{}, io.EOF
	}
	return Message{Payload: payload, Raw: raw, StartOffset: start, EndOffset: r.offset}, nil
}

func splitSegmentWithDelimiter(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for index, value := range data {
		if value != '\r' && value != '\n' {
			continue
		}
		advance = index + 1
		if value == '\r' && advance < len(data) && data[advance] == '\n' {
			advance++
		}
		return advance, data[:advance], nil
	}
	if atEOF && len(data) != 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
