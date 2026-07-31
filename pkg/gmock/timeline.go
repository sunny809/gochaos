package gmock

import (
	"fmt"
	"net/http"
	"sort"
	"sync"

	"github.com/sunny809/gochaos/internal/spec"
	"gopkg.in/yaml.v3"
)

// LoadTimelineYAML loads a fault timeline from YAML (declare or replay).
func (s *mockServer) LoadTimelineYAML(data []byte) error {
	var tl FaultTimeline
	if err := yaml.Unmarshal(data, &tl); err != nil {
		return fmt.Errorf("gmock: invalid timeline YAML: %w", err)
	}
	return s.LoadTimeline(&tl)
}

// LoadTimeline installs a fault timeline from a struct (declare or replay).
// The recorder is cleared so record mode starts fresh: recorded fires from a
// previous timeline must not bleed into the new run's export.
func (s *mockServer) LoadTimeline(tl *FaultTimeline) error {
	if err := s.timelineRunner.Load(tl); err != nil {
		return fmt.Errorf("gmock: invalid timeline: %w", err)
	}
	s.timelineRecord.clear()
	return nil
}

// ExportTimeline returns a fault timeline artifact describing every event
// that fired so far (record). Triggers are request-keyed, so the artifact can
// be loaded into another server for deterministic replay. The artifact
// combines the runner's record of declared timeline events with the recorded
// stub-driven fault fires (see timelineRecorder), so a probabilistic chaos
// run can be exported and replayed exactly.
func (s *mockServer) ExportTimeline() (*FaultTimeline, error) {
	tl := s.timelineRunner.Export()
	tl.Events = append(tl.Events, s.timelineRecord.events()...)
	return tl, nil
}

// --- record mode: stub-driven fires ---

// recordedFire is one stub-driven fault fire: the recorder counter value
// (1-based) at which the fault fired, and the fault definition.
type recordedFire struct {
	at    int
	fault *spec.FaultDefinition
}

// recordedKey is the per-(method, path) state of the recorder.
type recordedKey struct {
	method string
	path   string
	count  int // matching requests observed (mirrors the runner's counter)
	fires  []recordedFire
}

// timelineRecorder captures stub-driven fault fires so that ExportTimeline
// can serialize them into a replayable artifact (record mode). It mirrors
// the timeline runner's first-match-wins counter semantics: the per-key
// counter advances on every matching request that no timeline event
// consumed, and each fire is tagged with the request's observed position —
// the value observe returned for that request — so replay counters align
// exactly even under concurrency.
type timelineRecorder struct {
	mu   sync.Mutex
	keys map[string]*recordedKey
}

func newTimelineRecorder() *timelineRecorder {
	return &timelineRecorder{keys: make(map[string]*recordedKey)}
}

// observe advances the per-key counter for a request that was not consumed
// by a timeline event and returns the request's position in the observed
// order (the post-increment counter value). Called once per request, before
// stub matching completes, mirroring the runner's counter behavior on
// replay. The returned position must be passed to recordFire for the same
// request: observe and recordFire are separated by WriteResponse, which
// applies the stub's delay first, so reading the counter at recordFire time
// could tag the fire with a later request's position under concurrency.
func (r *timelineRecorder) observe(req *http.Request) int {
	key := req.Method + "\x00" + req.URL.Path
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.keys[key]
	if k == nil {
		k = &recordedKey{method: req.Method, path: req.URL.Path}
		r.keys[key] = k
	}
	k.count++
	return k.count
}

// recordFire records a stub-driven fault fire at the request's observed
// position: the value observe returned for that request. The position is
// passed in because the fire is recorded after the response — and its
// delay — has been written, by which time the counter may have advanced
// past this request under concurrency.
func (r *timelineRecorder) recordFire(req *http.Request, fault *spec.FaultDefinition, at int) {
	key := req.Method + "\x00" + req.URL.Path
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.keys[key]
	if k == nil {
		// No prior observe for this key: the recorder was cleared (Reset)
		// between observe and recordFire, so the observed position belongs
		// to the previous epoch. Dropping the fire keeps the new epoch's
		// export free of phantom events at stale positions.
		return
	}
	k.fires = append(k.fires, recordedFire{at: at, fault: fault})
}

// clear drops all observed state (used by Reset).
func (r *timelineRecorder) clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keys = make(map[string]*recordedKey)
}

// events synthesizes timeline events from the recorded fires. Consecutive
// fires collapse into a single at/until window (matching the runner's export
// convention). Triggers are adjusted for the runner's first-match-wins
// semantics so replay is exact: each event's counter counts only the
// matching requests it actually sees, and requests consumed by earlier
// events' fires do not advance it, so the trigger of a window is its first
// fire index minus the fires of all earlier windows.
func (r *timelineRecorder) events() []TimelineEvent {
	r.mu.Lock()
	defer r.mu.Unlock()

	keys := make([]*recordedKey, 0, len(r.keys))
	for _, k := range r.keys {
		keys = append(keys, k)
	}
	// Deterministic artifact order across keys.
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].method != keys[j].method {
			return keys[i].method < keys[j].method
		}
		return keys[i].path < keys[j].path
	})

	var events []TimelineEvent
	for _, k := range keys {
		if len(k.fires) == 0 {
			continue
		}
		// Fires are recorded in completion order, which can differ from
		// observe order when stubs delay responses under concurrency — sort
		// by position so each window carries the fault of the fire at its
		// actual observed position (otherwise replay injects the wrong fault
		// at the wrong positions).
		sort.SliceStable(k.fires, func(i, j int) bool { return k.fires[i].at < k.fires[j].at })

		consumed := 0 // fires of earlier windows (each consumes one request)
		for i := 0; i < len(k.fires); {
			j := i
			for j+1 < len(k.fires) && k.fires[j+1].at == k.fires[j].at+1 {
				j++
			}
			at := k.fires[i].at - consumed
			var until *TimelineTrigger
			if j > i {
				until = &TimelineTrigger{Request: k.fires[j].at - consumed}
			}
			events = append(events, TimelineEvent{
				At:    &TimelineTrigger{Request: at},
				Until: until,
				Match: spec.RequestPattern{Method: k.method, URLPath: k.path},
				Fault: deterministicFault(k.fires[i].fault),
			})
			consumed += j - i + 1
			i = j + 1
		}
	}
	return events
}

// deterministicFault returns a copy of the fault with its activation criteria
// stripped. Stub-driven faults fire probabilistically; the recorded timeline
// must replay them unconditionally (the event table decides when), so the
// activation is dropped.
func deterministicFault(fault *spec.FaultDefinition) *spec.FaultDefinition {
	if fault == nil {
		return nil
	}
	copy := *fault
	copy.Activation = nil
	return &copy
}
