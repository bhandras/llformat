package formatter

import (
	llast "github.com/bhandras/llformat/ast"
)

// OwnedSpanProvider is an optional interface that a stage formatter can
// implement to declare which regions of the source it conceptually "owns".
//
// Ownership is used to prevent stage fighting by allowing earlier stages to
// avoid rewriting inside spans that will be formatted later.
type OwnedSpanProvider interface {
	OwnedSpans(src []byte) llast.OffsetSpanSet
}

// OwnershipAware is an optional interface that a stage formatter can implement
// to receive pipeline-level ownership information about later stages.
type OwnershipAware interface {
	SetOwnershipRegistry(reg *OwnershipRegistry)
}

// OwnershipRegistry holds owned spans for a specific formatting pass.
//
// The registry is always scoped to the current source snapshot. Pipelines that
// want robustness across stage rewrites should recompute the registry between
// stages, rather than attempting to map offsets through edits.
type OwnershipRegistry struct {
	allOwned  llast.OffsetSpanSet
	byStage   map[string]llast.OffsetSpanSet
	stageList []string
}

func newOwnershipRegistry() *OwnershipRegistry {
	return &OwnershipRegistry{
		byStage: make(map[string]llast.OffsetSpanSet),
	}
}

// AllOwned returns the union of all owned spans.
func (r *OwnershipRegistry) AllOwned() llast.OffsetSpanSet {
	if r == nil {
		return llast.OffsetSpanSet{}
	}

	return r.allOwned
}

// AllOwnedExcept returns the union of all owned spans except those owned by the
// provided stage name.
func (r *OwnershipRegistry) AllOwnedExcept(
	stageName string) llast.OffsetSpanSet {

	if r == nil {
		return llast.OffsetSpanSet{}
	}
	if stageName == "" {
		return r.allOwned
	}

	var out llast.OffsetSpanSet
	for _, name := range r.stageList {
		if name == stageName {
			continue
		}
		out = out.Union(r.byStage[name])
	}

	return out
}

// ByStage returns the span set owned by a specific stage name, if present.
func (r *OwnershipRegistry) ByStage(stageName string) (llast.OffsetSpanSet,
	bool) {

	if r == nil {
		return llast.OffsetSpanSet{}, false
	}
	s, ok := r.byStage[stageName]

	return s, ok
}

// AllOwnedAfter returns the union of owned spans declared by stages that run
// after the provided stage name.
//
// This supports a directional ownership policy: earlier stages should avoid
// rewriting spans that later stages "own", but later stages are allowed to
// rewrite inside earlier-stage owned spans as part of the pipeline.
func (r *OwnershipRegistry) AllOwnedAfter(
	stageName string) llast.OffsetSpanSet {

	if r == nil {
		return llast.OffsetSpanSet{}
	}
	if stageName == "" {
		return r.allOwned
	}

	idx := -1
	for i, name := range r.stageList {
		if name == stageName {
			idx = i
			break
		}
	}
	if idx == -1 {

		// Unknown stage name: conservatively treat all owned spans as
		// forbidden.
		return r.allOwned
	}

	var out llast.OffsetSpanSet
	for i := idx + 1; i < len(r.stageList); i++ {
		out = out.Union(r.byStage[r.stageList[i]])
	}

	return out
}

func (r *OwnershipRegistry) add(stageName string, spans llast.OffsetSpanSet) {
	if r.byStage == nil {
		r.byStage = make(map[string]llast.OffsetSpanSet)
	}

	if existing, ok := r.byStage[stageName]; ok {
		spans = existing.Union(spans)
	} else {
		r.stageList = append(r.stageList, stageName)
	}

	r.byStage[stageName] = spans
	r.allOwned = r.allOwned.Union(spans)
}

// BuildOwnershipRegistry computes owned spans for the provided stages on the
// given source snapshot.
func BuildOwnershipRegistry(src []byte, stages []Stage) *OwnershipRegistry {
	reg := newOwnershipRegistry()
	for _, s := range stages {
		if s.Formatter == nil {
			continue
		}
		provider, ok := s.Formatter.(OwnedSpanProvider)
		if !ok {
			continue
		}
		reg.add(s.Name, provider.OwnedSpans(src))
	}

	return reg
}
