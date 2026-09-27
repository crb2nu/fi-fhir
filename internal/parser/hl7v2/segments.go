package hl7v2

// SplitMessage tokenizes raw HL7v2 into segments and fields exactly the way
// ParseWithResult does before it builds a semantic event: line endings are
// normalized, MSH-1 and MSH-2 set the delimiters, and each Segment's Fields
// are indexed by HL7 field number (Fields[0] is the segment ID, and for MSH
// Fields[1] is the field separator itself). Strict A01 validation is not
// applied. It exists so a caller that shows segments beside the event (the
// browser kernel) shows the parser's own tokenization, not a second splitter.
func SplitMessage(raw string) (*Message, error) {
	return (&Parser{}).parseRaw(raw)
}
