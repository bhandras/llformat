// Package ast provides AST parsing and inspection utilities for Go source code.
package ast

import "sort"

// OffsetSpan is a half-open interval [Start, End) in byte offsets.
type OffsetSpan struct {
	Start int
	End   int
}

func (s OffsetSpan) contains(off int) bool {
	return off >= s.Start && off < s.End
}

// OffsetSpanSet is a sorted, non-overlapping set of spans.
type OffsetSpanSet struct {
	spans []OffsetSpan
}

func NewOffsetSpanSet(spans []OffsetSpan) OffsetSpanSet {
	if len(spans) == 0 {
		return OffsetSpanSet{}
	}

	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Start != spans[j].Start {
			return spans[i].Start < spans[j].Start
		}
		return spans[i].End < spans[j].End
	})

	merged := spans[:0]
	for _, s := range spans {
		if len(merged) == 0 {
			merged = append(merged, s)
			continue
		}
		last := &merged[len(merged)-1]
		if s.Start <= last.End {
			if s.End > last.End {
				last.End = s.End
			}
			continue
		}
		merged = append(merged, s)
	}

	return OffsetSpanSet{spans: merged}
}

// Overlaps reports whether any span in the set intersects the half-open interval
// [start, end). Intervals where end <= start are treated as empty and never
// overlap.
func (s OffsetSpanSet) Overlaps(start, end int) bool {
	if len(s.spans) == 0 {
		return false
	}
	if end <= start {
		return false
	}

	// Find the first span whose end is > start; if that span starts < end, the
	// intervals overlap.
	idx := sort.Search(len(s.spans), func(i int) bool {
		return s.spans[i].End > start
	})
	if idx >= len(s.spans) {
		return false
	}
	return s.spans[idx].Start < end
}

// Contains returns true if off falls inside any span in the set.
func (s OffsetSpanSet) Contains(off int) bool {
	if len(s.spans) == 0 {
		return false
	}

	// Find the first span whose end is > off; if that span also starts <= off,
	// the offset is contained.
	idx := sort.Search(len(s.spans), func(i int) bool {
		return s.spans[i].End > off
	})
	if idx >= len(s.spans) {
		return false
	}
	return s.spans[idx].contains(off)
}

// Union returns a new set that contains all spans from s and other.
func (s OffsetSpanSet) Union(other OffsetSpanSet) OffsetSpanSet {
	if len(s.spans) == 0 {
		return other
	}
	if len(other.spans) == 0 {
		return s
	}

	spans := make([]OffsetSpan, 0, len(s.spans)+len(other.spans))
	spans = append(spans, s.spans...)
	spans = append(spans, other.spans...)
	return NewOffsetSpanSet(spans)
}
