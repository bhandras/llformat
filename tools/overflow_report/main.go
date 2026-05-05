package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bhandras/llformat/width"
	"github.com/pmezard/go-difflib/difflib"
)

type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, ",") }
func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)

	return nil
}

type overflow struct {
	Line  int
	Width int
	Text  string
}

type snippet struct {
	Kind       string
	StartLine  int
	EndLine    int
	Text       string
	ReproText  string
	Reproduces bool
	ReproWhy   string
}

func main() {
	var (
		rootDir = flag.String(
			"dir", "", "Root directory to scan (required)",
		)
		outFile = flag.String(
			"out", "BUG_REPORTS.md", "Output markdown file (or "+
				"directory prefix for per-bug reports)",
		)
		llfmt = flag.String(
			"llformat", "./bin/llformat", "Path to llformat binary",
		)
		col     = flag.Int("col", 80, "Column limit")
		tabStop = flag.Int("tab", width.DefaultTabStop, "Tab stop")
		maxPer  = flag.Int(
			"max-per-file", 20,
			"Maximum number of overflow lines to report per file",
		)
		context = flag.Int(
			"context", 2, "Context lines to show around each "+
				"overflow (0 disables)",
		)
		onlyChg = flag.Bool(
			"only-changed", true,
			"Only report overflows on lines modified by llformat",
		)
		chgCtx = flag.Int(
			"changed-context", 0, "When --only-changed is set, "+
				"also include overflows within N lines of "+
				"a changed line",
		)
		minExc = flag.Int(
			"min-excess", 1, "Only report lines that exceed "+
				"--col by at least this many columns",
		)
	)
	var excludes stringSliceFlag
	var excludeSuffixes stringSliceFlag
	flag.Var(
		&excludes, "exclude",
		"Exclude directory prefix (repeatable, relative to --dir)",
	)
	flag.Var(
		&excludeSuffixes, "exclude-ext",
		"Exclude file suffix (repeatable, e.g. .pb.go)",
	)
	flag.Var(
		&excludeSuffixes, "exclude-suffix",
		"Exclude file suffix (repeatable, e.g. .pb.go)",
	)
	flag.Parse()

	if *rootDir == "" {
		_, _ = fmt.Fprintln(os.Stderr, "missing required flag: --dir")
		flag.Usage()
		os.Exit(2)
	}
	if *col <= 0 {
		_, _ = fmt.Fprintln(os.Stderr, "--col must be > 0")
		os.Exit(2)
	}
	if *tabStop <= 0 {
		_, _ = fmt.Fprintln(os.Stderr, "--tab must be > 0")
		os.Exit(2)
	}
	if *maxPer <= 0 {
		_, _ = fmt.Fprintln(os.Stderr, "--max-per-file must be > 0")
		os.Exit(2)
	}
	if *context < 0 {
		_, _ = fmt.Fprintln(os.Stderr, "--context must be >= 0")
		os.Exit(2)
	}
	if *minExc <= 0 {
		_, _ = fmt.Fprintln(os.Stderr, "--min-excess must be > 0")
		os.Exit(2)
	}
	if *chgCtx < 0 {
		_, _ = fmt.Fprintln(os.Stderr, "--changed-context must be >= 0")
		os.Exit(2)
	}

	if _, err := os.Stat(*llfmt); err != nil {
		_, _ = fmt.Fprintf(
			os.Stderr, "llformat not found: %s: %v\n", *llfmt, err,
		)
		os.Exit(2)
	}

	absRoot, err := filepath.Abs(*rootDir)
	if err != nil {
		_, _ = fmt.Fprintf(
			os.Stderr, "failed to resolve --dir: %v\n", err,
		)
		os.Exit(2)
	}

	files, err := goFilesUnder(absRoot, excludes, excludeSuffixes)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "scan failed: %v\n", err)
		os.Exit(1)
	}

	now := time.Now()
	perBug, outDir, err := resolveOutput(*outFile, now)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "invalid --out: %v\n", err)
		os.Exit(2)
	}

	var report bytes.Buffer
	report.WriteString("# Formatting Overflow Report\n\n")
	fmt.Fprintf(&report, "Generated: `%s`\n", now.Format(time.RFC3339))
	fmt.Fprintf(&report, "Root: `%s`\n", absRoot)
	fmt.Fprintf(&report, "Column limit: `%d`\n", *col)
	fmt.Fprintf(&report, "Tab stop: `%d`\n\n", *tabStop)

	type bugIndexItem struct {
		RelPath string
		Summary string
	}
	var indexItems []bugIndexItem

	for _, path := range files {
		original, err := os.ReadFile(path)
		if err != nil {
			if !perBug {
				fmt.Fprintf(&report, "## %s\n\n", path)
				fmt.Fprintf(
					&report, "- read error: `%v`\n\n", err,
				)
			}
			continue
		}

		formatted, err := runLLFormat(*llfmt, *col, *tabStop, path)
		if err != nil {
			if !perBug {
				fmt.Fprintf(&report, "## %s\n\n", path)
				fmt.Fprintf(
					&report, "- llformat error: `%v`\n\n",
					err,
				)
			}
			continue
		}

		if bytes.Equal(original, formatted) {
			continue
		}

		overflows := findOverflowsMinExcess(
			formatted, *col, *tabStop, *minExc,
		)
		if *onlyChg && len(overflows) > 0 {
			changed := changedLines(original, formatted, *chgCtx)
			filtered := overflows[:0]
			for _, ov := range overflows {
				if changed[ov.Line] {
					filtered = append(filtered, ov)
				}
			}
			overflows = filtered
		}
		if len(overflows) == 0 {
			continue
		}
		sort.Slice(
			overflows,
			func(i, j int) bool { return overflows[i].Width > overflows[j].Width },
		)
		if len(overflows) > *maxPer {
			overflows = overflows[:*maxPer]
		}
		sort.Slice(
			overflows,
			func(i, j int) bool { return overflows[i].Line < overflows[j].Line },
		)

		snips, parseErr := extractSnippets(
			path, formatted, overflows, *llfmt, *col, *tabStop,
		)
		if !perBug {
			fmt.Fprintf(&report, "## %s\n\n", path)
			if parseErr != nil {
				fmt.Fprintf(
					&report, "- parse error (report "+
						"still includes raw "+
						"context): `%v`\n\n", parseErr,
				)
			}
		}

		lines := splitLines(string(formatted))
		for idx, ov := range overflows {
			rawLine := ov.Text
			displayLine := expandTabsForDisplay(rawLine, *tabStop)
			rawTabs := strings.Count(rawLine, "\t")
			rawBytes := len([]byte(rawLine))
			var parseErrStr string
			if parseErr != nil {
				parseErrStr = parseErr.Error()
			}

			if perBug {
				relName, md, err := renderBugReport(
					renderBugParams{
						GeneratedAt:     now,
						RootDir:         absRoot,
						FilePath:        path,
						ColLimit:        *col,
						TabStop:         *tabStop,
						MinExcess:       *minExc,
						OnlyChanged:     *onlyChg,
						ChangedContext:  *chgCtx,
						Overflow:        ov,
						OverflowBytes:   rawBytes,
						OverflowTabs:    rawTabs,
						OverflowDisplay: displayLine,
						FormattedLines:  lines,
						Context:         *context,
						Snippet: safeIndex(
							snips, idx,
						),
						ParseErr: parseErrStr,
					},
				)
				if err == nil {
					bugPath := filepath.Join(
						outDir, relName,
					)
					if err := os.WriteFile(
						bugPath, []byte(md), 0o644,
					); err == nil {

						indexItems = append(
							indexItems,
							bugIndexItem{
								RelPath: relName,
								Summary: fmt.Sprintf(
									"%s:%"+
										"d "+
										"(wid"+
										"th %"+
										"d)",
									path,
									ov.Line,
									ov.Width),
							},
						)
					}
				}
				continue
			}

			fmt.Fprintf(
				&report, "- Line %d (visual width %d, bytes "+
					"%d, tabs %d):\n\n", ov.Line, ov.Width,
				rawBytes, rawTabs,
			)
			report.WriteString("```go\n")
			report.WriteString(displayLine)
			report.WriteString("\n```\n\n")

			if *context > 0 {
				report.WriteString(
					"<details>" +
						"\n" +
						"<summary>Context</summary>" +
						"\n\n```go\n",
				)
				start := max(1, ov.Line-*context)
				end := min(len(lines), ov.Line+*context)
				for ln := start; ln <= end; ln++ {
					report.WriteString(lines[ln-1])
					if !strings.HasSuffix(lines[ln-1], "\n") {
						report.WriteString("\n")
					}
				}
				report.WriteString("```\n\n</details>\n\n")
			}

			if rawTabs > 0 {
				report.WriteString(
					"<details>\n<summary>Raw line " +
						"(contains " +
						"tabs)</summary>\n\n```go\n",
				)
				report.WriteString(rawLine)
				report.WriteString("\n```\n\n</details>\n\n")
			}

			if idx < len(snips) && snips[idx].Text != "" {
				s := snips[idx]
				report.WriteString("<details>\n")
				fmt.Fprintf(
					&report, "<summary>Enclosing %s "+
						"(lines %d-%d)</summary>\n\n",
					s.Kind, s.StartLine, s.EndLine,
				)
				report.WriteString("```go\n")
				report.WriteString(s.Text)
				if !strings.HasSuffix(s.Text, "\n") {
					report.WriteString("\n")
				}
				report.WriteString("```\n\n</details>\n\n")

				if s.ReproText != "" {
					report.WriteString(
						"<details>" +
							"\n" +
							"<summary>Standalone" +
							" repro</summary>" +
							"\n\n```go\n",
					)
					report.WriteString(s.ReproText)
					if !strings.HasSuffix(s.ReproText, "\n") {
						report.WriteString("\n")
					}
					report.WriteString("```\n\n")
					if s.Reproduces {
						report.WriteString(
							"Repro check: ✅ overflow reproduces after llformat\n\n",
						)
					} else {
						report.WriteString(
							"Repro check: ❌ overflow did not reproduce after llformat\n\n",
						)
					}
					if s.ReproWhy != "" {
						fmt.Fprintf(
							&report, "Note: %s\n\n",
							s.ReproWhy,
						)
					}
					report.WriteString("</details>\n\n")
				}
			}
		}
	}

	if perBug {
		sort.Slice(
			indexItems,
			func(i, j int) bool { return indexItems[i].RelPath < indexItems[j].RelPath },
		)

		var index bytes.Buffer
		index.WriteString("# llformat overflow bug reports\n\n")
		fmt.Fprintf(
			&index, "Generated: `%s`\n", now.Format(time.RFC3339),
		)
		fmt.Fprintf(&index, "Root: `%s`\n", absRoot)
		fmt.Fprintf(&index, "Column limit: `%d`\n", *col)
		fmt.Fprintf(&index, "Tab stop: `%d`\n", *tabStop)
		fmt.Fprintf(&index, "Min excess: `%d`\n", *minExc)
		fmt.Fprintf(&index, "Only changed: `%t`\n", *onlyChg)
		fmt.Fprintf(&index, "Changed context: `%d`\n\n", *chgCtx)

		for _, it := range indexItems {
			fmt.Fprintf(
				&index, "- [%s](%s)\n", it.Summary, it.RelPath,
			)
		}

		if err := os.MkdirAll(outDir, 0o755); err != nil {
			_, _ = fmt.Fprintf(
				os.Stderr, "mkdir %s failed: %v\n", outDir, err,
			)
			os.Exit(1)
		}
		if err := os.WriteFile(
			filepath.Join(outDir, "index.md"), index.Bytes(), 0o644,
		); err != nil {

			_, _ = fmt.Fprintf(
				os.Stderr, "write %s failed: %v\n",
				filepath.Join(outDir, "index.md"), err,
			)
			os.Exit(1)
		}

		return
	}

	if err := os.WriteFile(*outFile, report.Bytes(), 0o644); err != nil {
		_, _ = fmt.Fprintf(
			os.Stderr, "write %s failed: %v\n", *outFile, err,
		)
		os.Exit(1)
	}
}

type renderBugParams struct {
	GeneratedAt time.Time
	RootDir     string
	FilePath    string

	ColLimit  int
	TabStop   int
	MinExcess int

	OnlyChanged    bool
	ChangedContext int

	Overflow        overflow
	OverflowBytes   int
	OverflowTabs    int
	OverflowDisplay string

	FormattedLines []string
	Context        int
	Snippet        snippet
	ParseErr       string
}

func renderBugReport(p renderBugParams) (relName string, markdown string,
	err error) {

	id := bugID(p.FilePath, p.Overflow.Line, p.Overflow.Text)
	base := fmt.Sprintf("bug_%s_%s_L%d.md", id, filepath.Base(p.FilePath),
		p.Overflow.Line)
	base = sanitizeFilename(base)

	var b bytes.Buffer
	b.WriteString("# llformat overflow bug\n\n")
	fmt.Fprintf(&b, "Generated: `%s`\n", p.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "Root: `%s`\n", p.RootDir)
	fmt.Fprintf(&b, "File: `%s`\n", p.FilePath)
	fmt.Fprintf(&b, "Line: `%d`\n", p.Overflow.Line)
	fmt.Fprintf(&b, "Visual width: `%d`\n", p.Overflow.Width)
	fmt.Fprintf(&b, "Bytes: `%d`\n", p.OverflowBytes)
	fmt.Fprintf(&b, "Tabs: `%d`\n", p.OverflowTabs)
	fmt.Fprintf(&b, "Column limit: `%d`\n", p.ColLimit)
	fmt.Fprintf(&b, "Tab stop: `%d`\n", p.TabStop)
	fmt.Fprintf(&b, "Min excess: `%d`\n", p.MinExcess)
	fmt.Fprintf(&b, "Only changed: `%t`\n", p.OnlyChanged)
	fmt.Fprintf(&b, "Changed context: `%d`\n\n", p.ChangedContext)

	if p.ParseErr != "" {
		fmt.Fprintf(&b, "Parse warning: `%s`\n\n", p.ParseErr)
	}

	b.WriteString("## Overflow line\n\n```go\n")
	b.WriteString(p.OverflowDisplay)
	b.WriteString("\n```\n\n")

	if p.OverflowTabs > 0 {
		b.WriteString("## Raw line (tabs)\n\n```go\n")
		b.WriteString(p.Overflow.Text)
		b.WriteString("\n```\n\n")
	}

	if p.Context > 0 && len(p.FormattedLines) > 0 {
		b.WriteString("## Context\n\n```go\n")
		start := max(1, p.Overflow.Line-p.Context)
		end := min(len(p.FormattedLines), p.Overflow.Line+p.Context)
		for ln := start; ln <= end; ln++ {
			b.WriteString(p.FormattedLines[ln-1])
			if !strings.HasSuffix(p.FormattedLines[ln-1], "\n") {
				b.WriteString("\n")
			}
		}
		b.WriteString("```\n\n")
	}

	if p.Snippet.Text != "" {
		fmt.Fprintf(
			&b, "## Enclosing %s (lines %d-%d)\n\n```go\n",
			p.Snippet.Kind, p.Snippet.StartLine, p.Snippet.EndLine,
		)
		b.WriteString(p.Snippet.Text)
		if !strings.HasSuffix(p.Snippet.Text, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("```\n\n")
	}

	if p.Snippet.ReproText != "" {
		b.WriteString("## Standalone repro\n\n```go\n")
		b.WriteString(p.Snippet.ReproText)
		if !strings.HasSuffix(p.Snippet.ReproText, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("```\n\n")

		if p.Snippet.Reproduces {
			b.WriteString(
				"Repro check: overflow reproduces after " +
					"llformat\n\n",
			)
		} else {
			b.WriteString(
				"Repro check: overflow did not reproduce " +
					"after llformat\n\n",
			)
		}
		if p.Snippet.ReproWhy != "" {
			fmt.Fprintf(&b, "Note: %s\n\n", p.Snippet.ReproWhy)
		}
	}

	return base, b.String(), nil
}

func resolveOutput(out string, now time.Time) (perBug bool, outDir string,
	err error) {

	out = strings.TrimSpace(out)
	if out == "" {
		return false, "", fmt.Errorf("empty output")
	}
	if strings.HasSuffix(out, ".md") ||
		strings.HasSuffix(out, ".markdown") {
		return false, "", nil
	}
	// Treat --out as a directory prefix.
	ts := now.Format("20060102_150405")
	outDir = out + "_" + ts
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return false, "", err
	}

	return true, outDir, nil
}

func bugID(path string, line int, text string) string {
	h := sha1.New()
	_, _ = h.Write([]byte(path))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strconv.Itoa(line)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(text))
	sum := h.Sum(nil)

	return hex.EncodeToString(sum)[:10]
}

func sanitizeFilename(s string) string {
	s = strings.ReplaceAll(s, string(os.PathSeparator), "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "\t", "_")
	s = strings.ReplaceAll(s, "\n", "_")
	s = strings.ReplaceAll(s, "\r", "_")

	return s
}

func safeIndex[T any](xs []T, idx int) T {
	var zero T
	if idx < 0 || idx >= len(xs) {
		return zero
	}

	return xs[idx]
}

func goFilesUnder(root string, excludes stringSliceFlag,
	excludeSuffixes ...stringSliceFlag) ([]string, error) {

	excludePrefix := make([]string, 0, len(excludes))
	for _, p := range excludes {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		excludePrefix = append(
			excludePrefix,
			filepath.Clean(
				filepath.Join(root, p),
			),
		)
	}

	var excludedSuffixes []string
	for _, group := range excludeSuffixes {
		for _, s := range group {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			excludedSuffixes = append(excludedSuffixes, s)
		}
	}

	var out []string
	err := filepath.WalkDir(
		root,
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				for _, ex := range excludePrefix {
					if path == ex || strings.HasPrefix(
						path,
						ex+string(filepath.Separator),
					) {
						return filepath.SkipDir
					}
				}

				return nil
			}
			if filepath.Ext(path) != ".go" {
				return nil
			}
			for _, suf := range excludedSuffixes {
				if strings.HasSuffix(path, suf) {
					return nil
				}
			}
			out = append(out, path)

			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	sort.Strings(out)

	return out, nil
}

func runLLFormat(llformatBin string, col, tabStop int,
	path string) ([]byte, error) {

	cmd := exec.Command(
		llformatBin, "--col", strconv.Itoa(col),
		"--tab", strconv.Itoa(tabStop), path,
	)
	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return nil, fmt.Errorf(
			"%w: %s", err, strings.TrimSpace(string(ee.Stderr)),
		)
	}

	return nil, err
}

func splitLines(s string) []string {
	// Keep trailing newline behavior stable for display.
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}

func findOverflowsMinExcess(src []byte, colLimit, tabStop,
	minExcess int) []overflow {

	lines := splitLines(string(src))
	out := make([]overflow, 0)
	threshold := colLimit + max(1, minExcess) - 1
	for i, line := range lines {
		trimmed := strings.TrimSuffix(line, "\n")
		w := width.VisualLenWithTab(trimmed, tabStop)
		if w > threshold {
			out = append(
				out, overflow{
					Line:  i + 1,
					Width: w,
					Text:  trimmed,
				},
			)
		}
	}

	return out
}

func expandTabsForDisplay(s string, tabStop int) string {
	if tabStop <= 0 || !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))

	col := 0
	for _, r := range s {
		switch r {
		case '\t':
			spaces := tabStop - (col % tabStop)
			for i := 0; i < spaces; i++ {
				b.WriteByte(' ')
			}
			col += spaces

		default:
			b.WriteRune(r)
			col += width.RuneWidth(r)
		}
	}

	return b.String()
}

func changedLines(original, formatted []byte, radius int) map[int]bool {
	origLines := splitLines(string(original))
	fmtLines := splitLines(string(formatted))

	m := difflib.NewMatcher(origLines, fmtLines)
	ops := m.GetOpCodes()

	changed := make(map[int]bool)
	for _, op := range ops {
		if op.Tag == 'e' { // equal
			continue
		}
		// Mark any line that is present in the formatted output and
		// part of a non-equal opcode. (Deletes have J1==J2 and will not
		// mark lines.)
		for j := op.J1; j < op.J2; j++ {
			line := j + 1
			for ln := line - radius; ln <= line+radius; ln++ {
				if ln > 0 {
					changed[ln] = true
				}
			}
		}
		// If llformat only deleted lines, we still want to treat the
		// deletion point as "changed context" so nearby overflows can
		// be attributed.
		if op.Tag == 'd' && radius > 0 {
			line := op.J1 + 1
			for ln := line - radius; ln <= line+radius; ln++ {
				if ln > 0 {
					changed[ln] = true
				}
			}
		}
	}

	return changed
}

func extractSnippets(originalPath string, formatted []byte,
	overflows []overflow, llformatBin string, col,
	tabStop int) ([]snippet, error) {

	// Parsing the formatted file tends to match llformat's own syntax
	// handling best, and makes position math a lot simpler.
	fset := token.NewFileSet()
	f, err := parser.ParseFile(
		fset, originalPath, formatted,
		parser.ParseComments|parser.AllErrors,
	)
	if err != nil {

		// Keep going: we can still include raw context in the report.
		return make([]snippet, len(overflows)), err
	}
	tf := fset.File(f.Pos())
	if tf == nil {
		return make([]snippet, len(overflows)), fmt.Errorf(
			"missing token.File for %s", originalPath,
		)
	}

	out := make([]snippet, 0, len(overflows))
	for _, ov := range overflows {
		s := snippet{}
		pos, ok := safeLineStart(tf, ov.Line)
		if !ok {
			out = append(out, s)
			continue
		}

		startPos, endPos, kind := enclosingRange(f, pos)
		if startPos == token.NoPos || endPos == token.NoPos ||
			kind == "" {

			out = append(out, s)
			continue
		}

		startOff, endOff, ok := safeOffsets(tf, startPos, endPos)
		if !ok {
			out = append(out, s)
			continue
		}

		s.Kind = kind
		s.StartLine = tf.Position(startPos).Line
		s.EndLine = tf.Position(endPos).Line
		s.Text = string(formatted[startOff:endOff])

		// Build a standalone repro file (syntactically valid Go) that
		// can be pasted into a new fixture or shared in an issue.
		pkg := "p"
		if f.Name != nil && f.Name.Name != "" {
			pkg = f.Name.Name
		}
		s.ReproText = "package " + pkg + "\n\n" + s.Text + "\n"

		reproOK, why := reproCheck(
			llformatBin, col, tabStop, s.ReproText,
		)
		s.Reproduces = reproOK
		s.ReproWhy = why

		out = append(out, s)
	}

	return out, nil
}

func enclosingRange(f *ast.File, pos token.Pos) (start token.Pos, end token.Pos,
	kind string) {

	if fd := findEnclosingFuncDecl(f, pos); fd != nil {
		start = fd.Pos()
		if fd.Doc != nil {
			start = fd.Doc.Pos()
		}

		return start, fd.End(), "function"
	}
	if gd := findEnclosingGenDecl(f, pos); gd != nil {
		start = gd.Pos()
		if gd.Doc != nil {
			start = gd.Doc.Pos()
		}

		return start, gd.End(), "declaration"
	}
	if cg := findEnclosingCommentGroup(f, pos); cg != nil {
		return cg.Pos(), cg.End(), "comment"
	}

	return f.Pos(), f.End(), "file"
}

func findEnclosingFuncDecl(f *ast.File, pos token.Pos) *ast.FuncDecl {
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fd.Pos() <= pos && pos < fd.End() {
			return fd
		}
	}

	return nil
}

func findEnclosingGenDecl(f *ast.File, pos token.Pos) *ast.GenDecl {
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok {
			continue
		}
		if gd.Pos() <= pos && pos < gd.End() {
			return gd
		}
	}

	return nil
}

func findEnclosingCommentGroup(f *ast.File, pos token.Pos) *ast.CommentGroup {
	for _, cg := range f.Comments {
		if cg.Pos() <= pos && pos < cg.End() {
			return cg
		}
	}

	return nil
}

func safeLineStart(tf *token.File, line int) (token.Pos, bool) {
	pos := token.NoPos
	ok := false
	if tf == nil || line <= 0 {
		return pos, ok
	}
	// token.File panics if line is out-of-range.
	defer func() {
		if r := recover(); r != nil {
			pos = token.NoPos
			ok = false
		}
	}()
	pos = tf.LineStart(line)
	ok = true

	return pos, ok
}

func safeOffsets(tf *token.File, start, end token.Pos) (int, int, bool) {
	startOff := 0
	endOff := 0
	ok := false
	if tf == nil || start == token.NoPos || end == token.NoPos {
		return startOff, endOff, ok
	}
	defer func() {
		if r := recover(); r != nil {
			startOff = 0
			endOff = 0
			ok = false
		}
	}()
	startOff = tf.Offset(start)
	endOff = tf.Offset(end)
	if startOff < 0 || endOff < 0 || endOff < startOff {
		return 0, 0, false
	}
	ok = true

	return startOff, endOff, ok
}

func reproCheck(llformatBin string, col, tabStop int,
	src string) (bool, string) {

	tmp, err := os.CreateTemp("", "llformat-overflow-repro-*.go")
	if err != nil {
		return false, fmt.Sprintf("tempfile: %v", err)
	}
	defer func() {
		_ = os.Remove(tmp.Name())
	}()

	if _, err := tmp.WriteString(src); err != nil {
		_ = tmp.Close()

		return false, fmt.Sprintf("write: %v", err)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Sprintf("close: %v", err)
	}

	formatted, err := runLLFormat(llformatBin, col, tabStop, tmp.Name())
	if err != nil {
		return false, fmt.Sprintf("llformat: %v", err)
	}
	ovs := findOverflowsMinExcess(formatted, col, tabStop, 1)
	if len(ovs) == 0 {
		return false, "no overflows detected in standalone repro"
	}

	return true, ""
}

func min(a, b int) int {
	if a < b {
		return a
	}

	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}

	return b
}
