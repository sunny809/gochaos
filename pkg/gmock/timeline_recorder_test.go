package gmock

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/sunny809/gochaos/internal/spec"
)

// testFault returns a stub fault with probabilistic activation so tests can
// assert that synthesized events carry the deterministic (activation-stripped)
// copy.
func testFault() *spec.FaultDefinition {
	return &spec.FaultDefinition{
		Type:       "error",
		Activation: &spec.Activation{Probability: 0.5},
	}
}

func recorderRequest(method, path string) *http.Request {
	return httptest.NewRequest(method, path, nil)
}

// TestTimelineRecorder_ObserveReturnsPosition exercises the observe
// contract: it returns the request's position in the observed order (the
// post-increment counter value).
func TestTimelineRecorder_ObserveReturnsPosition(t *testing.T) {
	rec := newTimelineRecorder()
	for i := 1; i <= 5; i++ {
		if got := rec.observe(recorderRequest(http.MethodGet, "/x")); got != i {
			t.Fatalf("observe #%d: position = %d, want %d", i, got, i)
		}
	}
}

// TestTimelineRecorder_Events_Windows checks that events() synthesizes the
// at/until windows from fires recorded at known positions, with the
// first-match-wins adjustment, and that keys sort deterministically.
//
// Fires at observed positions 2,3,5: positions 2,3 are consecutive and
// collapse into a [2,3] window; the fire at 5 is adjusted down by the two
// fires of the earlier window (each fire consumes exactly one request on
// replay, so the replayed trigger is 5-2=3). Windows: [2,3] and [3,3].
func TestTimelineRecorder_Events_Windows(t *testing.T) {
	rec := newTimelineRecorder()
	get := recorderRequest(http.MethodGet, "/x")

	// Observe five requests, recording fires at positions 2, 3 and 5.
	positions := make([]int, 0, 5)
	for i := 0; i < 5; i++ {
		positions = append(positions, rec.observe(get))
	}
	for _, at := range []int{positions[1], positions[2], positions[4]} {
		rec.recordFire(get, testFault(), at)
	}

	// A second key exercises cross-key deterministic ordering (GET before POST).
	post := recorderRequest(http.MethodPost, "/y")
	rec.recordFire(post, testFault(), rec.observe(post))

	events := rec.events()
	if len(events) != 3 {
		t.Fatalf("events: got %d, want 3", len(events))
	}

	// First window: fires at 2 and 3 collapse to At=2, Until=3.
	ev := events[0]
	if ev.At == nil || ev.At.Request != 2 {
		t.Fatalf("events[0].At: got %+v, want request 2", ev.At)
	}
	if ev.Until == nil || ev.Until.Request != 3 {
		t.Fatalf("events[0].Until: got %+v, want request 3", ev.Until)
	}
	if ev.Match.Method != http.MethodGet || ev.Match.URLPath != "/x" {
		t.Fatalf("events[0].Match: got %+v, want GET /x", ev.Match)
	}
	if ev.Fault == nil || ev.Fault.Activation != nil {
		t.Fatalf("events[0].Fault: got %+v, want activation-stripped copy", ev.Fault)
	}

	// Second window: the fire at 5 adjusted by the two earlier fires.
	ev = events[1]
	if ev.At == nil || ev.At.Request != 3 {
		t.Fatalf("events[1].At: got %+v, want request 3 (5 - 2 consumed)", ev.At)
	}
	if ev.Until != nil {
		t.Fatalf("events[1].Until: got %+v, want nil", ev.Until)
	}

	// Third key's event comes last (POST sorts after GET).
	ev = events[2]
	if ev.Match.Method != http.MethodPost || ev.Match.URLPath != "/y" {
		t.Fatalf("events[2].Match: got %+v, want POST /y", ev.Match)
	}
	if ev.At == nil || ev.At.Request != 1 {
		t.Fatalf("events[2].At: got %+v, want request 1", ev.At)
	}
}

// TestTimelineRecorder_RecordFireUsesPassedPosition checks the recordFire
// contract: the fire is tagged with the passed position even when the
// counter has advanced past it (the buggy behavior read the counter at
// record time), and a fire without a prior observe still creates the key
// entry.
func TestTimelineRecorder_RecordFireUsesPassedPosition(t *testing.T) {
	rec := newTimelineRecorder()
	req := recorderRequest(http.MethodGet, "/x")

	// Three later requests observe and advance the counter to 3 before the
	// fire for the second request is recorded.
	rec.observe(req)
	rec.observe(req)
	rec.observe(req)
	rec.recordFire(req, testFault(), 2)

	// A fire on a fresh key with no prior observe must still record.
	fresh := recorderRequest(http.MethodGet, "/z")
	rec.recordFire(fresh, testFault(), 1)

	events := rec.events()
	if len(events) != 2 {
		t.Fatalf("events: got %d, want 2", len(events))
	}
	if events[0].At == nil || events[0].At.Request != 2 {
		t.Fatalf("events[0].At: got %+v, want request 2 (the passed position, not the counter 3)", events[0].At)
	}
	if events[1].At == nil || events[1].At.Request != 1 {
		t.Fatalf("events[1].At: got %+v, want request 1 (fire without prior observe)", events[1].At)
	}
}

// TestTimelineRecorder_ConcurrentObserveAndRecordFire exercises the recorder
// under concurrent observe/recordFire/events calls. Run with -race: any
// unsynchronized access to the recorder state fails the run. Fires are
// recorded at odd observed positions on per-goroutine keys, so each fire is
// its own window and the per-key triggers are exactly 1..perWorker/2.
func TestTimelineRecorder_ConcurrentObserveAndRecordFire(t *testing.T) {
	rec := newTimelineRecorder()

	const workers = 8
	const perWorker = 200

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			req := recorderRequest(http.MethodGet, "/x/"+string(rune('a'+w)))
			for i := 0; i < perWorker; i++ {
				at := rec.observe(req)
				if i%2 == 0 {
					rec.recordFire(req, testFault(), at)
				}
			}
		}(w)
	}

	// A concurrent reader of the synthesized artifact while writers run.
	stop := make(chan struct{})
	var readerWG sync.WaitGroup
	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		for {
			select {
			case <-stop:
				return
			default:
				rec.events()
			}
		}
	}()

	wg.Wait()
	close(stop)
	readerWG.Wait()

	// Fires at odd positions 1..199 on each key are non-consecutive, so each
	// synthesizes its own single-request window with trigger (p+1)/2.
	events := rec.events()
	if len(events) != workers*perWorker/2 {
		t.Fatalf("events: got %d, want %d", len(events), workers*perWorker/2)
	}
	for _, ev := range events {
		if ev.At == nil || ev.At.Request < 1 || ev.At.Request > perWorker/2 {
			t.Fatalf("event At: got %+v, want request in [1, %d]", ev.At, perWorker/2)
		}
		if ev.Until != nil {
			t.Fatalf("event Until: got %+v, want nil", ev.Until)
		}
	}
}
