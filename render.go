package loom

import (
	"sort"
	"strconv"
	"strings"
)

// Render turns a result into the lines a host prints for it. It is the one
// rendering both the REPL and the conformance corpus use, so the corpus pins
// exactly what a user sees.
//
//	Evaluate     the value, in surface syntax
//	Assert       claim:N
//	Transaction  claim:N per claim, in order
//	Query        one line per match, "x = Bob", captures sorted by name
//	Define       nothing
//	Retract      nothing
//
// A query with no matches renders "(no matches)"; a match that binds nothing
// renders "(match)". Match results are a set, so the lines are sorted to give a
// deterministic order without implying one (§25).
func Render(store *Store, r Result) []string {
	switch r.Command.(type) {
	case Evaluate:
		return []string{store.Display(r.Value)}
	case Assert, Transaction:
		out := make([]string, 0, len(r.Claims))
		for _, id := range r.Claims {
			out = append(out, "claim:"+strconv.FormatUint(uint64(id), 10))
		}
		return out
	case Query:
		if len(r.Rows) == 0 {
			return []string{"(no matches)"}
		}
		out := make([]string, 0, len(r.Rows))
		for _, row := range r.Rows {
			out = append(out, RenderBindings(store, row))
		}
		sort.Strings(out)
		return out
	}
	return nil
}

// RenderBindings writes one match as "x = Bob", or "a = A, b = B" for several
// captures, always in name order.
func RenderBindings(store *Store, row Bindings) string {
	if len(row) == 0 {
		return "(match)"
	}
	names := make([]string, 0, len(row))
	for name := range row {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+" = "+store.Source(row[name]))
	}
	return strings.Join(parts, ", ")
}

// Held returns the canonical form of every value held at this snapshot, sorted
// by byte order. Sorting is what makes the listing comparable across
// implementations, whose internal value ordering has no reason to agree.
func (w World) Held() []string {
	values := w.Values()
	out := make([]string, 0, len(values))
	for _, id := range values {
		out = append(out, w.store.Canonical(id))
	}
	sort.Strings(out)
	return out
}
