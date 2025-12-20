package formatter

import (
	llast "github.com/lightninglabs/llformat/ast"
)

// OwnershipPolicy describes how a stage should treat pipeline ownership spans.
//
// The default policy is "forbid edits that overlap any spans owned by other
// stages in the current pass". This prevents stage fighting when multiple
// stages could rewrite the same region.
type OwnershipPolicy struct {
	Registry  *OwnershipRegistry
	StageName string
}

func NewOwnershipPolicy(reg *OwnershipRegistry, stageName string) OwnershipPolicy {
	return OwnershipPolicy{
		Registry:  reg,
		StageName: stageName,
	}
}

func (p OwnershipPolicy) ForbiddenSpans() llast.OffsetSpanSet {
	if p.Registry == nil {
		return llast.OffsetSpanSet{}
	}
	if p.StageName == "" {
		return p.Registry.AllOwned()
	}

	// Directional ownership: this stage should avoid rewriting within spans that
	// later stages will format, but it is allowed to rewrite inside spans owned
	// by earlier stages.
	return p.Registry.AllOwnedAfter(p.StageName)
}
