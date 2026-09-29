package operator

import (
	"context"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// Verification reads (.loom/42 E-2): the durable admissions an operator
// verifies against, browsed and counted from columns. Like the rest of this
// package they never return a canonical event payload value: a row carries its
// identifiers, its receipt, the revisions that admitted it, its retention
// marks, and the structural field projection the trace uses (payload.go).

// StatisticsBucket is the width of one time-series bucket.
type StatisticsBucket string

const (
	// StatisticsBucketHour groups by UTC hour.
	StatisticsBucketHour StatisticsBucket = "hour"
	// StatisticsBucketDay groups by UTC calendar day.
	StatisticsBucketDay StatisticsBucket = "day"

	// MaxStatisticsBuckets bounds one statistics window: a month of hours or
	// two years of days. A wider window is refused, never silently clipped.
	MaxStatisticsBuckets = 744
	// MaxStatisticsGroups bounds each grouped count (by event type, by
	// definition, by destination). Groups are ordered by count, largest first;
	// AdmissionStatistics.GroupsTruncated says when a list was cut.
	MaxStatisticsGroups = MaxPageSize
)

// CanonicalEventFilter bounds a canonical event browse. Every field is an
// exact match on a column; From/To bound recorded_at inclusively.
// IncludePurged admits rows whose payload retention has tombstoned; by
// default the browse shows only rows whose payload is intact.
type CanonicalEventFilter struct {
	EventType       string
	DefinitionID    string
	ReceiptID       string
	SourceMessageID string
	CorrelationID   string
	From            *time.Time
	To              *time.Time
	IncludePurged   bool
}

// CanonicalEventRecord is one durable admission: a canonical event joined to
// the receipt that admitted it and the revisions that produced it.
type CanonicalEventRecord struct {
	Event EventSummary
	// ReceiptStatus is the admitting receipt's status (accepted or rejected).
	ReceiptStatus string
	// Definition is the integration definition revision the receipt recorded.
	Definition integration.ArtifactRevisionRef
	// Source is the source connection revision from the event's lineage, or
	// nil when the event has no lineage row.
	Source *integration.ArtifactRevisionRef
}

// StatisticsRequest is one half-open [From, To) window and its bucket width.
type StatisticsRequest struct {
	From   time.Time
	To     time.Time
	Bucket StatisticsBucket
}

// KeyCount is one grouped count.
type KeyCount struct {
	Key   string
	Count int
}

// DefinitionAdmissions counts receipts per integration definition revision.
type DefinitionAdmissions struct {
	DefinitionID string
	RevisionID   string
	Accepted     int
	Rejected     int
}

// DestinationAttempts counts delivery attempts per destination by status.
type DestinationAttempts struct {
	DestinationArtifactID string
	Queued                int
	Succeeded             int
	Failed                int
}

// StatisticsBucketCounts is one bucket of the admission time series.
// Receipts are bucketed by their recorded_at, attempts by theirs (when the
// attempt was created), each carrying its current status.
type StatisticsBucketCounts struct {
	Start     time.Time
	Accepted  int
	Rejected  int
	Queued    int
	Succeeded int
	Failed    int
}

// AdmissionStatistics is every count of one window, read in one snapshot.
type AdmissionStatistics struct {
	From   time.Time
	To     time.Time
	Bucket StatisticsBucket

	AcceptedReceipts int
	RejectedReceipts int

	CanonicalEvents int
	// PurgedEvents were tombstoned by retention (purged_at set).
	PurgedEvents int
	// ScheduledForPurge carry a retention deadline and are still intact.
	ScheduledForPurge int

	QueuedAttempts    int
	SucceededAttempts int
	FailedAttempts    int

	EventsByType          []KeyCount
	ReceiptsByDefinition  []DefinitionAdmissions
	AttemptsByDestination []DestinationAttempts
	// GroupsTruncated is true when any grouped list reached
	// MaxStatisticsGroups and was cut; the totals above are never cut.
	GroupsTruncated bool

	Series []StatisticsBucketCounts
}

// ListCanonicalEvents browses the tenant's durable admissions newest first.
func (s *Service) ListCanonicalEvents(
	ctx context.Context,
	filter CanonicalEventFilter,
	page PageRequest,
) (Page[CanonicalEventRecord], error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return Page[CanonicalEventRecord]{}, err
	}
	return s.reads.ListCanonicalEvents(ctx, security.TenantID, filter, page)
}

// AdmissionStatistics counts the tenant's admissions and delivery attempts
// over one window.
func (s *Service) AdmissionStatistics(ctx context.Context, request StatisticsRequest) (AdmissionStatistics, error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return AdmissionStatistics{}, err
	}
	return s.reads.AdmissionStatistics(ctx, security.TenantID, request)
}

func (f CanonicalEventFilter) validate() error {
	if !optionalToken(f.EventType, 128) || !optionalToken(f.DefinitionID, 256) ||
		!optionalToken(f.ReceiptID, 256) || !optionalToken(f.SourceMessageID, 256) ||
		!optionalToken(f.CorrelationID, 256) {
		return ErrInvalidRequest
	}
	return validWindow(f.From, f.To)
}

// normalize validates the window and returns it in UTC with its bucket starts.
// The first bucket starts at From truncated to the bucket width, so a window
// that begins mid-hour still labels that hour's bucket by the hour.
func (r StatisticsRequest) normalize() (StatisticsRequest, []time.Time, error) {
	if r.From.IsZero() || r.To.IsZero() || !r.To.After(r.From) {
		return StatisticsRequest{}, nil, ErrInvalidRequest
	}
	if r.Bucket != StatisticsBucketHour && r.Bucket != StatisticsBucketDay {
		return StatisticsRequest{}, nil, ErrInvalidRequest
	}
	normalized := StatisticsRequest{From: r.From.UTC(), To: r.To.UTC(), Bucket: r.Bucket}
	starts := make([]time.Time, 0, 32)
	for start := truncateToBucket(normalized.From, r.Bucket); start.Before(normalized.To); start = nextBucket(start, r.Bucket) {
		if len(starts) == MaxStatisticsBuckets {
			return StatisticsRequest{}, nil, ErrInvalidRequest
		}
		starts = append(starts, start)
	}
	return normalized, starts, nil
}

func truncateToBucket(value time.Time, bucket StatisticsBucket) time.Time {
	value = value.UTC()
	if bucket == StatisticsBucketDay {
		return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
	}
	return value.Truncate(time.Hour)
}

func nextBucket(start time.Time, bucket StatisticsBucket) time.Time {
	if bucket == StatisticsBucketDay {
		return start.AddDate(0, 0, 1)
	}
	return start.Add(time.Hour)
}
