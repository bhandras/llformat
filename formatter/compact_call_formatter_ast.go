package formatter

import (
	"bytes"
	formatstd "go/format"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"

	"github.com/lightninglabs/llformat/text"
)

type compactCallCandidate struct {
	start int
	end   int

	lparen int

	// targetMatch is the exact matched target prefix (including '(') if the
	// candidate is a targeted call.
	targetMatch string
}

func formatWithTargetsAST(src []byte, targets []string) []byte {
	currentTargets = targets

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "in.go", src, parser.AllErrors)
	if err != nil || file == nil {
		// Preserve legacy behavior on unparseable input.
		return formatWithTargetsScan(src, targets)
	}

	candidates := compactCallCandidatesFromAST(file, fset, src, targets)
	if len(candidates) == 0 {
		if skipGofmt {
			return src
		}
		if formatted, err := formatstd.Source(src); err == nil {
			return formatted
		}
		return src
	}

	var out bytes.Buffer
	out.Grow(len(src))
	pos := 0

	for _, c := range candidates {
		if c.start < pos {
			continue
		}
		if c.start < 0 || c.end > len(src) || c.start >= c.end {
			continue
		}

		considerAsCall := c.targetMatch != ""
		if !considerAsCall && fallbackNonTargets {
			considerAsCall = true
		}
		if !considerAsCall {
			continue
		}

		out.Write(src[pos:c.start])

		lineStart := text.LastLineStart(src, c.start)
		indentBytes := src[lineStart:c.start]
		wsIndent := text.LeadingWhitespace(src, lineStart)

		if c.targetMatch != "" {
			formatted := formatCallGreedy(src[c.start:c.end], string(wsIndent), visualLen(string(indentBytes)))
			out.WriteString(formatted)
			pos = c.end
			continue
		}

		// Legacy fallback formatting for non-target calls.
		indentPrefix := string(indentBytes)
		tp := strings.TrimSpace(indentPrefix)
		if tp == ")" || tp == ")." {
			indentPrefix = string(text.LeadingWhitespace(src, lineStart))
		}

		callText := string(src[c.start:c.end])
		flat := strings.Join(strings.Fields(stripComments(callText)), " ")
		singleLineLen := visualLen(flat)
		currentLineLen := visualLen(indentPrefix) + singleLineLen
		needsWrap := currentLineLen > columnLimit
		if needsWrap && !isChainedShortCall(src, c.start, c.end) {
			formatted := formatCallPackedMultiLine(src[c.start:c.end], string(wsIndent), string(wsIndent), true)
			out.WriteString(formatted)
			pos = c.end
			continue
		}

		// Consume the call but leave it unchanged; this matches the scan-based
		// fallback behavior (which skips nested target calls inside other calls).
		out.Write(src[c.start:c.end])
		pos = c.end
	}

	out.Write(src[pos:])
	res := out.Bytes()

	if skipGofmt {
		return res
	}
	if formatted, err := formatstd.Source(res); err == nil {
		return formatted
	}
	return res
}

func compactCallCandidatesFromAST(file *ast.File, fset *token.FileSet, src []byte, targets []string) []compactCallCandidate {
	var candidates []compactCallCandidate

	ast.Inspect(file, func(n ast.Node) bool {
		ce, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ce == nil || ce.Lparen == token.NoPos || ce.Rparen == token.NoPos {
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
		if start >= len(src) || lparen >= len(src) || end > len(src) {
			return true
		}
		if start >= lparen || lparen >= end {
			return true
		}

		targetMatch := ""
		for _, t := range targets {
			// The legacy scanner matches a target by exact byte prefix at the
			// call start, with the '(' immediately after the function name.
			if start+len(t) <= len(src) && start+len(t)-1 == lparen && string(src[start:start+len(t)]) == t {
				targetMatch = t
				break
			}
		}

		candidates = append(candidates, compactCallCandidate{
			start:       start,
			end:         end,
			lparen:      lparen,
			targetMatch: targetMatch,
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
