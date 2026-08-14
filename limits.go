package loom

// Limits are the resource controls the spec requires (§35). Loom is Turing
// complete, so evaluation may not terminate; reaching any limit raises
// resource_limit. The values are deployment policy, not language constants.
type Limits struct {
	Steps        int // evaluation steps (one per term evaluated, one per application)
	Depth        int // call depth, standing in for stack protection
	Size         int // nodes in one constructed persistable value
	MatchResults int // bindings a single match may return
}

// DefaultLimits are sized so an ordinary program never notices them and a
// runaway one stops in well under a second.
func DefaultLimits() Limits {
	return Limits{
		Steps:        1_000_000,
		Depth:        10_000,
		Size:         1_000_000,
		MatchResults: 100_000,
	}
}

// maxSize keeps two node sizes plus one from overflowing uint32 when Apply adds
// them.
const maxSize = 1 << 30

func (l Limits) withDefaults() Limits {
	d := DefaultLimits()
	if l.Steps <= 0 {
		l.Steps = d.Steps
	}
	if l.Depth <= 0 {
		l.Depth = d.Depth
	}
	if l.Size <= 0 {
		l.Size = d.Size
	}
	if l.Size > maxSize {
		l.Size = maxSize
	}
	if l.MatchResults <= 0 {
		l.MatchResults = d.MatchResults
	}
	return l
}
