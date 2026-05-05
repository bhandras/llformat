package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bhandras/llformat/width"
	"github.com/pmezard/go-difflib/difflib"
)

type stringListFlag []string

func (f *stringListFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *stringListFlag) Set(v string) error {
	*f = append(*f, v)

	return nil
}

type report struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Config      reportConfig  `json:"config"`
	Repos       []repoSummary `json:"repos"`
	Cases       []caseRecord  `json:"cases"`
	Clusters    []cluster     `json:"clusters"`
}

type reportConfig struct {
	LLFormat        string   `json:"llformat"`
	Profile         string   `json:"profile"`
	ColumnLimit     int      `json:"column_limit"`
	TabStop         int      `json:"tab_stop"`
	CommentMode     string   `json:"comment_mode"`
	Redact          bool     `json:"redact"`
	ExcludeDirs     []string `json:"exclude_dirs"`
	ExcludeSuffix   []string `json:"exclude_suffixes"`
	MaxCasesPerFile int      `json:"max_cases_per_file"`
}

type repoSummary struct {
	Name                    string `json:"name"`
	Root                    string `json:"root"`
	FilesTotal              int    `json:"files_total"`
	FilesChanged            int    `json:"files_changed"`
	FilesWithCases          int    `json:"files_with_cases"`
	ParseFailuresBefore     int    `json:"parse_failures_before"`
	ParseFailuresAfter      int    `json:"parse_failures_after"`
	ASTInequivalentFiles    int    `json:"ast_inequivalent_files"`
	ASTStructuralDiffFiles  int    `json:"ast_structural_diff_files"`
	NonIdempotentFiles      int    `json:"non_idempotent_files"`
	LLFormatFailures        int    `json:"llformat_failures"`
	OriginalOverflowLines   int    `json:"original_overflow_lines"`
	FormattedOverflowLines  int    `json:"formatted_overflow_lines"`
	ChangedOverflowLines    int    `json:"changed_overflow_lines"`
	NewOverflowLines        int    `json:"new_overflow_lines"`
	ImprovedOverflowLineNet int    `json:"improved_overflow_line_net"`
}

type caseRecord struct {
	ID                  string   `json:"id"`
	Repo                string   `json:"repo"`
	RepoRoot            string   `json:"repo_root"`
	File                string   `json:"file"`
	AbsFile             string   `json:"abs_file"`
	Kind                string   `json:"kind"`
	Line                int      `json:"line"`
	Width               int      `json:"width"`
	OriginalWidth       int      `json:"original_same_line_width"`
	ColumnLimit         int      `json:"column_limit"`
	Text                string   `json:"text"`
	Syntax              string   `json:"syntax"`
	ChangedLine         bool     `json:"changed_line"`
	ParseOKBefore       bool     `json:"parse_ok_before"`
	ParseOKAfter        bool     `json:"parse_ok_after"`
	ASTEquivalent       bool     `json:"ast_equivalent"`
	ASTStrictEquivalent bool     `json:"ast_strict_equivalent"`
	ASTDiffKind         string   `json:"ast_diff_kind"`
	Idempotent          bool     `json:"idempotent"`
	EnclosingKind       string   `json:"enclosing_kind"`
	EnclosingStart      int      `json:"enclosing_start_line,omitempty"`
	EnclosingEnd        int      `json:"enclosing_end_line,omitempty"`
	NodeKind            string   `json:"node_kind"`
	NodePath            []string `json:"node_path,omitempty"`
	ClusterKey          string   `json:"cluster_key"`
}

type cluster struct {
	Key       string   `json:"key"`
	Count     int      `json:"count"`
	Kind      string   `json:"kind"`
	Syntax    string   `json:"syntax"`
	NodeKind  string   `json:"node_kind"`
	Examples  []string `json:"examples"`
	MaxWidth  int      `json:"max_width"`
	AvgExcess float64  `json:"avg_excess"`
}

type overflowLine struct {
	Line  int
	Width int
	Text  string
}

type fileAnalysis struct {
	relPath             string
	absPath             string
	original            []byte
	formatted           []byte
	changed             bool
	parseOKBefore       bool
	parseOKAfter        bool
	astEquivalent       bool
	astStrictEquivalent bool
	astDiffKind         string
	idempotent          bool
	llformatErr         error
	originalOverflows   []overflowLine
	formattedOverflows  []overflowLine
	changedLines        map[int]bool
	cases               []caseRecord
}

type astContext struct {
	EnclosingKind  string
	EnclosingStart int
	EnclosingEnd   int
	NodeKind       string
	NodePath       []string
}

func main() {
	var repos stringListFlag
	var excludeDirs stringListFlag
	var excludeSuffixes stringListFlag

	var (
		outDir = flag.String(
			"out", ".corpus_reports/latest", "directory to "+
				"write summary.md, clusters.md, and cases.json",
		)
		llformatBin = flag.String(
			"llformat", "./bin/llformat", "path to llformat binary",
		)
		profile = flag.String(
			"profile", "adoption",
			"diagnostic profile: adoption or all",
		)
		col = flag.Int("col", 80, "column limit")
		tab = flag.Int(
			"tab", width.DefaultTabStop, "tab stop",
		)
		commentMode = flag.String(
			"comments", "overflow",
			"comment formatting mode to pass to llformat",
		)
		redact = flag.Bool(
			"redact", true,
			"redact repo names, paths, and source text in reports",
		)
		maxCasesPerFile = flag.Int(
			"max-cases-per-file", 20,
			"maximum case records to emit per file",
		)
	)
	flag.Var(&repos, "repo", "target repo root to analyze (repeatable)")
	flag.Var(
		&excludeDirs, "exclude-dir",
		"directory name or repo-relative path to skip (repeatable)",
	)
	flag.Var(
		&excludeSuffixes, "exclude-suffix",
		"file suffix to skip, e.g. .pb.go (repeatable)",
	)
	flag.Parse()

	if len(repos) == 0 {
		fatalf("missing required --repo")
	}
	if *col <= 0 {
		fatalf("--col must be > 0")
	}
	if *tab <= 0 {
		fatalf("--tab must be > 0")
	}
	if *maxCasesPerFile <= 0 {
		fatalf("--max-cases-per-file must be > 0")
	}
	if _, err := os.Stat(*llformatBin); err != nil {
		fatalf("llformat not found at %q: %v", *llformatBin, err)
	}

	cfg := reportConfig{
		LLFormat:        *llformatBin,
		Profile:         *profile,
		ColumnLimit:     *col,
		TabStop:         *tab,
		CommentMode:     *commentMode,
		Redact:          *redact,
		ExcludeDirs:     defaultExcludeDirs(excludeDirs),
		ExcludeSuffix:   append([]string{}, excludeSuffixes...),
		MaxCasesPerFile: *maxCasesPerFile,
	}
	if err := applyProfile(&cfg); err != nil {
		fatalf("%v", err)
	}

	rep, err := buildReport(repos, cfg)
	if err != nil {
		fatalf("%v", err)
	}
	if err := writeReport(*outDir, rep); err != nil {
		fatalf("write report: %v", err)
	}

	fmt.Fprintf(
		os.Stderr, "wrote corpus report: %s\n", filepath.Clean(*outDir),
	)
}

func buildReport(repos []string, cfg reportConfig) (report, error) {
	rep := report{
		GeneratedAt: time.Now(),
		Config:      cfg,
	}

	for _, repoArg := range repos {
		root, err := filepath.Abs(repoArg)
		if err != nil {
			return report{}, fmt.Errorf("resolve repo %q: %w",
				repoArg, err)
		}
		info, err := os.Stat(root)
		if err != nil {
			return report{}, fmt.Errorf("stat repo %q: %w", root,
				err)
		}
		if !info.IsDir() {
			return report{}, fmt.Errorf("repo %q is not a "+
				"directory", root)
		}

		summary, cases, err := analyzeRepo(root, cfg)
		if err != nil {
			return report{}, err
		}
		rep.Repos = append(rep.Repos, summary)
		rep.Cases = append(rep.Cases, cases...)
	}

	if cfg.Redact {
		redactReport(&rep)
	}
	rep.Clusters = buildClusters(rep.Cases, cfg.ColumnLimit)

	return rep, nil
}

func analyzeRepo(root string, cfg reportConfig) (repoSummary, []caseRecord,
	error) {

	files, err := goFilesUnder(root, cfg.ExcludeDirs, cfg.ExcludeSuffix)
	if err != nil {
		return repoSummary{}, nil, fmt.Errorf("scan %s: %w", root, err)
	}

	summary := repoSummary{
		Name: filepath.Base(root),
		Root: root,
	}
	var allCases []caseRecord
	for _, path := range files {
		summary.FilesTotal++

		analysis := analyzeFile(root, path, cfg)
		accumulateSummary(&summary, analysis)
		if len(analysis.cases) > 0 {
			summary.FilesWithCases++
			allCases = append(allCases, analysis.cases...)
		}
	}
	summary.ImprovedOverflowLineNet = summary.OriginalOverflowLines -
		summary.FormattedOverflowLines

	return summary, allCases, nil
}

func analyzeFile(root, path string, cfg reportConfig) fileAnalysis {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	analysis := fileAnalysis{
		relPath:      filepath.ToSlash(rel),
		absPath:      path,
		changedLines: make(map[int]bool),
	}

	original, err := os.ReadFile(path)
	if err != nil {
		analysis.llformatErr = fmt.Errorf("read: %w", err)

		return analysis
	}
	analysis.original = original
	analysis.parseOKBefore = parseOK(original)
	analysis.originalOverflows = findOverflows(
		original, cfg.ColumnLimit, cfg.TabStop,
	)

	formatted, err := runLLFormat(
		cfg.LLFormat, cfg.ColumnLimit, cfg.TabStop, cfg.CommentMode,
		path,
	)
	if err != nil {
		analysis.llformatErr = err

		return analysis
	}
	analysis.formatted = formatted
	analysis.changed = !bytes.Equal(original, formatted)
	analysis.parseOKAfter = parseOK(formatted)
	analysis.formattedOverflows = findOverflows(
		formatted, cfg.ColumnLimit, cfg.TabStop,
	)
	analysis.changedLines = changedLines(original, formatted, 0)
	analysis.astStrictEquivalent, analysis.astEquivalent,
		analysis.astDiffKind = classifyASTDiff(
		original, formatted,
	)
	analysis.idempotent = checkIdempotent(formatted, cfg)
	analysis.cases = casesForFile(root, analysis, cfg)

	return analysis
}

func accumulateSummary(summary *repoSummary, analysis fileAnalysis) {
	if analysis.llformatErr != nil {
		summary.LLFormatFailures++

		return
	}
	if analysis.changed {
		summary.FilesChanged++
	}
	if !analysis.parseOKBefore {
		summary.ParseFailuresBefore++
	}
	if !analysis.parseOKAfter {
		summary.ParseFailuresAfter++
	}
	if !analysis.astStrictEquivalent {
		summary.ASTInequivalentFiles++
	}
	if analysis.astDiffKind == "structural" {
		summary.ASTStructuralDiffFiles++
	}
	if !analysis.idempotent {
		summary.NonIdempotentFiles++
	}
	summary.OriginalOverflowLines += len(analysis.originalOverflows)
	summary.FormattedOverflowLines += len(analysis.formattedOverflows)
	for _, ov := range analysis.formattedOverflows {
		if analysis.changedLines[ov.Line] {
			summary.ChangedOverflowLines++
		}
	}
	for _, c := range analysis.cases {
		if c.Kind == "new_overflow" {
			summary.NewOverflowLines++
		}
	}
}

func casesForFile(root string, analysis fileAnalysis,
	cfg reportConfig) []caseRecord {

	var cases []caseRecord
	if analysis.llformatErr != nil {
		return nil
	}

	ctx := astContextsByLine(analysis.formatted)
	repoName := filepath.Base(root)
	originalOverflowTexts := make(
		map[string]int, len(analysis.originalOverflows),
	)
	for _, ov := range analysis.originalOverflows {
		originalOverflowTexts[ov.Text]++
	}
	for _, ov := range analysis.formattedOverflows {
		origWidth := originalSameLineWidth(
			analysis.original, ov.Line, cfg.TabStop,
		)
		changed := analysis.changedLines[ov.Line]
		kind := classifyCase(
			ov.Width, origWidth, changed, cfg.ColumnLimit,
			originalOverflowTexts[ov.Text] > 0,
		)
		if kind == "unchanged_overflow" {
			continue
		}

		ac := ctx[ov.Line]
		syntax := classifySyntax(ov.Text, ac)
		rec := caseRecord{
			ID: caseID(
				repoName, analysis.relPath, ov.Line, ov.Text,
			),
			Repo:                repoName,
			RepoRoot:            root,
			File:                analysis.relPath,
			AbsFile:             analysis.absPath,
			Kind:                kind,
			Line:                ov.Line,
			Width:               ov.Width,
			OriginalWidth:       origWidth,
			ColumnLimit:         cfg.ColumnLimit,
			Text:                ov.Text,
			Syntax:              syntax,
			ChangedLine:         changed,
			ParseOKBefore:       analysis.parseOKBefore,
			ParseOKAfter:        analysis.parseOKAfter,
			ASTEquivalent:       analysis.astEquivalent,
			ASTStrictEquivalent: analysis.astStrictEquivalent,
			ASTDiffKind:         analysis.astDiffKind,
			Idempotent:          analysis.idempotent,
			EnclosingKind:       ac.EnclosingKind,
			EnclosingStart:      ac.EnclosingStart,
			EnclosingEnd:        ac.EnclosingEnd,
			NodeKind:            ac.NodeKind,
			NodePath:            ac.NodePath,
		}
		rec.ClusterKey = clusterKey(rec)
		cases = append(cases, rec)
	}

	sort.Slice(
		cases,
		func(i, j int) bool {
			return caseRank(cases[i]) > caseRank(cases[j])
		},
	)
	if len(cases) > cfg.MaxCasesPerFile {
		cases = cases[:cfg.MaxCasesPerFile]
	}
	sort.Slice(
		cases,
		func(i, j int) bool {
			if cases[i].File != cases[j].File {
				return cases[i].File < cases[j].File
			}

			return cases[i].Line < cases[j].Line
		},
	)

	return cases
}

func classifyCase(width, originalWidth int, changed bool, col int,
	existingOverflowText bool) string {

	if changed && originalWidth <= col {
		return "new_overflow"
	}
	if changed {
		return "touched_overflow"
	}
	if originalWidth <= col && width > col {
		if existingOverflowText {
			return "moved_existing_overflow"
		}

		return "shifted_overflow"
	}

	return "unchanged_overflow"
}

func classifySyntax(line string, ctx astContext) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "//") ||
		strings.HasPrefix(trimmed, "/*") {
		return "comment"
	}
	if strings.Contains(trimmed, "`") || strings.Contains(trimmed, "\"") {
		return "string"
	}
	if strings.Contains(trimmed, ").") ||
		strings.HasPrefix(trimmed, ".") ||
		strings.Count(trimmed, ".") >= 3 {
		return "method_chain"
	}
	if strings.Contains(trimmed, "func(") ||
		strings.HasPrefix(trimmed, "func ") {
		return "signature"
	}
	if strings.Contains(trimmed, "&&") || strings.Contains(trimmed, "||") {
		return "logical_expr"
	}
	if strings.Contains(trimmed, "{") || strings.Contains(trimmed, "}") {
		return "composite_or_block"
	}
	switch ctx.NodeKind {
	case "*ast.CallExpr":
		return "call"

	case "*ast.FuncDecl", "*ast.FuncLit", "*ast.FuncType":
		return "signature"

	case "*ast.CompositeLit":
		return "composite_lit"

	case "*ast.BinaryExpr":
		return "binary_expr"
	}

	return "other"
}

func clusterKey(c caseRecord) string {
	node := c.NodeKind
	if node == "" {
		node = "unknown_node"
	}

	return c.Kind + "|" + c.Syntax + "|" + node
}

func caseRank(c caseRecord) int {
	score := c.Width - c.ColumnLimit
	switch c.Kind {
	case "new_overflow":
		score += 1000

	case "touched_overflow":
		score += 500

	case "shifted_overflow":
		score += 250

	case "moved_existing_overflow":
		score += 50
	}
	if !c.ParseOKAfter || !c.ASTEquivalent || !c.Idempotent {
		score += 2000
	}

	return score
}

func buildClusters(cases []caseRecord, col int) []cluster {
	byKey := make(map[string]*cluster)
	for _, c := range cases {
		cl := byKey[c.ClusterKey]
		if cl == nil {
			cl = &cluster{
				Key:      c.ClusterKey,
				Kind:     c.Kind,
				Syntax:   c.Syntax,
				NodeKind: c.NodeKind,
			}
			byKey[c.ClusterKey] = cl
		}
		cl.Count++
		if c.Width > cl.MaxWidth {
			cl.MaxWidth = c.Width
		}
		cl.AvgExcess += float64(c.Width - col)
		if len(cl.Examples) < 5 {
			cl.Examples = append(
				cl.Examples, fmt.Sprintf("%s:%s:%d width=%d",
					c.Repo, c.File, c.Line, c.Width),
			)
		}
	}

	clusters := make([]cluster, 0, len(byKey))
	for _, cl := range byKey {
		if cl.Count > 0 {
			cl.AvgExcess /= float64(cl.Count)
		}
		clusters = append(clusters, *cl)
	}
	sort.Slice(
		clusters,
		func(i, j int) bool {
			if clusters[i].Count != clusters[j].Count {
				return clusters[i].Count > clusters[j].Count
			}
			if clusters[i].MaxWidth != clusters[j].MaxWidth {
				return clusters[i].MaxWidth > clusters[j].MaxWidth
			}

			return clusters[i].Key < clusters[j].Key
		},
	)

	return clusters
}

func goFilesUnder(root string, excludeDirs,
	excludeSuffixes []string) ([]string, error) {

	var files []string
	err := filepath.WalkDir(
		root,
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if shouldSkipDir(
					root, path, d.Name(), excludeDirs,
				) {
					return filepath.SkipDir
				}

				return nil
			}
			if filepath.Ext(path) != ".go" {
				return nil
			}
			for _, suffix := range excludeSuffixes {
				if suffix != "" && strings.HasSuffix(
					path, suffix,
				) {
					return nil
				}
			}
			files = append(files, path)

			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	return files, nil
}

func shouldSkipDir(root, path, name string, excludeDirs []string) bool {
	for _, ex := range excludeDirs {
		ex = strings.TrimSpace(ex)
		if ex == "" {
			continue
		}
		if name == ex {
			return true
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		ex = filepath.ToSlash(filepath.Clean(ex))
		if rel == ex || strings.HasPrefix(rel, ex+"/") {
			return true
		}
	}

	return false
}

func defaultExcludeDirs(extra []string) []string {
	out := []string{
		".git",
		".gocache",
		".gomodcache",
		"vendor",
		"third_party",
		"testdata",
	}
	seen := make(map[string]struct{}, len(out)+len(extra))
	uniq := out[:0]
	for _, d := range append(out, extra...) {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		uniq = append(uniq, d)
	}

	return uniq
}

func applyProfile(cfg *reportConfig) error {
	switch cfg.Profile {
	case "", "adoption":
		cfg.Profile = "adoption"
		cfg.ExcludeDirs = mergeStringLists(
			cfg.ExcludeDirs,
			[]string{
				"generated",
				"gen",
			},
		)
		cfg.ExcludeSuffix = mergeStringLists(
			cfg.ExcludeSuffix,
			[]string{
				".pb.go",
				".pb.gw.go",
				".pb.validate.go",
				".connect.go",
				".gen.go",
				"_gen.go",
				"_generated.go",
			},
		)

	case "all":
	default:
		return fmt.Errorf("unknown --profile %q: want adoption or all",
			cfg.Profile)
	}

	return nil
}

func mergeStringLists(lists ...[]string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, list := range lists {
		for _, value := range list {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}

	return out
}

func runLLFormat(llformatBin string, col, tabStop int, commentMode string,
	path string) ([]byte, error) {

	cmd := exec.Command(
		llformatBin, "--col", strconv.Itoa(col),
		"--tab", strconv.Itoa(tabStop),
		"--comments", commentMode, path,
	)
	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return nil, fmt.Errorf("%w: %s", err,
			strings.TrimSpace(string(ee.Stderr)))
	}

	return nil, err
}

func parseOK(src []byte) bool {
	fset := token.NewFileSet()
	_, err := parser.ParseFile(
		fset, "in.go", src, parser.ParseComments|parser.AllErrors,
	)

	return err == nil
}

func classifyASTDiff(before, after []byte) (strictEquivalent bool,
	safeEquivalent bool, kind string) {

	if astEquivalent(before, after) {
		return true, true, "none"
	}
	if astEquivalentFoldedStringConcat(before, after) {
		return false, true, "string_const_rewrite"
	}

	return false, false, "structural"
}

func astEquivalent(before, after []byte) bool {
	a, err := canonicalASTDump(before)
	if err != nil {
		return false
	}
	b, err := canonicalASTDump(after)
	if err != nil {
		return false
	}

	return a == b
}

func astEquivalentFoldedStringConcat(before, after []byte) bool {
	a, err := canonicalASTDumpFoldedStringConcat(before)
	if err != nil {
		return false
	}
	b, err := canonicalASTDumpFoldedStringConcat(after)
	if err != nil {
		return false
	}

	return a == b
}

func canonicalASTDump(src []byte) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(
		fset, "in.go", src, parser.AllErrors|parser.ParseComments,
	)
	if err != nil {
		return "", err
	}

	stripASTMetadata(file)

	var b bytes.Buffer
	if err := format.Node(&b, token.NewFileSet(), file); err != nil {
		return "", err
	}

	return b.String(), nil
}

func canonicalASTDumpFoldedStringConcat(src []byte) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(
		fset, "in.go", src, parser.AllErrors|parser.ParseComments,
	)
	if err != nil {
		return "", err
	}

	stripASTMetadata(file)
	normalizeStringConcatInAST(file)

	var b bytes.Buffer
	if err := format.Node(&b, token.NewFileSet(), file); err != nil {
		return "", err
	}

	return b.String(), nil
}

func normalizeStringConcatInAST(v any) {
	normalizeValue(reflect.ValueOf(v))
}

func normalizeValue(v reflect.Value) {
	if !v.IsValid() {
		return
	}
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if !f.CanSet() && f.Kind() != reflect.Pointer &&
				f.Kind() != reflect.Interface &&
				f.Kind() != reflect.Slice &&
				f.Kind() != reflect.Struct {

				continue
			}
			if f.CanSet() && f.Type().Implements(exprType) {
				if expr, ok := f.Interface().(ast.Expr); ok {
					f.Set(
						reflect.ValueOf(
							normalizeExpr(expr),
						),
					)
				}
				continue
			}
			normalizeValue(f)
		}

	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			item := v.Index(i)
			if item.CanSet() && item.Type().Implements(exprType) {
				if expr, ok := item.Interface().(ast.Expr); ok {
					item.Set(
						reflect.ValueOf(
							normalizeExpr(expr),
						),
					)
				}
				continue
			}
			normalizeValue(item)
		}
	}
}

var exprType = reflect.TypeOf((*ast.Expr)(nil)).Elem()

func normalizeExpr(expr ast.Expr) ast.Expr {
	if expr == nil {
		return nil
	}
	rv := reflect.ValueOf(expr)
	if !rv.IsValid() || (rv.Kind() == reflect.Pointer && rv.IsNil()) {
		return expr
	}
	normalizeValue(reflect.ValueOf(expr))
	if text, ok := flattenStringConstExpr(expr); ok {
		return &ast.BasicLit{
			Kind:  token.STRING,
			Value: strconv.Quote(text),
		}
	}

	return expr
}

func flattenStringConstExpr(expr ast.Expr) (string, bool) {
	if expr == nil {
		return "", false
	}
	rv := reflect.ValueOf(expr)
	if !rv.IsValid() || (rv.Kind() == reflect.Pointer && rv.IsNil()) {
		return "", false
	}
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		text, err := strconv.Unquote(e.Value)
		if err != nil {
			return "", false
		}

		return text, true

	case *ast.ParenExpr:
		return flattenStringConstExpr(e.X)

	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		left, ok := flattenStringConstExpr(e.X)
		if !ok {
			return "", false
		}
		right, ok := flattenStringConstExpr(e.Y)
		if !ok {
			return "", false
		}

		return left + right, true
	}

	return "", false
}

func stripASTMetadata(node ast.Node) {
	ast.Inspect(
		node,
		func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.File:
				v.Scope = nil
				v.Unresolved = nil
				v.Comments = nil

			case *ast.Ident:
				v.Obj = nil

			case *ast.GenDecl:
				v.Doc = nil

			case *ast.FuncDecl:
				v.Doc = nil

			case *ast.Field:
				v.Doc = nil
				v.Comment = nil

			case *ast.ImportSpec:
				v.Doc = nil
				v.Comment = nil

			case *ast.TypeSpec:
				v.Doc = nil
				v.Comment = nil

			case *ast.ValueSpec:
				v.Doc = nil
				v.Comment = nil
			}

			return true
		},
	)
}

func checkIdempotent(src []byte, cfg reportConfig) bool {
	tmp, err := os.CreateTemp("", "llformat-corpus-check-*.go")
	if err != nil {
		return false
	}
	defer func() {
		_ = os.Remove(tmp.Name())
	}()
	if _, err := tmp.Write(src); err != nil {
		_ = tmp.Close()

		return false
	}
	if err := tmp.Close(); err != nil {
		return false
	}
	out, err := runLLFormat(
		cfg.LLFormat, cfg.ColumnLimit, cfg.TabStop, cfg.CommentMode,
		tmp.Name(),
	)
	if err != nil {
		return false
	}

	return bytes.Equal(src, out)
}

func splitLines(src []byte) []string {
	if len(src) == 0 {
		return nil
	}
	lines := strings.SplitAfter(string(src), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}

func findOverflows(src []byte, colLimit, tabStop int) []overflowLine {
	var out []overflowLine
	var prevLine string
	for idx, line := range splitLines(src) {
		line = strings.TrimSuffix(line, "\n")
		if hasLLNolint(line) || hasStandaloneLLNolint(prevLine) {
			prevLine = line
			continue
		}
		w := width.VisualLenWithTab(line, tabStop)
		if w > colLimit {
			out = append(
				out, overflowLine{
					Line:  idx + 1,
					Width: w,
					Text:  line,
				},
			)
		}
		prevLine = line
	}

	return out
}

func hasStandaloneLLNolint(line string) bool {
	trimmed := strings.TrimSpace(line)

	return strings.HasPrefix(trimmed, "//") && hasLLNolint(trimmed)
}

func hasLLNolint(line string) bool {
	idx := strings.Index(line, "//nolint")
	if idx < 0 {
		return false
	}
	directive := line[idx+len("//nolint"):]
	if directive == "" {
		return false
	}
	if !strings.HasPrefix(directive, ":") {
		return false
	}
	for _, name := range strings.Split(directive[1:], ",") {
		name = strings.TrimSpace(name)
		if fields := strings.Fields(name); len(fields) > 0 {
			name = fields[0]
		}
		if name == "ll" {
			return true
		}
	}

	return false
}

func originalSameLineWidth(src []byte, line, tabStop int) int {
	lines := splitLines(src)
	if line <= 0 || line > len(lines) {
		return 0
	}
	if tabStop <= 0 {
		tabStop = width.DefaultTabStop
	}

	return width.VisualLenWithTab(
		strings.TrimSuffix(lines[line-1], "\n"), tabStop,
	)
}

func changedLines(original, formatted []byte, radius int) map[int]bool {
	origLines := splitLines(original)
	fmtLines := splitLines(formatted)

	matcher := difflib.NewMatcher(origLines, fmtLines)
	ops := matcher.GetOpCodes()

	changed := make(map[int]bool)
	for _, op := range ops {
		if op.Tag == 'e' {
			continue
		}
		for j := op.J1; j < op.J2; j++ {
			line := j + 1
			for ln := line - radius; ln <= line+radius; ln++ {
				if ln > 0 {
					changed[ln] = true
				}
			}
		}
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

func astContextsByLine(src []byte) map[int]astContext {
	ctx := make(map[int]astContext)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(
		fset, "out.go", src, parser.ParseComments|parser.AllErrors,
	)
	if err != nil || file == nil {
		return ctx
	}
	tf := fset.File(file.Pos())
	if tf == nil {
		return ctx
	}

	lineCount := len(splitLines(src))
	for line := 1; line <= lineCount; line++ {
		pos, ok := safeLineStart(tf, line)
		if !ok {
			continue
		}
		ctx[line] = astContextForPos(fset, file, pos)
	}

	return ctx
}

func astContextForPos(fset *token.FileSet, file *ast.File,
	pos token.Pos) astContext {

	var stack []string
	var best ast.Node
	ast.Inspect(
		file,
		func(n ast.Node) bool {
			if n == nil {
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}

				return true
			}
			kind := fmt.Sprintf("%T", n)
			if n.Pos() <= pos && pos < n.End() {
				stack = append(stack, kind)
				best = n

				return true
			}

			return false
		},
	)

	out := astContext{
		NodePath: append([]string{}, stack...),
	}
	if best != nil {
		out.NodeKind = fmt.Sprintf("%T", best)
	}
	start, end, kind := enclosingRange(file, pos)
	out.EnclosingKind = kind
	out.EnclosingStart = fset.Position(start).Line
	out.EnclosingEnd = fset.Position(end).Line

	return out
}

func enclosingRange(f *ast.File, pos token.Pos) (token.Pos, token.Pos, string) {
	if fd := findEnclosingFuncDecl(f, pos); fd != nil {
		start := fd.Pos()
		if fd.Doc != nil {
			start = fd.Doc.Pos()
		}

		return start, fd.End(), "function"
	}
	if gd := findEnclosingGenDecl(f, pos); gd != nil {
		start := gd.Pos()
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

func caseID(repo, file string, line int, text string) string {
	h := sha1.New()
	_, _ = h.Write([]byte(repo))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(file))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strconv.Itoa(line)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(text))

	return hex.EncodeToString(h.Sum(nil))[:12]
}

func redactReport(rep *report) {
	repoNames := make(map[string]string, len(rep.Repos))
	for i := range rep.Repos {
		redacted := fmt.Sprintf("repo%d", i+1)
		repoNames[rep.Repos[i].Name] = redacted
		rep.Repos[i].Name = redacted
		rep.Repos[i].Root = ""
	}

	fileNames := make(map[string]string)
	nextFile := 1
	for i := range rep.Cases {
		c := &rep.Cases[i]
		if name, ok := repoNames[c.Repo]; ok {
			c.Repo = name
		}
		c.RepoRoot = ""
		c.AbsFile = ""
		fileKey := c.Repo + "\x00" + c.File
		redactedFile, ok := fileNames[fileKey]
		if !ok {
			redactedFile = fmt.Sprintf("file_%04d.go", nextFile)
			nextFile++
			fileNames[fileKey] = redactedFile
		}
		c.File = redactedFile
		c.Text = redactSourceShape(c.Text)
		c.ID = caseID(c.Repo, c.File, c.Line, c.Text)
		c.ClusterKey = clusterKey(*c)
	}
}

func redactSourceShape(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteByte('a')

		case r >= 'A' && r <= 'Z':
			b.WriteByte('A')

		case r >= '0' && r <= '9':
			b.WriteByte('0')

		case r == '_' || r == '-':
			b.WriteRune(r)

		case r == '\t' || r == ' ':
			b.WriteRune(r)

		case r == '"' || r == '`' || r == '\'':
			b.WriteRune(r)

		default:
			b.WriteRune(r)
		}
	}

	return b.String()
}

func writeReport(outDir string, rep report) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	casesJSON, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(
		filepath.Join(outDir, "cases.json"), casesJSON, 0o644,
	); err != nil {
		return err
	}
	if err := os.WriteFile(
		filepath.Join(outDir, "summary.md"),
		[]byte(
			renderSummary(rep),
		),
		0o644,
	); err != nil {
		return err
	}
	if err := os.WriteFile(
		filepath.Join(outDir, "clusters.md"),
		[]byte(
			renderClusters(rep),
		),
		0o644,
	); err != nil {
		return err
	}

	return nil
}

func renderSummary(rep report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# llformat Corpus Check\n\n")
	fmt.Fprintf(
		&b, "Generated: `%s`\n\n", rep.GeneratedAt.Format(time.RFC3339),
	)
	fmt.Fprintf(&b, "- llformat: `%s`\n", rep.Config.LLFormat)
	fmt.Fprintf(&b, "- profile: `%s`\n", rep.Config.Profile)
	fmt.Fprintf(&b, "- column limit: `%d`\n", rep.Config.ColumnLimit)
	fmt.Fprintf(&b, "- tab stop: `%d`\n", rep.Config.TabStop)
	fmt.Fprintf(&b, "- comment mode: `%s`\n", rep.Config.CommentMode)
	fmt.Fprintf(&b, "- redacted: `%t`\n", rep.Config.Redact)
	fmt.Fprintf(
		&b, "- exclude dirs: `%s`\n",
		strings.Join(rep.Config.ExcludeDirs, ", "),
	)
	fmt.Fprintf(
		&b, "- exclude suffixes: `%s`\n",
		strings.Join(rep.Config.ExcludeSuffix, ", "),
	)
	fmt.Fprintf(&b, "- emitted cases: `%d`\n", len(rep.Cases))
	fmt.Fprintf(&b, "- clusters: `%d`\n\n", len(rep.Clusters))

	fmt.Fprintf(&b, "## Repositories\n\n")
	fmt.Fprintf(
		&b, "| repo | files | changed | case files | before ov | "+
			"after ov | new ov | changed ov | parse after fail "+
			"| AST diff | structural diff | non-idem | "+
			"llformat fail |\n",
	)
	fmt.Fprintf(
		&b, "| --- | ---: | ---: | ---: | ---: | ---: | ---: | "+
			"---: | ---: | ---: | ---: | ---: | ---: |\n",
	)
	for _, r := range rep.Repos {
		fmt.Fprintf(
			&b, "| `%s` | %d | %d | %d | %d | %d | %d | %d | %d "+
				"| %d | %d | %d | %d |\n", r.Name, r.FilesTotal,
			r.FilesChanged, r.FilesWithCases,
			r.OriginalOverflowLines, r.FormattedOverflowLines,
			r.NewOverflowLines, r.ChangedOverflowLines,
			r.ParseFailuresAfter, r.ASTInequivalentFiles,
			r.ASTStructuralDiffFiles, r.NonIdempotentFiles,
			r.LLFormatFailures,
		)
	}

	fmt.Fprintf(&b, "\n## Top Cases\n\n")
	top := append([]caseRecord{}, rep.Cases...)
	sort.Slice(
		top,
		func(i, j int) bool { return caseRank(top[i]) > caseRank(top[j]) },
	)
	if len(top) > 25 {
		top = top[:25]
	}
	fmt.Fprintf(
		&b,
		"| kind | syntax | file | line | width | node |\n",
	)
	fmt.Fprintf(&b, "| --- | --- | --- | ---: | ---: | --- |\n")
	for _, c := range top {
		fmt.Fprintf(
			&b, "| `%s` | `%s` | `%s:%s` | %d | %d | `%s` |\n",
			c.Kind, c.Syntax, c.Repo, c.File, c.Line, c.Width,
			c.NodeKind,
		)
	}

	return b.String()
}

func renderClusters(rep report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# llformat Corpus Clusters\n\n")
	fmt.Fprintf(
		&b, "Generated: `%s`\n\n", rep.GeneratedAt.Format(time.RFC3339),
	)
	for _, c := range rep.Clusters {
		fmt.Fprintf(&b, "## `%s`\n\n", c.Key)
		fmt.Fprintf(&b, "- count: `%d`\n", c.Count)
		fmt.Fprintf(&b, "- max width: `%d`\n", c.MaxWidth)
		fmt.Fprintf(&b, "- average excess: `%.1f`\n", c.AvgExcess)
		fmt.Fprintf(&b, "- kind: `%s`\n", c.Kind)
		fmt.Fprintf(&b, "- syntax: `%s`\n", c.Syntax)
		fmt.Fprintf(&b, "- node: `%s`\n\n", c.NodeKind)
		fmt.Fprintf(&b, "Examples:\n\n")
		for _, ex := range c.Examples {
			fmt.Fprintf(&b, "- `%s`\n", ex)
		}
		fmt.Fprintf(&b, "\n")
	}

	return b.String()
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
