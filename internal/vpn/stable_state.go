package vpn

import "time"

// StableState debounces observed boolean state changes until the same value has
// been observed for a configured duration.
type StableState struct {
	stableFor time.Duration

	observed      bool
	observedSet   bool
	observedSince time.Time

	stable    bool
	stableSet bool
}

// NewStableState returns a state debouncer. A zero stableFor duration makes
// observations stable immediately.
func NewStableState(stableFor time.Duration) *StableState {
	return &StableState{stableFor: stableFor}
}

// Observe records an observed state at now. The returned ok value is true once
// the observed value has been stable long enough. When ok is true, stable is the
// debounced state. The changed value reports whether the debounced state changed.
func (s *StableState) Observe(value bool, now time.Time) (stable bool, changed bool, ok bool) {
	if !s.observedSet || s.observed != value {
		s.observed = value
		s.observedSet = true
		s.observedSince = now
	}

	if s.stableFor > 0 && now.Sub(s.observedSince) < s.stableFor {
		return false, false, false
	}

	changed = !s.stableSet || s.stable != s.observed
	s.stable = s.observed
	s.stableSet = true
	return s.stable, changed, true
}
