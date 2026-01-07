package compat

import (
	"sync"

	"github.com/bhandras/llformat/width"
)

// These globals are retained to preserve behavior of the original legacy
// comment formatter implementation, which shares width helpers with the old
// formatter pipeline.
var (
	formatGlobalsMu sync.Mutex
	columnLimit     = 80
	tabStop         = 8
)

func visualLen(s string) int {
	ts := tabStop
	if ts <= 0 {
		ts = 8
	}

	return width.VisualLenWithTab(s, ts)
}
