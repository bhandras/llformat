package formatter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOffsetSpanSet_ContainsOffset(t *testing.T) {
	set := newOffsetSpanSet([]offsetSpan{
		{start: 10, end: 20},
		{start: 30, end: 40},
	})

	require.False(t, set.containsOffset(9))
	require.True(t, set.containsOffset(10))
	require.True(t, set.containsOffset(19))
	require.False(t, set.containsOffset(20))

	require.False(t, set.containsOffset(29))
	require.True(t, set.containsOffset(30))
	require.True(t, set.containsOffset(39))
	require.False(t, set.containsOffset(40))
}

func TestOffsetSpanSet_MergesOverlapsAndAdjacency(t *testing.T) {
	set := newOffsetSpanSet([]offsetSpan{
		{start: 30, end: 40},
		{start: 10, end: 20},
		{start: 18, end: 22}, // overlaps [10,20)
		{start: 22, end: 25}, // adjacent to previous merge
		{start: 25, end: 26}, // adjacent
	})

	require.True(t, set.containsOffset(10))
	require.True(t, set.containsOffset(21))
	require.True(t, set.containsOffset(25))
	require.False(t, set.containsOffset(26))

	require.True(t, set.containsOffset(30))
	require.True(t, set.containsOffset(39))
	require.False(t, set.containsOffset(40))
}

