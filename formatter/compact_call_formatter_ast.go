package formatter

import (
	"bytes"
	"go/ast"
	formatstd "go/format"
	"go/parser"
	"go/token"
	"strings"

	"github.com/bhandras/llformat/text"
)

func isSelectorChainCallStartOnLine(src []byte, start int) bool {
	i := start - 1
	for i >= 0 && (src[i] == ' ' || src[i] == '\t') {
		i--
	}

	return i >= 0 && src[i] == '.'
}

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
		return maybeGofmt(src)
	}

	res := formatCompactCallCandidates(src, candidates)

	return maybeGofmt(res)
}

func maybeGofmt(src []byte) []byte {
	if skipGofmt {
		return src
	}
	if formatted, err := formatstd.Source(src); err == nil {
		return formatted
	}

	return src
}

func formatCompactCallCandidates(src []byte,
	candidates []compactCallCandidate) []byte {

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

		if !shouldFormatCandidate(c) {
			continue
		}

		out.Write(src[pos:c.start])

		pos = formatCompactCallCandidate(&out, src, c)
	}

	out.Write(src[pos:])

	return out.Bytes()
}

func shouldFormatCandidate(c compactCallCandidate) bool {
	if c.targetMatch != "" {
		return true
	}

	return fallbackNonTargets
}

func formatCompactCallCandidate(out *bytes.Buffer, src []byte,
	c compactCallCandidate) int {

	lineStart := text.LastLineStart(src, c.start)
	indentBytes := src[lineStart:c.start]
	wsIndent := text.LeadingWhitespace(src, lineStart)

	if c.targetMatch != "" {
		formatted := formatCallGreedy(
			src[c.start:c.end], string(wsIndent),
			visualLen(
				string(indentBytes),
			),
		)
		out.WriteString(formatted)

		return c.end
	}

	return formatFallbackCompactCall(
		out, src, c, lineStart, indentBytes, wsIndent,
	)
}

func formatFallbackCompactCall(out *bytes.Buffer, src []byte,
	c compactCallCandidate, lineStart int, indentBytes []byte,
	wsIndent []byte) int {

	span := src[c.start:c.end]
	// Avoid rewriting calls that contain inline comments; rewriting these
	// can cause non-idempotent comment attachment across pipeline runs.
	if spanHasCommentOutsideStrings(span) {
		out.Write(span)

		return c.end
	}

	indentPrefix := string(indentBytes)
	if !fallbackCallPrefixAllowed(indentPrefix, src, c.start) {
		// Consume the call but leave it unchanged; this matches the
		// scan-based fallback behavior (which skips nested target calls
		// inside other calls).
		out.Write(span)

		return c.end
	}
	if !fallbackCallCalleeAllowed(src, c) {
		out.Write(span)

		return c.end
	}

	tp := strings.TrimSpace(indentPrefix)
	if tp == ")" || tp == ")." {
		indentPrefix = string(
			text.LeadingWhitespace(src, lineStart),
		)
	}

	callText := string(src[c.start:c.end])
	flat := strings.Join(
		strings.Fields(
			stripComments(callText),
		), " ",
	)
	singleLineLen := visualLen(flat)
	currentLineLen := visualLen(indentPrefix) + singleLineLen
	needsWrap := currentLineLen > columnLimit
	if needsWrap && !isChainedShortCall(src, c.start, c.end) {
		formatted := formatCallPackedMultiLine(
			span, string(wsIndent), string(wsIndent), true,
		)
		out.WriteString(formatted)

		return c.end
	}

	// Consume the call but leave it unchanged; this matches the scan-based
	// fallback behavior (which skips nested target calls inside other
	// calls).
	out.Write(span)

	return c.end
}

func fallbackCallPrefixAllowed(indentPrefix string, src []byte,
	callStart int) bool {

	trimmedPrefix := strings.TrimSpace(indentPrefix)
	allowedByPrefix := trimmedPrefix == "" ||
		strings.HasSuffix(trimmedPrefix, ":=") ||
		strings.HasSuffix(trimmedPrefix, "=") ||
		strings.HasSuffix(trimmedPrefix, "return") ||
		strings.HasSuffix(trimmedPrefix, "go") ||
		strings.HasSuffix(trimmedPrefix, "defer")
	if !allowedByPrefix &&
		isSelectorChainCallStartOnLine(src, callStart) {

		return true
	}

	return allowedByPrefix
}

func fallbackCallCalleeAllowed(src []byte, c compactCallCandidate) bool {
	callee := strings.TrimSpace(
		string(src[c.start:c.lparen]),
	)
	if callNameContainsAny(
		callee, fallbackNonTargetsExcludes,
	) {

		return false
	}

	if fallbackNonTargetsExcludeSelectors {
		// Exclude selector calls, including method-chain calls where
		// the call start is the selector ident (e.g. ".Execute(") so
		// the callee span itself contains no '.'.
		if isSelectorChainCallStartOnLine(src, c.start) {
			return false
		}
		if strings.Contains(callee, ".") {
			return false
		}
	}

	return true
}

func compactCallCandidatesFromAST(file *ast.File, fset *token.FileSet,
	src []byte, targets []string) []compactCallCandidate {

	spans := legacyCallSpansFromAST(file, fset, src)
	candidates := make([]compactCallCandidate, 0, len(spans))
	for _, s := range spans {
		targetMatch := ""
		for _, t := range targets {
			// The legacy scanner matches a target by exact byte
			// prefix at the call start, with the '(' immediately
			// after the function name.
			if s.Start+len(t) <= len(src) &&
				s.Start+len(t)-
					1 == s.Lparen && string(
				src[s.Start:s.Start+len(t)],
			) == t {

				targetMatch = t
				break
			}
		}

		candidates = append(
			candidates, compactCallCandidate{
				start:       s.Start,
				end:         s.End,
				lparen:      s.Lparen,
				targetMatch: targetMatch,
			},
		)
	}

	return candidates
}
