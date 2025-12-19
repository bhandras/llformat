package formatter

import "sync"

// formatGlobalsMu guards package-level formatting globals (columnLimit/tabStop
// and other related state) so formatters can be used safely from parallel tests
// and concurrent callers.
var formatGlobalsMu sync.Mutex
