package connection

import (
	"net"
	"net/netip"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// The predicates below restate, character for character, the unexported
// checks inside mllp/source.go, mllp/identity.go, batch/source.go, and
// destination/revision.go. They are copies rather than calls because those
// constructors deliberately return one coarse error for inventory safety,
// and the catalog needs the field that failed.
//
// The constructors stay the authority. TestConnectionChecker_MirrorsConstructorBounds
// drives every bound through both the checker and the constructor, so a
// constructor that tightens a rule without the copy here turns that test red
// instead of turning a compile into CONSTRUCTOR_REJECTED.

// validIdentity is validIdentity from mllp, batch, and destination.
func validIdentity(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}

// validSecretToken is pkg/integration.validSecretToken: a binding key or
// version is at most 256 bytes with no space or control character.
func validSecretToken(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for _, character := range value {
		if character <= 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

// connectionIDPattern is the catalog's own identifier rule. It is a strict
// subset of validIdentity: a connection ID becomes an artifact ID inside a
// mounted document and the stem of a downloaded file name, so it admits no
// separator a path or a shell would reinterpret.
var connectionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func validConnectionID(value string) bool {
	return connectionIDPattern.MatchString(value)
}

// validListenAddress is mllp.validListenAddress.
func validListenAddress(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\x00") {
		return false
	}
	_, port, err := net.SplitHostPort(value)
	if err != nil {
		return false
	}
	number, err := strconv.Atoi(port)
	return err == nil && number >= 1 && number <= 65535 && strconv.Itoa(number) == port
}

// canonicalCIDR reports whether value is an allowlist entry mllp accepts: a
// prefix written in its canonical, masked form.
func canonicalCIDR(value string) (netip.Prefix, bool) {
	prefix, err := netip.ParsePrefix(value)
	if err != nil || prefix.String() != value || prefix != prefix.Masked() {
		return netip.Prefix{}, false
	}
	return prefix, true
}

// validURISAN is mllp.validateURISAN as a predicate.
func validURISAN(value string) bool {
	if !validIdentity(value) || len(value) > 512 {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" {
		return false
	}
	if parsed.Host == "" && parsed.Opaque == "" && parsed.Path == "" {
		return false
	}
	return parsed.String() == value
}

// validSPKIPin is mllp.validSPKIPin.
func validSPKIPin(value string) bool {
	hexDigits, found := strings.CutPrefix(value, "sha256:")
	if !found || len(hexDigits) != 64 {
		return false
	}
	for index := 0; index < len(hexDigits); index++ {
		character := hexDigits[index]
		if character >= '0' && character <= '9' || character >= 'a' && character <= 'f' {
			continue
		}
		return false
	}
	return true
}

// validEndpoint is batch.validEndpoint.
func validEndpoint(value string) bool {
	if value == "" || strings.Contains(value, "://") || strings.TrimSpace(value) != value || strings.ContainsAny(value, "/\r\n\x00") {
		return false
	}
	host, port, err := net.SplitHostPort(value)
	if err == nil {
		number, parseErr := strconv.Atoi(port)
		return validHost(host) && parseErr == nil && number > 0 && number <= 65535 && strconv.Itoa(number) == port
	}
	return validHost(value)
}

// loopbackEndpoint is batch.loopbackEndpoint.
func loopbackEndpoint(value string) bool {
	host := value
	if parsedHost, _, err := net.SplitHostPort(value); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	return host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

// validHost is batch.validHost.
func validHost(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\x00/\\") {
		return false
	}
	if ip := net.ParseIP(strings.Trim(value, "[]")); ip != nil {
		return true
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, character := range label {
			if !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '-' {
				return false
			}
		}
	}
	return true
}

// validRelativePrefix is batch.validRelativePrefix.
func validRelativePrefix(value string) bool {
	return value != "" && value == path.Clean(value) && !path.IsAbs(value) && value != "." &&
		!strings.HasPrefix(value, "../") && !strings.ContainsAny(value, "\r\n\x00\\")
}

// validAbsolutePath is batch.validAbsolutePath.
func validAbsolutePath(value string) bool {
	return value != "" && path.IsAbs(value) && value == path.Clean(value) && value != "/" &&
		!strings.ContainsAny(value, "\r\n\x00\\")
}

// prefixesOverlap is batch.prefixesOverlap.
func prefixesOverlap(left, right string) bool {
	left = strings.TrimSuffix(left, "/")
	right = strings.TrimSuffix(right, "/")
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

// httpsURLProblem returns the reason an https destination URL is refused, or
// "" when destination.validateHTTPS would accept it. withQuery is false for a
// FHIR base URL, which additionally forbids a query.
func httpsURLProblem(value string, withQuery bool) string {
	if value == "" {
		return "required"
	}
	if len(value) > 2048 {
		return "must be at most 2048 characters"
	}
	parsed, err := url.Parse(value)
	switch {
	case err != nil:
		return "is not a URL"
	case parsed.Scheme != "https":
		return "must use the https scheme"
	case parsed.Host == "":
		return "must name a host"
	case parsed.User != nil:
		return "must not carry credentials"
	case parsed.Fragment != "":
		return "must not carry a fragment"
	case !withQuery && parsed.RawQuery != "":
		return "must not carry a query"
	}
	return ""
}
