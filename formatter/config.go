// Package formatter provides Go source code formatting utilities.
package formatter

import "github.com/bhandras/llformat/width"

// DefaultColumnLimit is the default maximum line length.
const DefaultColumnLimit = 80

// DefaultTabStop is the default tab width for visual calculations.
const DefaultTabStop = 8

// BaseConfig holds common formatting configuration.
type BaseConfig struct {
	ColumnLimit int // Maximum line length (default: 80)
	TabStop     int // Tab width for visual calculations (default: 8)
}

// NewBaseConfig creates a BaseConfig with defaults applied. Zero values are
// replaced with defaults.
func NewBaseConfig(col, tab int) BaseConfig {
	if col <= 0 {
		col = DefaultColumnLimit
	}
	if tab <= 0 {
		tab = DefaultTabStop
	}

	return BaseConfig{
		ColumnLimit: col,
		TabStop:     tab,
	}
}

// Width returns the visual width of a string, accounting for tabs.
func (c BaseConfig) Width(s string) int {
	return width.VisualLenWithTab(s, c.TabStop)
}

// WidthFrom returns the visual width after appending content starting from
// currentCol.
func (c BaseConfig) WidthFrom(currentCol int, content string) int {
	return width.AdvanceColsWithTab(currentCol, content, c.TabStop)
}

// FitsInLimit checks if content at currentCol fits within the column limit.
func (c BaseConfig) FitsInLimit(currentCol int, content string) bool {
	return c.WidthFrom(currentCol, content) <= c.ColumnLimit
}

// Remaining returns how many columns remain before hitting the limit.
func (c BaseConfig) Remaining(currentCol int) int {
	if currentCol >= c.ColumnLimit {
		return 0
	}

	return c.ColumnLimit - currentCol
}

// FirstLineWidth returns the visual width of the first line in content.
func (c BaseConfig) FirstLineWidth(content string) int {
	return width.FirstLineLenWithTab(content, c.TabStop)
}

// LastLineWidth returns the visual width of the last line in content.
func (c BaseConfig) LastLineWidth(content string) int {
	return width.LastLineLenWithTab(content, c.TabStop)
}
