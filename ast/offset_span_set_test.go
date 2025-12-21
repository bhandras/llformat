package ast

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOffsetSpanSet_Contains(t *testing.T) {
	set := NewOffsetSpanSet(
		[]OffsetSpan{
			{Start: 10, End: 20},
			{Start: 30, End: 40},
		},
	)

	require.False(t, set.Contains(9))
	require.True(t, set.Contains(10))
	require.True(t, set.Contains(19))
	require.False(t, set.Contains(20))

	require.False(t, set.Contains(29))
	require.True(t, set.Contains(30))
	require.True(t, set.Contains(39))
	require.False(t, set.Contains(40))
}

func TestOffsetSpanSet_MergesOverlapsAndAdjacency(t *testing.T) {
	set := NewOffsetSpanSet([]OffsetSpan{
		{Start: 30, End: 40},
		{Start: 10, End: 20},
		{Start: 18, End: 22}, // overlaps [10,20)
		{Start: 22, End: 25}, // adjacent to previous merge
		{Start: 25, End: 26}, // adjacent
	})

	require.True(t, set.Contains(10))
	require.True(t, set.Contains(21))
	require.True(t, set.Contains(25))
	require.False(t, set.Contains(26))

	require.True(t, set.Contains(30))
	require.True(t, set.Contains(39))
	require.False(t, set.Contains(40))
}

func TestOffsetSpanSet_UnionMerges(t *testing.T) {
	a := NewOffsetSpanSet(
		[]OffsetSpan{
			{Start: 10, End: 20},
			{Start: 30, End: 40},
		},
	)
	b := NewOffsetSpanSet(
		[]OffsetSpan{
			{Start: 18, End: 22},
			{Start: 22, End: 25},
		},
	)

	u := a.Union(b)
	require.True(t, u.Contains(10))
	require.True(t, u.Contains(21))
	require.True(t, u.Contains(24))
	require.False(t, u.Contains(25))

	require.True(t, u.Contains(30))
	require.True(t, u.Contains(39))
	require.False(t, u.Contains(40))
}
