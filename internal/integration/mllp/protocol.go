package mllp

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"time"
	"unicode"
)

var (
	ErrFrameInvalid     = errors.New("invalid MLLP frame")
	ErrFrameTooLarge    = errors.New("MLLP frame exceeds configured limit")
	ErrFrameTimeout     = errors.New("MLLP frame read timed out")
	ErrMessageHeader    = errors.New("invalid HL7v2 message header")
	ErrResponseEncoding = errors.New("MLLP acknowledgement encoding failed")
)

var acknowledgementErrorCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

type messageHeader struct {
	fieldSeparator       byte
	encodingCharacters   string
	sendingApplication   string
	sendingFacility      string
	receivingApplication string
	receivingFacility    string
	messageType          string
	triggerEvent         string
	controlID            string
	processingID         string
	version              string
}

type acknowledgementOutcome uint8

const (
	acknowledgementAccepted acknowledgementOutcome = iota + 1
	acknowledgementTransientError
	acknowledgementPermanentReject
)

func readFrame(reader *bufio.Reader, policy FramingPolicy, maxBytes int64) ([]byte, error) {
	if reader == nil || maxBytes <= 0 {
		return nil, ErrFrameInvalid
	}
	start, err := reader.ReadByte()
	if err != nil {
		return nil, mapFrameReadError(err)
	}
	if start != policy.StartByte {
		return nil, ErrFrameInvalid
	}

	payload := make([]byte, 0, minInt64(maxBytes, 4096))
	for {
		value, err := reader.ReadByte()
		if err != nil {
			return nil, mapFrameReadError(err)
		}
		if value == policy.EndByte {
			trailer, err := reader.ReadByte()
			if err != nil {
				return nil, mapFrameReadError(err)
			}
			if trailer != policy.TrailerByte || len(payload) == 0 {
				return nil, ErrFrameInvalid
			}
			return payload, nil
		}
		if value == policy.StartByte {
			return nil, ErrFrameInvalid
		}
		payload = append(payload, value)
		if int64(len(payload)) > maxBytes {
			return nil, ErrFrameTooLarge
		}
	}
}

func mapFrameReadError(err error) error {
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return fmt.Errorf("%w: %w", ErrFrameTimeout, err)
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return ErrFrameInvalid
	}
	return fmt.Errorf("%w: %w", ErrFrameInvalid, err)
}

func framePayload(payload []byte, policy FramingPolicy) ([]byte, error) {
	if len(payload) == 0 || validateFraming(policy) != nil {
		return nil, ErrResponseEncoding
	}
	framed := make([]byte, 0, len(payload)+3)
	framed = append(framed, policy.StartByte)
	framed = append(framed, payload...)
	framed = append(framed, policy.EndByte, policy.TrailerByte)
	return framed, nil
}

func writeFrame(writer io.Writer, payload []byte, policy FramingPolicy) error {
	framed, err := framePayload(payload, policy)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, bytes.NewReader(framed))
	return err
}

func parseMessageHeader(payload []byte, policy FramingPolicy) (messageHeader, error) {
	if len(payload) < 12 || !bytes.HasPrefix(payload, []byte("MSH")) {
		return messageHeader{}, ErrMessageHeader
	}
	segmentEnd := bytes.IndexByte(payload, '\r')
	if segmentEnd < 0 {
		segmentEnd = len(payload)
	}
	segment := payload[:segmentEnd]
	if len(segment) < 8 {
		return messageHeader{}, ErrMessageHeader
	}
	separator := segment[3]
	if !validDelimiter(separator, policy) {
		return messageHeader{}, ErrMessageHeader
	}
	fields := strings.Split(string(segment), string(separator))
	if len(fields) < 12 || fields[0] != "MSH" || len(fields[1]) != 4 {
		return messageHeader{}, ErrMessageHeader
	}
	if !validEncodingCharacters(fields[1], separator, policy) {
		return messageHeader{}, ErrMessageHeader
	}
	for _, index := range []int{2, 3, 4, 5, 8, 9, 10, 11} {
		if !validHeaderField(fields[index], 256, policy) {
			return messageHeader{}, ErrMessageHeader
		}
	}
	if fields[9] == "" || containsDelimiter(fields[9], separator, fields[1]) {
		return messageHeader{}, ErrMessageHeader
	}
	components := strings.Split(fields[8], string(fields[1][0]))
	if len(components) == 0 || !validToken(components[0]) {
		return messageHeader{}, ErrMessageHeader
	}
	trigger := ""
	if len(components) > 1 {
		if !validToken(components[1]) {
			return messageHeader{}, ErrMessageHeader
		}
		trigger = components[1]
	}
	return messageHeader{
		fieldSeparator: separator, encodingCharacters: fields[1],
		sendingApplication: fields[2], sendingFacility: fields[3],
		receivingApplication: fields[4], receivingFacility: fields[5],
		messageType: components[0], triggerEvent: trigger, controlID: fields[9],
		processingID: fields[10], version: fields[11],
	}, nil
}

func buildAcknowledgement(
	header messageHeader,
	policy AcknowledgementPolicy,
	outcome acknowledgementOutcome,
	errorCode string,
	now time.Time,
	messageID string,
) ([]byte, error) {
	code, err := acknowledgementCode(policy.Mode, outcome)
	if err != nil || now.IsZero() || !validToken(messageID) {
		return nil, ErrResponseEncoding
	}
	if outcome != acknowledgementAccepted && !acknowledgementErrorCodePattern.MatchString(errorCode) {
		return nil, ErrResponseEncoding
	}
	separator := string(header.fieldSeparator)
	component := string(header.encodingCharacters[0])
	trigger := header.triggerEvent
	messageType := "ACK"
	if trigger != "" {
		messageType = strings.Join([]string{"ACK", trigger, "ACK"}, component)
	}
	msh := strings.Join([]string{
		"MSH" + separator + header.encodingCharacters,
		header.receivingApplication,
		header.receivingFacility,
		header.sendingApplication,
		header.sendingFacility,
		now.UTC().Format("20060102150405"),
		"",
		messageType,
		messageID,
		header.processingID,
		header.version,
	}, separator)
	msa := strings.Join([]string{"MSA", code, header.controlID}, separator)
	segments := []string{msh, msa}
	if outcome != acknowledgementAccepted && policy.IncludeErrorSegment {
		errorValue := strings.Join([]string{errorCode, errorCode, "FI-FHIR"}, component)
		segments = append(segments, strings.Join([]string{"ERR", "", "", errorValue, "E"}, separator))
	}
	return []byte(strings.Join(segments, "\r") + "\r"), nil
}

func acknowledgementCode(mode AcknowledgementMode, outcome acknowledgementOutcome) (string, error) {
	switch mode {
	case AcknowledgementModeApplication:
		switch outcome {
		case acknowledgementAccepted:
			return "AA", nil
		case acknowledgementTransientError:
			return "AE", nil
		case acknowledgementPermanentReject:
			return "AR", nil
		}
	case AcknowledgementModeCommit:
		switch outcome {
		case acknowledgementAccepted:
			return "CA", nil
		case acknowledgementTransientError:
			return "CE", nil
		case acknowledgementPermanentReject:
			return "CR", nil
		}
	}
	return "", ErrResponseEncoding
}

func validDelimiter(value byte, policy FramingPolicy) bool {
	if value < 0x21 || value > 0x7e {
		return false
	}
	return value != policy.StartByte && value != policy.EndByte && value != policy.TrailerByte
}

func validEncodingCharacters(value string, separator byte, policy FramingPolicy) bool {
	seen := map[byte]struct{}{separator: {}}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if !validDelimiter(character, policy) {
			return false
		}
		if _, duplicate := seen[character]; duplicate {
			return false
		}
		seen[character] = struct{}{}
	}
	return true
}

func validHeaderField(value string, maxBytes int, policy FramingPolicy) bool {
	if len(value) > maxBytes {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || character == rune(policy.StartByte) ||
			character == rune(policy.EndByte) || character == rune(policy.TrailerByte) {
			return false
		}
	}
	return true
}

func containsDelimiter(value string, separator byte, encoding string) bool {
	return strings.ContainsRune(value, rune(separator)) || strings.ContainsAny(value, encoding)
}

func validToken(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) &&
			character != '-' && character != '_' && character != '.' {
			return false
		}
	}
	return true
}

func minInt64(left, right int64) int {
	if left < right {
		return int(left)
	}
	return int(right)
}
