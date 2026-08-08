package autoroute

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

// ErrNotifierUnavailable is returned when a review notifier or webhook sink is
// constructed or run without the collaborators it needs.
var ErrNotifierUnavailable = errors.New("autoroute review notifier unavailable")

// ReviewEventName identifies the notification payload shape for receivers.
const ReviewEventName = "terminology.pending_autoroute.review_required"

const (
	// DefaultNotifyScanLimit bounds how many pending rows one scan inspects.
	// Rows come back ordered by confidence descending, so the scan always sees
	// the highest-confidence work first.
	DefaultNotifyScanLimit = 50

	// defaultNotifyQueueSize bounds the asynchronous dispatch queue. It is small
	// on purpose: a backed-up webhook should drop stale digests rather than
	// accumulate them, because each digest already re-states the current queue.
	defaultNotifyQueueSize = 8

	// notifySeenCapacity bounds the de-duplication set. Exceeding it evicts the
	// oldest IDs, which can at worst re-notify about a very old pending row.
	notifySeenCapacity = 1024

	// webhookMaxAttempts is one delivery plus one bounded retry. Notifications
	// are advisory; a determined retry loop would be a second failure domain for
	// no clinical benefit.
	webhookMaxAttempts = 2

	// webhookRetryDelay is the fixed pause between the two attempts.
	webhookRetryDelay = 250 * time.Millisecond
)

// ReviewItem is the PHI-minimal projection of a pending autoroute that leaves
// the process in a notification.
//
// Only coded identity, confidence, and lifecycle timestamps are included.
// Everything free-text or LLM-authored on db.PendingAutoroute is deliberately
// omitted — source_display, suggested_display, reasoning, decision_trace,
// alternates, rejection_reason, reviewed_by — because those fields can quote
// source message content. A reviewer follows the ID back into the approval UI
// for the full trace; the webhook receiver is an untrusted egress point.
type ReviewItem struct {
	ID            int64      `json:"id"`
	SourceSystem  string     `json:"source_system"`
	SourceCode    string     `json:"source_code"`
	TargetSystem  string     `json:"target_system"`
	SuggestedCode string     `json:"suggested_code"`
	Confidence    float64    `json:"confidence"`
	Equivalence   string     `json:"equivalence,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

// newReviewItem projects a stored pending autoroute onto the wire shape.
func newReviewItem(p *db.PendingAutoroute) ReviewItem {
	item := ReviewItem{
		ID:            p.ID,
		SourceSystem:  p.SourceSystem,
		SourceCode:    p.SourceCode,
		TargetSystem:  p.TargetSystem,
		SuggestedCode: p.SuggestedCode,
		Confidence:    p.Confidence,
		Equivalence:   p.Equivalence,
		CreatedAt:     p.CreatedAt,
	}
	if p.ExpiresAt != nil {
		expires := *p.ExpiresAt
		item.ExpiresAt = &expires
	}
	return item
}

// ReviewNotification is one dispatched digest of pending autoroutes that have
// crossed the confidence threshold and are waiting on a human.
type ReviewNotification struct {
	Event         string    `json:"event"`
	GeneratedAt   time.Time `json:"generated_at"`
	MinConfidence float64   `json:"min_confidence"`
	// NewCount is how many items in this digest had not been notified before.
	NewCount int `json:"new_count"`
	// EligibleCount is how many rows at or above the threshold the scan saw,
	// including previously notified ones. It is the review backlog size.
	EligibleCount int          `json:"eligible_count"`
	Items         []ReviewItem `json:"items"`
}

// NotificationSink delivers a notification to an external system.
//
// Webhook delivery is the only implementation today. The interface exists so a
// second transport (queue, e-mail relay) does not have to touch the scan loop,
// and so tests can assert dispatch behavior without a network.
type NotificationSink interface {
	Publish(ctx context.Context, notification ReviewNotification) error
}

// PendingAutorouteLister reads the pending autoroute review queue.
//
// *db.MappingStore satisfies this. As with PendingAutorouteExpirer, the
// notifier depends on the narrow behavior rather than the store so notification
// policy stays out of the database package and is testable without Postgres.
type PendingAutorouteLister interface {
	ListPendingAutoroutes(ctx context.Context, filter db.ListPendingAutoroutesFilter) ([]*db.PendingAutoroute, int, error)
}

// NotifyResult reports one scan of the review queue.
type NotifyResult struct {
	// Eligible is how many scanned rows met the confidence threshold.
	Eligible int
	// New is how many of those had not been notified before.
	New int
	// Queued is 1 when a digest was enqueued for delivery, 0 otherwise.
	Queued int
	// Dropped is 1 when a digest was discarded because the dispatch queue was
	// full. A drop is a warning, never an error: the next scan restates the
	// backlog.
	Dropped int
	// Duration is how long the scan took.
	Duration time.Duration
}

// DeliveryResult reports one webhook dispatch attempt series.
type DeliveryResult struct {
	// Items is how many review items the delivered digest carried.
	Items int
	// Duration is how long delivery took, including any retry.
	Duration time.Duration
}

// WebhookSink posts notifications as JSON to a single HTTP(S) endpoint.
type WebhookSink struct {
	url    string
	client *http.Client
}

// NewWebhookSink builds a webhook sink. The URL must be absolute http(s); a
// non-positive timeout is a configuration error rather than an unbounded
// request, because an unbounded webhook is exactly the failure this lane exists
// to prevent.
func NewWebhookSink(rawURL string, timeout time.Duration) (*WebhookSink, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("%w: no webhook URL", ErrNotifierUnavailable)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: parsing webhook URL: %w", ErrNotifierUnavailable, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: webhook URL scheme must be http or https, got %q", ErrNotifierUnavailable, parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("%w: webhook URL must include a host", ErrNotifierUnavailable)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("%w: webhook timeout must be positive, got %s", ErrNotifierUnavailable, timeout)
	}
	return &WebhookSink{
		url:    rawURL,
		client: &http.Client{Timeout: timeout},
	}, nil
}

// Publish posts the notification, retrying once on failure.
func (s *WebhookSink) Publish(ctx context.Context, notification ReviewNotification) error {
	if s == nil || s.client == nil {
		return ErrNotifierUnavailable
	}
	if ctx == nil {
		return ErrNotifierUnavailable
	}
	body, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("encoding review notification: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= webhookMaxAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("review notification cancelled: %w", ctx.Err())
			case <-time.After(webhookRetryDelay):
			}
		}
		lastErr = s.postOnce(ctx, body)
		if lastErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return lastErr
		}
	}
	return lastErr
}

// postOnce performs a single delivery attempt.
func (s *WebhookSink) postOnce(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building review notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "fi-fhir-terminology-review-notifier")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("posting review notification: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	// Drain so the connection can be reused; the receiver's body is not part of
	// the contract.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("review notification webhook returned %s", resp.Status)
	}
	return nil
}

// ReviewNotifierConfig configures the pending autoroute review notifier.
type ReviewNotifierConfig struct {
	// Store reads the review queue. Required.
	Store PendingAutorouteLister

	// Sink delivers notifications. Required.
	Sink NotificationSink

	// Interval is the scan cadence. Must be positive; callers that want
	// notifications disabled should not construct a notifier at all.
	Interval time.Duration

	// MinConfidence is the inclusive confidence floor for notification. Must be
	// within [0, 1].
	MinConfidence float64

	// ScanLimit bounds rows inspected per scan. Zero uses DefaultNotifyScanLimit.
	ScanLimit int

	// QueueSize bounds the dispatch queue. Zero uses the package default.
	QueueSize int

	// Observe, when set, is called once per scan with the result and any error.
	// It must not block.
	Observe func(NotifyResult, error)

	// ObserveDelivery, when set, is called once per dispatched notification with
	// the delivery outcome. It must not block.
	ObserveDelivery func(DeliveryResult, error)
}

// ReviewNotifier notifies an external webhook that high-confidence pending
// autoroutes are waiting on a human reviewer.
//
// Dispatch is asynchronous by construction. Notify hands work to a bounded
// queue and returns immediately, so no caller — including the pending-autoroute
// creation path, should it ever notify per event — can be slowed or failed by a
// hanging or erroring webhook. A full queue drops the notification; the next
// scan restates the whole eligible backlog, so a drop loses no durable state.
//
// The shipped driver is a periodic scan rather than a hook in the resolution
// path: it keeps the hot path untouched, and the digest it produces is what a
// reviewer actually needs (the current queue, not a single row). Notify is
// exported so a per-event hook can be added later without redesigning dispatch.
type ReviewNotifier struct {
	store         PendingAutorouteLister
	sink          NotificationSink
	interval      time.Duration
	minConfidence float64
	scanLimit     int
	queue         chan ReviewNotification

	observe         func(NotifyResult, error)
	observeDelivery func(DeliveryResult, error)

	mu        sync.Mutex
	seen      map[int64]struct{}
	seenOrder []int64
}

// NewReviewNotifier builds a review notifier. Missing collaborators or an
// out-of-range threshold are configuration errors, not silent no-ops.
func NewReviewNotifier(cfg ReviewNotifierConfig) (*ReviewNotifier, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("%w: no pending autoroute store", ErrNotifierUnavailable)
	}
	if cfg.Sink == nil {
		return nil, fmt.Errorf("%w: no notification sink", ErrNotifierUnavailable)
	}
	if cfg.Interval <= 0 {
		return nil, fmt.Errorf("%w: notify interval must be positive, got %s", ErrNotifierUnavailable, cfg.Interval)
	}
	if cfg.MinConfidence < 0 || cfg.MinConfidence > 1 {
		return nil, fmt.Errorf("%w: min confidence must be within [0,1], got %v", ErrNotifierUnavailable, cfg.MinConfidence)
	}
	scanLimit := cfg.ScanLimit
	if scanLimit <= 0 {
		scanLimit = DefaultNotifyScanLimit
	}
	queueSize := cfg.QueueSize
	if queueSize <= 0 {
		queueSize = defaultNotifyQueueSize
	}
	return &ReviewNotifier{
		store:           cfg.Store,
		sink:            cfg.Sink,
		interval:        cfg.Interval,
		minConfidence:   cfg.MinConfidence,
		scanLimit:       scanLimit,
		queue:           make(chan ReviewNotification, queueSize),
		observe:         cfg.Observe,
		observeDelivery: cfg.ObserveDelivery,
		seen:            make(map[int64]struct{}),
	}, nil
}

// Interval reports the configured scan cadence.
func (n *ReviewNotifier) Interval() time.Duration {
	if n == nil {
		return 0
	}
	return n.interval
}

// MinConfidence reports the configured confidence floor.
func (n *ReviewNotifier) MinConfidence() float64 {
	if n == nil {
		return 0
	}
	return n.minConfidence
}

// Notify enqueues a notification for asynchronous delivery.
//
// It never blocks and never fails: when the bounded queue is full the
// notification is dropped and false is returned. That contract is what makes it
// safe to call from any path, including pending-autoroute creation.
func (n *ReviewNotifier) Notify(notification ReviewNotification) bool {
	if n == nil || n.queue == nil {
		return false
	}
	select {
	case n.queue <- notification:
		return true
	default:
		return false
	}
}

// ScanOnce inspects the review queue once and enqueues a digest when there is
// anything new above the threshold.
//
// A store failure is returned to the caller; it never propagates to a producer
// of pending autoroutes, because nothing on the creation path calls this.
func (n *ReviewNotifier) ScanOnce(ctx context.Context) (NotifyResult, error) {
	if n == nil || n.store == nil || ctx == nil {
		return NotifyResult{}, ErrNotifierUnavailable
	}
	started := time.Now()

	minConfidence := n.minConfidence
	rows, _, err := n.store.ListPendingAutoroutes(ctx, db.ListPendingAutoroutesFilter{
		Status:        db.StatusPending,
		MinConfidence: &minConfidence,
		Limit:         n.scanLimit,
	})
	result := NotifyResult{Duration: time.Since(started)}
	if err != nil {
		return result, fmt.Errorf("listing pending autoroutes for review notification: %w", err)
	}

	eligible := make([]*db.PendingAutoroute, 0, len(rows))
	for _, row := range rows {
		// The store filter already applies the threshold; re-check so a store
		// implementation that ignores the filter cannot leak low-confidence
		// rows to an external endpoint.
		if row == nil || row.Confidence < n.minConfidence {
			continue
		}
		eligible = append(eligible, row)
	}
	result.Eligible = len(eligible)
	if len(eligible) == 0 {
		result.Duration = time.Since(started)
		return result, nil
	}

	fresh := n.markUnseen(eligible)
	result.New = len(fresh)
	if len(fresh) == 0 {
		// Nothing new since the last notification: stay silent rather than
		// re-paging reviewers on every tick.
		result.Duration = time.Since(started)
		return result, nil
	}

	items := make([]ReviewItem, 0, len(fresh))
	for _, row := range fresh {
		items = append(items, newReviewItem(row))
	}
	notification := ReviewNotification{
		Event:         ReviewEventName,
		GeneratedAt:   time.Now().UTC(),
		MinConfidence: n.minConfidence,
		NewCount:      len(items),
		EligibleCount: len(eligible),
		Items:         items,
	}
	if n.Notify(notification) {
		result.Queued = 1
	} else {
		result.Dropped = 1
	}
	result.Duration = time.Since(started)
	return result, nil
}

// Run scans immediately, then on every interval tick, until ctx is cancelled.
//
// A failing scan is reported through Observe and does not stop the loop: a
// transient database problem must not take down the server. Only cancellation
// ends Run, and it does so without error so a normal shutdown is not reported
// as a component failure.
func (n *ReviewNotifier) Run(ctx context.Context) error {
	if n == nil || n.store == nil || n.sink == nil || ctx == nil {
		return ErrNotifierUnavailable
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		n.dispatch(ctx)
	}()

	ticker := time.NewTicker(n.interval)
	defer ticker.Stop()
	for {
		result, err := n.ScanOnce(ctx)
		if n.observe != nil {
			n.observe(result, err)
		}
		select {
		case <-ctx.Done():
			wg.Wait()
			return nil
		case <-ticker.C:
		}
	}
}

// dispatch drains the queue until ctx is cancelled. Notifications still queued
// at shutdown are abandoned: they are advisory, and the next process restates
// the backlog on its first scan.
func (n *ReviewNotifier) dispatch(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case notification := <-n.queue:
			started := time.Now()
			err := n.sink.Publish(ctx, notification)
			if n.observeDelivery != nil {
				n.observeDelivery(DeliveryResult{
					Items:    len(notification.Items),
					Duration: time.Since(started),
				}, err)
			}
		}
	}
}

// markUnseen returns the rows that have not been notified before and records
// them as seen.
//
// De-duplication is by pending autoroute ID. A re-suggested mapping keeps its
// row (CreatePendingAutoroute upserts on the natural key), so it is notified
// once rather than on every re-resolution.
func (n *ReviewNotifier) markUnseen(rows []*db.PendingAutoroute) []*db.PendingAutoroute {
	n.mu.Lock()
	defer n.mu.Unlock()

	fresh := make([]*db.PendingAutoroute, 0, len(rows))
	for _, row := range rows {
		if _, ok := n.seen[row.ID]; ok {
			continue
		}
		n.seen[row.ID] = struct{}{}
		n.seenOrder = append(n.seenOrder, row.ID)
		fresh = append(fresh, row)
	}
	for len(n.seenOrder) > notifySeenCapacity {
		oldest := n.seenOrder[0]
		n.seenOrder = n.seenOrder[1:]
		delete(n.seen, oldest)
	}

	// Highest confidence first mirrors the store's ordering and puts the most
	// actionable item at the top of the digest.
	sort.SliceStable(fresh, func(i, j int) bool {
		return fresh[i].Confidence > fresh[j].Confidence
	})
	return fresh
}
