package dsl

import (
	"bytes"
	"fmt"
	"sort"
)

// Edit represents a single source edit in byte offsets: replace src[Start:End]
// with Replace. Insertions have Start == End.
type Edit struct {
	Start   int
	End     int
	Replace []byte
}

// ApplyEdits applies a set of non-overlapping edits to src and returns the
// modified source.
//
// Edits may be provided unsorted; they are applied in increasing Start order.
func ApplyEdits(src []byte, edits []Edit) ([]byte, error) {
	if len(edits) == 0 {
		return src, nil
	}

	sorted := make([]Edit, len(edits))
	copy(sorted, edits)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Start != sorted[j].Start {
			return sorted[i].Start < sorted[j].Start
		}
		return sorted[i].End < sorted[j].End
	})

	if err := validateEdits(src, sorted); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.Grow(estimatedSize(len(src), sorted))

	cursor := 0
	for _, e := range sorted {
		out.Write(src[cursor:e.Start])
		out.Write(e.Replace)
		cursor = e.End
	}
	out.Write(src[cursor:])

	return out.Bytes(), nil
}

func estimatedSize(srcLen int, edits []Edit) int {
	size := srcLen
	for _, e := range edits {
		size += len(e.Replace) - (e.End - e.Start)
	}
	if size < 0 {
		return 0
	}
	return size
}

func validateEdits(src []byte, edits []Edit) error {
	srcLen := len(src)
	for i, e := range edits {
		if e.Start < 0 || e.End < 0 || e.Start > srcLen || e.End > srcLen {
			return fmt.Errorf("edit %d out of bounds: [%d:%d] (len=%d)", i, e.Start, e.End, srcLen)
		}
		if e.Start > e.End {
			return fmt.Errorf("edit %d invalid range: start=%d end=%d", i, e.Start, e.End)
		}
		if i == 0 {
			continue
		}
		prev := edits[i-1]
		if prev.End > e.Start {
			return fmt.Errorf("edits overlap: prev=[%d:%d] next=[%d:%d]", prev.Start, prev.End, e.Start, e.End)
		}
	}
	return nil
}

