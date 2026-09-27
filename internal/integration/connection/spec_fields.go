package connection

import (
	"fmt"
)

// Field-level helpers shared by the per-kind checkers. Each returns whether
// the value passed, so a checker can skip cross-field rules whose inputs are
// already known to be wrong.

// identity checks a required identifier field.
func (c *checker) identity(path, value string) bool {
	switch {
	case value == "":
		c.add(CodeRequired, path, "is required")
		return false
	case !validIdentity(value):
		c.add(CodeInvalidValue, path, "must be at most 256 characters with no whitespace or control characters")
		return false
	}
	return true
}

// binding checks a required `*_binding` field's own shape. Whether it names a
// declared binding is checkBindings' question.
func (c *checker) binding(path, value string) bool {
	return c.identity(path, value)
}

// intRange checks a required integer field against inclusive bounds.
func (c *checker) intRange(path string, value *int64, low, high int64) bool {
	if value == nil {
		c.add(CodeRequired, path, "is required")
		return false
	}
	if *value < low || *value > high {
		c.add(CodeOutOfRange, path, fmt.Sprintf("must be between %d and %d", low, high))
		return false
	}
	return true
}

// enum checks a required string field against a closed set.
func (c *checker) enum(path, value string, allowed ...string) bool {
	if value == "" {
		c.add(CodeRequired, path, "is required")
		return false
	}
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	c.add(CodeInvalidEnum, path, "must be one of "+joinQuoted(allowed))
	return false
}

// grants checks a grant list: at most max canonical identifiers, no repeat.
func (c *checker) grants(path string, values []string, low, high int) bool {
	ok := true
	if len(values) < low || len(values) > high {
		if low > 0 && len(values) == 0 {
			c.add(CodeRequired, path, fmt.Sprintf("needs at least %d grant", low))
		} else {
			c.add(CodeOutOfRange, path, fmt.Sprintf("must hold between %d and %d grants", low, high))
		}
		ok = false
	}
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		elementPath := fmt.Sprintf("%s[%d]", path, index)
		if !c.identity(elementPath, value) {
			ok = false
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			c.add(CodeDuplicate, elementPath, "is repeated")
			ok = false
		}
		seen[value] = struct{}{}
	}
	return ok
}

func joinQuoted(values []string) string {
	out := ""
	for index, value := range values {
		switch {
		case index == 0:
		case index == len(values)-1:
			out += " or "
		default:
			out += ", "
		}
		out += fmt.Sprintf("%q", value)
	}
	return out
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func boolValue(value *bool) bool {
	return value != nil && *value
}
