package delivery

import (
	"context"
	"errors"
	"testing"
)

func TestDispatcherObservationPreservesLineageAndOutcome(t *testing.T) {
	for _, tc := range []struct {
		name              string
		idle, retry, fail bool
		want              Outcome
	}{
		{name: "published", want: OutcomePublished}, {name: "idle", idle: true, want: OutcomeIdle}, {name: "retry", retry: true, want: OutcomeRetry}, {name: "error", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := testWorkItem()
			store := &fakeStore{item: &item, retry: tc.retry}
			if tc.idle {
				store.item = nil
			}
			publisher := &fakePublisher{}
			if tc.retry {
				publisher.err = errors.New("PHI_PUBLISH_SENTINEL")
			}
			var transport DestinationTransport
			if tc.fail {
				transport = &stubDestinationTransport{owned: true, err: errors.New("PHI_TRANSPORT_SENTINEL")}
			}
			dispatcher, err := NewDispatcherWithDestination(store, publisher, "worker-a", testConfig(), nil, transport)
			if err != nil {
				t.Fatal(err)
			}
			completed := 0
			dispatcher.SetObserver(func(ctx context.Context, initial DispatchResult) (context.Context, func(DispatchResult)) {
				if !tc.idle && (initial.ReceiptID != item.ReceiptID || initial.TraceID != item.TraceID || initial.AttemptID != item.AttemptID) {
					t.Errorf("lost lineage: %+v", initial)
				}
				if tc.idle && initial.AttemptID != "" {
					t.Error("idle observation has an attempt")
				}
				return ctx, func(result DispatchResult) {
					completed++
					if result.Outcome != tc.want || result.Failed != tc.fail || result.Duration <= 0 {
						t.Errorf("unexpected result %+v", result)
					}
				}
			})
			_, err = dispatcher.RunOnce(context.Background())
			if (err != nil) != tc.fail {
				t.Fatalf("error: %v", err)
			}
			if completed != 1 {
				t.Fatalf("completed %d observations", completed)
			}
		})
	}
}
