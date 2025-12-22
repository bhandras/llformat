package dsl

// EditBuilder accumulates source edits and applies them in one validated pass.
// It is a small convenience layer over ApplyEdits.
type EditBuilder struct {
	edits []Edit
}

// Replace replaces src[start:end] with replace.
func (b *EditBuilder) Replace(start, end int, replace []byte) {
	b.edits = append(
		b.edits, Edit{
			Start:   start,
			End:     end,
			Replace: replace,
		},
	)
}

// Insert inserts content at pos.
func (b *EditBuilder) Insert(pos int, content []byte) {
	b.edits = append(b.edits, Edit{Start: pos, End: pos, Replace: content})
}

// Delete removes src[start:end].
func (b *EditBuilder) Delete(start, end int) {
	b.edits = append(b.edits, Edit{Start: start, End: end, Replace: nil})
}

// Apply applies the accumulated edits to src, returning the new source and a
// boolean indicating whether any effective change was made.
func (b *EditBuilder) Apply(src []byte) ([]byte, bool, error) {
	if len(b.edits) == 0 {
		return src, false, nil
	}

	edits := b.filterNoOps(src, b.edits)
	if len(edits) == 0 {
		return src, false, nil
	}

	out, err := ApplyEdits(src, edits)
	if err != nil {
		return nil, false, err
	}

	return out, true, nil
}

func (b *EditBuilder) filterNoOps(src []byte, edits []Edit) []Edit {
	filtered := make([]Edit, 0, len(edits))
	for _, e := range edits {
		if e.Start == e.End && len(e.Replace) == 0 {
			continue
		}
		if hasReplacement(src, e.Start, e.End, e.Replace) {
			continue
		}
		filtered = append(filtered, e)
	}

	return filtered
}
