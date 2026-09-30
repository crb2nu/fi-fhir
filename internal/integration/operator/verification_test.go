package operator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
)

func TestVerificationReadsRequireTheReadRoleBeforeTheDatabase(t *testing.T) {
	// newTestService's database handle points at a closed port: a read that got
	// past authorization would fail with a connection error, not ErrForbidden.
	service, _, _ := newTestService(t)
	window := StatisticsRequest{
		From:   time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC),
		To:     time.Date(2026, 9, 29, 1, 0, 0, 0, time.UTC),
		Bucket: StatisticsBucketHour,
	}
	forbidden := requestsecurity.WithSecurityContext(context.Background(),
		securityContext(testTenant, delivery.OperatorRole, DeploymentOperatorRole))
	if _, err := service.ListCanonicalEvents(forbidden, CanonicalEventFilter{}, PageRequest{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListCanonicalEvents without %s = %v, want %v", ReadRole, err, ErrForbidden)
	}
	if _, err := service.AdmissionStatistics(forbidden, window); !errors.Is(err, ErrForbidden) {
		t.Fatalf("AdmissionStatistics without %s = %v, want %v", ReadRole, err, ErrForbidden)
	}
	otherTenant := requestsecurity.WithSecurityContext(context.Background(), securityContext("tenant-b", ReadRole))
	if _, err := service.ListCanonicalEvents(otherTenant, CanonicalEventFilter{}, PageRequest{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListCanonicalEvents for another tenant = %v, want %v", err, ErrForbidden)
	}
	if _, err := service.AdmissionStatistics(context.Background(), window); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("AdmissionStatistics without identity = %v, want %v", err, ErrUnauthenticated)
	}
}

func TestVerificationReadsRefuseInvalidRequestsBeforeTheDatabase(t *testing.T) {
	service, _, _ := newTestService(t)
	ctx := requestsecurity.WithSecurityContext(context.Background(), securityContext(testTenant, ReadRole))
	from := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	earlier := from.Add(-time.Hour)

	filters := map[string]CanonicalEventFilter{
		"padded event type":   {EventType: " patient_admit"},
		"control character":   {ReceiptID: "receipt\x00a"},
		"oversized source id": {SourceMessageID: strings.Repeat("m", 257)},
		"inverted window":     {From: &from, To: &earlier},
	}
	for name, filter := range filters {
		if _, err := service.ListCanonicalEvents(ctx, filter, PageRequest{}); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: ListCanonicalEvents = %v, want %v", name, err, ErrInvalidRequest)
		}
	}
	if _, err := service.ListCanonicalEvents(ctx, CanonicalEventFilter{}, PageRequest{Cursor: "not-a-cursor"}); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("forged cursor: ListCanonicalEvents = %v, want %v", err, ErrInvalidRequest)
	}

	windows := map[string]StatisticsRequest{
		"empty window":        {From: from, To: from, Bucket: StatisticsBucketHour},
		"inverted window":     {From: from, To: earlier, Bucket: StatisticsBucketHour},
		"missing from":        {To: from, Bucket: StatisticsBucketHour},
		"unknown bucket":      {From: earlier, To: from, Bucket: "week"},
		"too many hours":      {From: from, To: from.Add(time.Duration(MaxStatisticsBuckets+1) * time.Hour), Bucket: StatisticsBucketHour},
		"too many days":       {From: from, To: from.AddDate(0, 0, MaxStatisticsBuckets+1), Bucket: StatisticsBucketDay},
		"no bucket specified": {From: earlier, To: from},
	}
	for name, window := range windows {
		if _, err := service.AdmissionStatistics(ctx, window); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: AdmissionStatistics = %v, want %v", name, err, ErrInvalidRequest)
		}
	}
}

func TestStatisticsWindowBucketsAreUTCAndCoverTheWholeWindow(t *testing.T) {
	eastern := time.FixedZone("UTC-4", -4*60*60)
	cases := []struct {
		name   string
		window StatisticsRequest
		starts []time.Time
	}{
		{
			name: "hours from a mid-hour start",
			window: StatisticsRequest{
				From:   time.Date(2026, 9, 29, 10, 30, 0, 0, time.UTC),
				To:     time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC),
				Bucket: StatisticsBucketHour,
			},
			starts: []time.Time{
				time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC),
				time.Date(2026, 9, 29, 11, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "days from a non-UTC caller",
			window: StatisticsRequest{
				// 2026-09-28 22:00 at UTC-4 is 2026-09-29 02:00 UTC.
				From:   time.Date(2026, 9, 28, 22, 0, 0, 0, eastern),
				To:     time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
				Bucket: StatisticsBucketDay,
			},
			starts: []time.Time{
				time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC),
				time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "exactly the bucket ceiling",
			window: StatisticsRequest{
				From:   time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
				To:     time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Add(MaxStatisticsBuckets * time.Hour),
				Bucket: StatisticsBucketHour,
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			normalized, starts, err := testCase.window.normalize()
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			if normalized.From.Location() != time.UTC || normalized.To.Location() != time.UTC {
				t.Fatalf("window not normalized to UTC: %v – %v", normalized.From, normalized.To)
			}
			if testCase.starts == nil {
				if len(starts) != MaxStatisticsBuckets {
					t.Fatalf("buckets = %d, want %d", len(starts), MaxStatisticsBuckets)
				}
				return
			}
			if len(starts) != len(testCase.starts) {
				t.Fatalf("starts = %v, want %v", starts, testCase.starts)
			}
			for index := range starts {
				if !starts[index].Equal(testCase.starts[index]) || starts[index].Location() != time.UTC {
					t.Fatalf("start %d = %v, want %v", index, starts[index], testCase.starts[index])
				}
			}
		})
	}
}
