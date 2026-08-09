package mllp

import (
	"bufio"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// declaredRate is the deployment-wide rate the revision declares. Everything
// below is expressed against it so the arithmetic in the failure messages is
// readable rather than magic.
const declaredRate = 100

// TestMLLPCapacity_TwoReplicasAdmitTwiceTheDeclaredRateToday is Lane S5-D's
// day-1 gate. It must PASS on unmodified main.
//
// docs/operations/PRODUCTION-MLLP.md:51-59 documents the per-replica multiple
// as intended behaviour rather than a defect, and pkg/integration/deployment.go
// repeats the claim in CapacityPolicy's doc comment. Both are prose. This test
// converts the prose into a number: two replicas of one deployment that
// declares max_messages_per_second: 100 admit ~200 messages in a measured
// second, because capacityGate (capacity.go:17-26) is one in-process struct per
// Service (service.go:122) with no cross-process state.
//
// Passing here is the point. It proves the N x consequence is real rather than
// documented-in-theory, and it quantifies exactly what slice 4.4b's budget 2
// would have measured on the two-replica reference profile had it been measured
// before the deployment-wide bucket exists: a number certifying nothing.
//
// It is also the lane's negative control, and it needs no build tag to be one.
// The replicas here are built with no RateQuota bound, which is exactly the
// "deployment-wide claim reverted" configuration: the same admission path, the
// same gate, the quota removed. So the control runs in every invocation
// alongside TestQuotaBoundsTheDeploymentRateAcrossTwoReplicas, which drives the
// identical shape with the coordinator bound and asserts <=100. A regression
// that quietly stopped consulting the quota would turn the second test red
// while this one stayed green, which is what a control is for.
func TestMLLPCapacity_TwoReplicasAdmitTwiceTheDeclaredRateToday(t *testing.T) {
	source := testSource(t)
	binding := testBinding(source)
	// In-flight and queue bounds are deliberately generous: this test measures
	// the rate gate, and a queue rejection would silently understate it.
	binding.Deployment.Capacity = integration.CapacityPolicy{
		MaxInFlight:          declaredRate * 4,
		MaxQueued:            declaredRate * 8,
		MaxMessagesPerSecond: declaredRate,
	}
	if err := binding.Deployment.Validate(); err != nil {
		t.Fatalf("the declared policy must be one the product accepts: %v", err)
	}

	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	var resolverCalls atomic.Int64
	replicaA := newCapacityReplica(t, source, binding, clock, &resolverCalls)
	replicaB := newCapacityReplica(t, source, binding, clock, &resolverCalls)

	// Phase 1 — the instantaneous burst. The bucket is seeded full on a
	// replica's first frame and burst equals the refill rate, so each replica
	// can admit the whole declared rate before any time passes. With no quota
	// bound, "the whole declared rate" is the deployment's, twice over.
	burstA := drainRateBudget(t, replicaA)
	burstB := drainRateBudget(t, replicaB)
	if burstA != declaredRate || burstB != declaredRate {
		t.Fatalf("each replica should burst the full declared rate independently: got %d and %d, want %d each",
			burstA, burstB, declaredRate)
	}
	if burstA+burstB <= declaredRate {
		t.Fatalf("aggregate burst %d did not exceed the declared %d: the deployment-wide bound already exists",
			burstA+burstB, declaredRate)
	}

	// Phase 2 — one measured second of steady state, driven by the injected
	// clock in ten 100ms slices so the continuous refill at capacity.go:119-123
	// is exercised rather than the initial burst.
	const slices = 10
	admitted := 0
	for i := 0; i < slices; i++ {
		now = now.Add(time.Second / slices)
		admitted += drainRateBudget(t, replicaA)
		admitted += drainRateBudget(t, replicaB)
	}

	// The bound slice 4.4e will introduce. Asserted as explicitly NOT observed,
	// so this test cannot pass for the wrong reason once the bucket is durable.
	if admitted <= declaredRate {
		t.Fatalf("two replicas admitted %d msg in one second against a declared %d/s: "+
			"a deployment-wide bound is already in force and this gate has nothing to prove",
			admitted, declaredRate)
	}
	// The bound that is actually in force today: one per replica.
	const want = 2 * declaredRate
	const tolerance = want / 50 // 2%
	if admitted < want-tolerance || admitted > want+tolerance {
		t.Fatalf("two replicas admitted %d msg in one second against a declared %d/s; "+
			"expected ~%d (N x the declared rate, N=2), tolerance %d",
			admitted, declaredRate, want, tolerance)
	}
	t.Logf("two replicas admitted %d msg/s against a declared %d msg/s (%0.1fx); "+
		"slice 4.4b budget 2 measured on this profile today would certify nothing",
		admitted, declaredRate, float64(admitted)/float64(declaredRate))

	// Capacity contributes zero queries. The only per-frame lookup is the
	// binding resolution that lifecycle/queries.go:192 already performs, so a
	// resolver call count equal to the attempt count proves the admission
	// decision reached no store. This is also the shape of the post-slice
	// assertion that admission must stay in-memory.
	attempts := int64(burstA + burstB + admitted)
	if calls := resolverCalls.Load(); calls < attempts {
		t.Fatalf("resolver was called %d times for %d admitted frames; accounting is wrong", calls, attempts)
	}
}

// TestMLLPCapacity_DurableRateStateLivesInExactlyOneLedger is the day-1 gate's
// other half, inverted by the slice that made it false.
//
// Before 4.4e it asserted that *no* migration in the repo carried rate state —
// PostgreSQL was read-only with respect to MLLP capacity, and the gate proved
// it mechanically. Lifecycle migration 0002 makes that false by construction,
// which was always the point: the assertion is the marker, and flipping it is
// the evidence the slice landed.
//
// Kept rather than deleted, and inverted rather than weakened, because the walk
// still earns its place: it pins the durable rate state to exactly one file in
// exactly one ledger. A second ledger sprouting a rate table, or a per-frame
// counter appearing beside the lease, turns this red — and a per-frame counter
// is the design this lane explicitly rejected, so it is worth a tripwire.
func TestMLLPCapacity_DurableRateStateLivesInExactlyOneLedger(t *testing.T) {
	const home = "internal/integration/lifecycle/migrations/0002_mllp_rate_claims.sql"
	carriers := migrationsCarryingRateState(t)
	if len(carriers) != 1 || carriers[0] != home {
		t.Fatalf("durable rate state lives in %v, want exactly [%s]", carriers, home)
	}
	t.Logf("durable rate state is confined to %s", home)
}

// migrationsCarryingRateState walks every ledger's migrations and returns, in a
// stable order, those that mention durable rate state.
func migrationsCarryingRateState(t *testing.T) []string {
	t.Helper()
	root := repoRoot(t)
	rateState := regexp.MustCompile(`(?i)rate_limit|token_bucket|capacity_counter|rate_quota|rate_claim|` +
		`message_rate|messages_per_second|deployment_quota|admission_rate`)

	scanned := 0
	var carriers []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", ".tmp", ".worktrees":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".sql" || !strings.Contains(filepath.ToSlash(path), "/migrations/") {
			return nil
		}
		scanned++
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if rateState.Match(body) {
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				relative = path
			}
			carriers = append(carriers, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk migrations: %v", err)
	}
	if scanned == 0 {
		t.Fatal("scanned zero migration files: the walk is wrong, not the repo")
	}
	sort.Strings(carriers)
	t.Logf("scanned %d migration files across every ledger", scanned)
	return carriers
}

// TestMLLPCapacity_RateLimitedFrameGetsATransientNAKAndTheConnectionSurvives
// establishes the end-to-end contract that slice 4.4e must preserve, against
// the existing in-memory gate, before the mechanism underneath it changes.
//
// Nothing asserted this today: grep RATE_EXCEEDED over the package's tests
// returns zero hits, and capacity_test.go exercises the gate in isolation with
// a frozen clock. A durable bucket must not be the first code to establish the
// contract it is supposed to preserve — that is the whole content of this
// lane's riskiest assumption, so the test lands before the mechanism does.
//
// What must hold, in both acknowledgement modes:
//   - the code is AE (application) or CE (commit), never AR/CR, which
//     server.go:198-201 reserves for permanent rejects;
//   - the ERR segment carries RATE_EXCEEDED;
//   - the TCP connection stays open and the very next frame on it is served;
//   - once the bucket refills, the same connection admits again.
func TestMLLPCapacity_RateLimitedFrameGetsATransientNAKAndTheConnectionSurvives(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		mode       AcknowledgementMode
		accepted   string
		transient  string
		permanent  string
		permanent2 string
	}{
		{name: "application mode", mode: AcknowledgementModeApplication, accepted: "AA", transient: "AE", permanent: "AR", permanent2: "CR"},
		{name: "commit mode", mode: AcknowledgementModeCommit, accepted: "CA", transient: "CE", permanent: "AR", permanent2: "CR"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			source := acknowledgementModeSource(t, testCase.mode)
			binding := testBinding(source)
			// One message per second: the second frame on the connection is
			// rate-limited with no wall-clock dependence worth worrying about,
			// because refilling a single token takes a full second.
			binding.Deployment.Capacity = integration.CapacityPolicy{MaxInFlight: 2, MaxQueued: 4, MaxMessagesPerSecond: 1}
			if err := binding.Deployment.Validate(); err != nil {
				t.Fatalf("the declared policy must be one the product accepts: %v", err)
			}

			// Real time plus a test-controlled offset. A frozen clock cannot be
			// used here: server.go:153,159,193 derive the socket deadlines from
			// the same clock, and a frozen one sets every deadline in the past.
			var offset atomic.Int64
			clock := func() time.Time { return time.Now().Add(time.Duration(offset.Load())) }

			server := newAcknowledgementServer(t, source, binding, clock)
			address, stop := serveTestServer(t, server)
			defer stop()

			connection, err := net.Dial("tcp", address)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = connection.Close() }()
			reader := bufio.NewReader(connection)

			code, errorCode := exchangeFrame(t, connection, reader, source, "CTRL1")
			if code != testCase.accepted {
				t.Fatalf("first frame: got %s (%s), want %s", code, errorCode, testCase.accepted)
			}

			// The rate-limited frame.
			code, errorCode = exchangeFrame(t, connection, reader, source, "CTRL2")
			if code == testCase.permanent || code == testCase.permanent2 {
				t.Fatalf("rate limiting produced a permanent reject %s: a throttled sender must be told to retry", code)
			}
			if code != testCase.transient {
				t.Fatalf("rate-limited frame: got %s, want the transient code %s", code, testCase.transient)
			}
			if errorCode != "RATE_EXCEEDED" {
				t.Fatalf("rate-limited frame carried error code %q, want RATE_EXCEEDED", errorCode)
			}

			// The connection must still be usable. If it were closed, this
			// exchange would fail on the write or the read rather than
			// returning another acknowledgement.
			code, errorCode = exchangeFrame(t, connection, reader, source, "CTRL3")
			if code != testCase.transient || errorCode != "RATE_EXCEEDED" {
				t.Fatalf("connection did not survive the NAK: third frame got %s (%s)", code, errorCode)
			}

			// And it must recover on the same connection once the bucket
			// refills, rather than needing a reconnect.
			offset.Add(int64(2 * time.Second))
			code, errorCode = exchangeFrame(t, connection, reader, source, "CTRL4")
			if code != testCase.accepted {
				t.Fatalf("after the bucket refilled the same connection got %s (%s), want %s",
					code, errorCode, testCase.accepted)
			}
		})
	}
}

// newCapacityReplica builds one MLLP replica of the same deployment. Two of
// these share nothing: that is the defect the day-1 gate quantifies.
func newCapacityReplica(
	t *testing.T,
	source SourceRevision,
	binding lifecycle.RunnableBinding,
	clock func() time.Time,
	resolverCalls *atomic.Int64,
) *Service {
	t.Helper()
	config := testServiceConfig(source,
		resolverFunc(func(context.Context, string, string) (lifecycle.RunnableBinding, error) {
			resolverCalls.Add(1)
			return binding, nil
		}),
		processorFunc(func(_ context.Context, request integration.ProcessRequest) (integration.ProcessResult, error) {
			return acceptedResult(request), nil
		}),
	)
	config.Clock = clock
	service, err := NewService(config)
	if err != nil {
		t.Fatalf("build replica: %v", err)
	}
	return service
}

// drainRateBudget admits frames until the rate gate refuses, and returns how
// many were admitted. It fails rather than looping if the gate never refuses,
// and it fails if the refusal is a queue rejection wearing a rate rejection's
// clothes.
func drainRateBudget(t *testing.T, service *Service) int {
	t.Helper()
	const ceiling = declaredRate * 10
	for admitted := 0; admitted <= ceiling; admitted++ {
		_, err := service.Submit(context.Background(), ConnectionIdentity{}, testHL7("CTRL1"))
		if err == nil {
			continue
		}
		if errors.Is(err, ErrRateExceeded) {
			return admitted
		}
		t.Fatalf("after %d admissions the gate refused with %v, want %v", admitted, err, ErrRateExceeded)
	}
	t.Fatalf("the rate gate admitted more than %d frames without refusing", ceiling)
	return 0
}

func acknowledgementModeSource(t *testing.T, mode AcknowledgementMode) SourceRevision {
	t.Helper()
	revision, err := NewSourceRevision(SourceRevisionInput{
		ArtifactID: "mllp-source", RevisionID: "source-v1", SourceID: "hospital-a",
		ListenAddress: "127.0.0.1:2575", Encoding: "utf-8",
		Framing:  FramingPolicy{StartByte: StandardStartByte, EndByte: StandardEndByte, TrailerByte: StandardTrailerByte},
		Timeouts: TimeoutPolicy{ReadSeconds: 2, WriteSeconds: 2, IdleSeconds: 3, ProcessSeconds: 2},
		TLS:      TLSPolicy{Mode: TLSModeDisabled}, Clients: ClientPolicy{AllowedCIDRs: []string{"127.0.0.0/8"}},
		Acknowledgements: AcknowledgementPolicy{Mode: mode, IncludeErrorSegment: true},
		MaxMessageBytes:  4096, MaxConnections: 4,
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	return revision
}

func newAcknowledgementServer(
	t *testing.T,
	source SourceRevision,
	binding lifecycle.RunnableBinding,
	clock func() time.Time,
) *Server {
	t.Helper()
	config := testServiceConfig(source,
		resolverFunc(func(context.Context, string, string) (lifecycle.RunnableBinding, error) { return binding, nil }),
		processorFunc(func(_ context.Context, request integration.ProcessRequest) (integration.ProcessResult, error) {
			return acceptedResult(request), nil
		}),
	)
	config.Clock = clock
	server, err := NewServer(ServerConfig{Service: config})
	if err != nil {
		t.Fatalf("build server: %v", err)
	}
	return server
}

// exchangeFrame writes one framed HL7v2 message and reads the acknowledgement,
// returning the MSA code and the ERR segment's error code.
func exchangeFrame(
	t *testing.T,
	connection net.Conn,
	reader *bufio.Reader,
	source SourceRevision,
	controlID string,
) (string, string) {
	t.Helper()
	framed, err := framePayload(testHL7(controlID), source.Framing)
	if err != nil {
		t.Fatalf("frame %s: %v", controlID, err)
	}
	if err := connection.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Write(framed); err != nil {
		t.Fatalf("write %s: %v", controlID, err)
	}
	if err := connection.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	payload, err := readFrame(reader, source.Framing, source.MaxMessageBytes)
	if err != nil {
		t.Fatalf("read acknowledgement for %s: %v", controlID, err)
	}
	return acknowledgementCodeFromPayload(t, payload), errorCodeFromPayload(payload)
}

// errorCodeFromPayload reads the ERR segment's identifier, which
// protocol.go:205-207 encodes as code^code^FI-FHIR. An accepted
// acknowledgement carries no ERR segment and yields the empty string.
func errorCodeFromPayload(payload []byte) string {
	for _, segment := range strings.Split(string(payload), "\r") {
		if !strings.HasPrefix(segment, "ERR|") {
			continue
		}
		fields := strings.Split(segment, "|")
		if len(fields) < 4 {
			return ""
		}
		return strings.Split(fields[3], "^")[0]
	}
	return ""
}

func repoRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("no go.mod above the test's working directory")
		}
		directory = parent
	}
}
