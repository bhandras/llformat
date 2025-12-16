package formatter

import (
	"bytes"
	"go/ast"
	formatstd "go/format"
	"go/parser"
	"go/token"
	"sort"
	"strings"

	"github.com/lightninglabs/llformat/text"
)

type legacyScanCallCandidate struct {
	start    int
	end      int
	funcName string
}

// formatMultiLineCallsInSourceAST is a legacy-compatible implementation that
// uses Go's parser/AST to select the next call to reformat, while reusing the
// exact same formatting logic as the scan-based legacy implementation.
//
// If the source is not parseable, it falls back to the scan-based selector.
func (f *MultiLineCallFormatter) formatMultiLineCallsInSourceAST(src []byte) []byte {
	result := src
	maxIterations := 20

	for iter := 0; iter < maxIterations; iter++ {
		modified, changed := f.formatOneCallInSourceAST(result)
		if !changed {
			break
		}
		result = modified
	}

	if f.cfg.SkipGofmt {
		return result
	}
	if formatted, err := formatstd.Source(result); err == nil {
		return formatted
	}
	return result
}

func (f *MultiLineCallFormatter) formatOneCallInSourceAST(src []byte) ([]byte, bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "in.go", src, parser.AllErrors)
	if err != nil || file == nil {
		return f.formatOneCallInSourceScan(src)
	}

	candidates := legacyScanCallCandidatesFromAST(file, fset, src)
	if len(candidates) == 0 {
		return src, false
	}

	for _, c := range candidates {
		if c.start < 0 || c.end > len(src) || c.start >= c.end {
			continue
		}

		if f.shouldExclude(c.funcName) {
			continue
		}

		lineStart := text.LastLineStart(src, c.start)
		indentBytes := src[lineStart:c.start]

		currentLineLen := visualLen(string(indentBytes)) + visualLen(string(src[c.start:c.end]))
		if currentLineLen <= f.cfg.ColumnLimit {
			continue
		}

		// Format as multi-line, matching the legacy formatter's indentation
		// model (indentation is based on leading whitespace only).
		wsIndent := string(text.LeadingWhitespace(src, lineStart))
		formatted := f.formatAsMultiLine(src[c.start:c.end], wsIndent)

		var out bytes.Buffer
		out.Grow(len(src) + len(formatted))
		out.Write(src[:c.start])
		out.WriteString(formatted)
		out.Write(src[c.end:])
		return out.Bytes(), true
	}

	return src, false
}

func legacyScanCallCandidatesFromAST(file *ast.File, fset *token.FileSet, src []byte) []legacyScanCallCandidate {
	var candidates []legacyScanCallCandidate

	ast.Inspect(file, func(n ast.Node) bool {
		ce, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		startPos := legacyScanCallStartPos(ce.Fun)
		if startPos == token.NoPos {
			return true
		}

		start := fset.Position(startPos).Offset
		lparen := fset.Position(ce.Lparen).Offset
		end := fset.Position(ce.Rparen).Offset + 1

		if start < 0 || lparen < 0 || end < 0 {
			return true
		}
		if start >= len(src) || lparen > len(src) || end > len(src) {
			return true
		}
		if start >= lparen || lparen >= end {
			return true
		}

		funcName := strings.TrimSpace(string(src[start:lparen]))
		candidates = append(candidates, legacyScanCallCandidate{
			start:    start,
			end:      end,
			funcName: funcName,
		})
		return true
	})

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].start != candidates[j].start {
			return candidates[i].start < candidates[j].start
		}
		return candidates[i].end < candidates[j].end
	})

	return candidates
}
