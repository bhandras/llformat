package formatter

import "sort"

type offsetSpan struct {
	start int
	end   int
}

func (s offsetSpan) contains(off int) bool {
	return off >= s.start && off < s.end
}

// offsetSpanSet is a sorted, non-overlapping set of spans.
type offsetSpanSet struct {
	spans []offsetSpan
}

func newOffsetSpanSet(spans []offsetSpan) offsetSpanSet {
	if len(spans) == 0 {
		return offsetSpanSet{}
	}

	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].end < spans[j].end
	})

	merged := spans[:0]
	for _, s := range spans {
		if len(merged) == 0 {
			merged = append(merged, s)
			continue
		}
		last := &merged[len(merged)-1]
		if s.start <= last.end {
			if s.end > last.end {
				last.end = s.end
			}
			continue
		}
		merged = append(merged, s)
	}

	return offsetSpanSet{spans: merged}
}

func (s offsetSpanSet) containsOffset(off int) bool {
	if len(s.spans) == 0 {
		return false
	}

	// Find the first span whose end is > off; if that span also starts <= off,
	// the offset is contained.
	idx := sort.Search(len(s.spans), func(i int) bool {
		return s.spans[i].end > off
	})
	if idx >= len(s.spans) {
		return false
	}
	return s.spans[idx].contains(off)
}

