package resolvers

import (
	"container/list"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/fhir/subscription"
)

// FHIR subscription destinations are a deployment-owned allowlist
// (docs/operations/SECURITY.md, SEC-2026-09-27-2).
//
// Before 2026-09-27 createFhirSubscription built an HTTP client for whatever
// `server` URL the caller supplied and POSTed a Subscription resource to it,
// and the per-URL client map grew without bound. Any `graphql:operator` holder
// could make the API pod issue requests to any in-cluster or LAN address.

// Environment keys serve reads to build FHIRSubscriptionPolicy. The resolver
// never reads the environment itself; cmd/fi-fhir does.
const (
	EnvFHIRSubscriptionAllowedHosts = "FI_FHIR_FHIR_SUBSCRIPTION_ALLOWED_HOSTS"
	EnvFHIRSubscriptionMaxClients   = "FI_FHIR_FHIR_SUBSCRIPTION_MAX_CLIENTS"
)

// DefaultFHIRSubscriptionMaxClients bounds the per-server client cache.
const DefaultFHIRSubscriptionMaxClients = 32

const (
	subscriptionClientTimeout   = 30 * time.Second
	subscriptionMaxRedirectHops = 5
)

// ErrFHIRSubscriptionDestinationNotAllowed is the one error every refused
// destination returns. It is inventory-safe: it says neither which part of the
// URL failed nor which hosts are allowed.
var ErrFHIRSubscriptionDestinationNotAllowed = errors.New("FHIR subscription destination not allowed")

// FHIRSubscriptionPolicy decides which FHIR servers createFhirSubscription (and
// delete/pause/resume) may contact.
//
// A destination is allowed when its scheme is https — or http to a loopback
// host, which exists for tests — it carries no user info, and its host matches
// AllowedHosts. An entry is an exact host name or IP literal, or `*.suffix`,
// which matches any name strictly below suffix. A wildcard never matches an IP
// literal or `localhost`; those must be listed exactly. The zero value allows
// nothing.
type FHIRSubscriptionPolicy struct {
	AllowedHosts []string
	MaxClients   int
}

// DefaultFHIRSubscriptionPolicy refuses every destination.
func DefaultFHIRSubscriptionPolicy() FHIRSubscriptionPolicy {
	return FHIRSubscriptionPolicy{MaxClients: DefaultFHIRSubscriptionMaxClients}
}

// ParseFHIRSubscriptionAllowedHosts parses the comma-separated allowlist. An
// entry that is not a bare host name or IP literal (it carries a scheme, port,
// path, or user info), or a wildcard that is not `*.` followed by a
// multi-label domain, is a configuration error rather than a silent drop.
func ParseFHIRSubscriptionAllowedHosts(raw string) ([]string, error) {
	var hosts []string
	seen := make(map[string]struct{})
	for _, item := range strings.Split(raw, ",") {
		entry := strings.ToLower(strings.TrimSpace(item))
		if entry == "" {
			continue
		}
		wildcard := strings.HasPrefix(entry, "*.")
		name := strings.TrimSuffix(strings.TrimPrefix(entry, "*."), ".")
		switch {
		case net.ParseIP(name) != nil:
			if wildcard {
				return nil, fmt.Errorf("%s wildcard %q cannot name an IP literal", EnvFHIRSubscriptionAllowedHosts, item)
			}
		case !validAllowlistHostName(name):
			return nil, fmt.Errorf("%s entry %q must be a bare host name, IP literal, or *.suffix", EnvFHIRSubscriptionAllowedHosts, item)
		case wildcard && !strings.Contains(name, "."):
			return nil, fmt.Errorf("%s wildcard %q must name a multi-label domain suffix", EnvFHIRSubscriptionAllowedHosts, item)
		}
		if wildcard {
			name = "*." + name
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		hosts = append(hosts, name)
	}
	return hosts, nil
}

func validAllowlistHostName(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
				return false
			}
		}
	}
	return true
}

// CheckDestination returns the canonical destination URL, or
// ErrFHIRSubscriptionDestinationNotAllowed.
func (p FHIRSubscriptionPolicy) CheckDestination(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Opaque != "" || parsed.User != nil || parsed.Host == "" {
		return nil, ErrFHIRSubscriptionDestinationNotAllowed
	}
	scheme := strings.ToLower(parsed.Scheme)
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" {
		return nil, ErrFHIRSubscriptionDestinationNotAllowed
	}
	switch scheme {
	case "https":
	case "http":
		if !isLoopbackHost(host) {
			return nil, ErrFHIRSubscriptionDestinationNotAllowed
		}
	default:
		return nil, ErrFHIRSubscriptionDestinationNotAllowed
	}
	if !p.hostAllowed(host) {
		return nil, ErrFHIRSubscriptionDestinationNotAllowed
	}
	parsed.Scheme = scheme
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsed, nil
}

func (p FHIRSubscriptionPolicy) hostAllowed(host string) bool {
	literal := net.ParseIP(host) != nil || host == "localhost"
	for _, entry := range p.AllowedHosts {
		if entry == host {
			return true
		}
		if literal || !strings.HasPrefix(entry, "*.") {
			continue
		}
		if strings.HasSuffix(host, entry[1:]) {
			return true
		}
	}
	return false
}

func (p FHIRSubscriptionPolicy) maxClients() int {
	if p.MaxClients <= 0 {
		return DefaultFHIRSubscriptionMaxClients
	}
	return p.MaxClients
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// newSubscriptionHTTPClient re-applies the destination policy to every
// redirect hop, so an allowed server cannot bounce the request elsewhere.
func newSubscriptionHTTPClient(policy FHIRSubscriptionPolicy) *http.Client {
	return &http.Client{
		Timeout: subscriptionClientTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= subscriptionMaxRedirectHops {
				return ErrFHIRSubscriptionDestinationNotAllowed
			}
			if _, err := policy.CheckDestination(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
}

// subscriptionClientCache is a bounded LRU of FHIR subscription clients keyed
// by canonical server URL. It is not safe for concurrent use; the resolver
// guards it with subscriptionMu.
type subscriptionClientCache struct {
	limit int
	order *list.List
	items map[string]*list.Element
}

type subscriptionClientEntry struct {
	key    string
	client *subscription.Client
}

func newSubscriptionClientCache(limit int) *subscriptionClientCache {
	return &subscriptionClientCache{limit: limit, order: list.New(), items: make(map[string]*list.Element)}
}

// entryOf returns the entry an element holds. Every element is inserted by
// add, so the assertion cannot fail; a nil return is treated as a miss.
func entryOf(element *list.Element) *subscriptionClientEntry {
	entry, _ := element.Value.(*subscriptionClientEntry)
	return entry
}

func (c *subscriptionClientCache) get(key string) (*subscription.Client, bool) {
	element, ok := c.items[key]
	if !ok {
		return nil, false
	}
	entry := entryOf(element)
	if entry == nil {
		return nil, false
	}
	c.order.MoveToFront(element)
	return entry.client, true
}

// add inserts a client and evicts the least recently used entries beyond the
// limit.
func (c *subscriptionClientCache) add(key string, client *subscription.Client) {
	if element, ok := c.items[key]; ok {
		if entry := entryOf(element); entry != nil {
			entry.client = client
			c.order.MoveToFront(element)
			return
		}
		c.order.Remove(element)
	}
	c.items[key] = c.order.PushFront(&subscriptionClientEntry{key: key, client: client})
	for c.order.Len() > c.limit {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		if entry := entryOf(oldest); entry != nil {
			delete(c.items, entry.key)
		}
	}
}

func (c *subscriptionClientCache) len() int { return c.order.Len() }
