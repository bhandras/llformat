package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/bhandras/llformat/dsl"
	"github.com/bhandras/llformat/formatter"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type cliFlags struct {
	write              bool
	version            bool
	colLimit           int
	tabStop            int
	moveInline         bool
	commentMode        string
	multilineExclude   string
	logCallsMinTailLen int
	logCallsNames      string
	logCallsPrefixes   string
	printPlan          bool
	printLogCalls      bool
	fixpointIters      int
	traceDSL           bool
	traceDSLReasons    bool
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("llformat", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		printUsage(stderr)
	}

	f := cliFlags{}

	fs.BoolVar(
		&f.write, "w", false,
		"write result to (source) file instead of stdout",
	)
	fs.BoolVar(
		&f.write, "write", false,
		"write result to (source) file instead of stdout",
	)
	fs.BoolVar(
		&f.version, "version", false,
		"print version information and exit",
	)
	fs.IntVar(&f.colLimit, "col", 80, "column limit for formatting")
	fs.IntVar(
		&f.tabStop, "tab", 8, "tab stop width for column calculations",
	)
	fs.BoolVar(
		&f.moveInline, "wrap-inline-comments", false, "when "+
			"formatting comments, hoist trailing inline "+
			"comments above for wrapping",
	)
	fs.StringVar(
		&f.commentMode, "comments", "overflow",
		"comment formatting mode: overflow, prose, or off",
	)
	fs.StringVar(
		&f.multilineExclude, "multiline-exclude", "", "comma-separat"+
			"ed list of function names to exclude from "+
			"multiline formatting",
	)
	fs.IntVar(
		&f.logCallsMinTailLen, "logcalls-min-tail-len", 0, "minimum "+
			"tail length when splitting printf/logcall strings "+
			"in next profile (0 => default)",
	)
	fs.StringVar(
		&f.logCallsNames, "logcalls-selector-names", "", "comma-sepa"+
			"rated list of selector/ident names to treat as "+
			"printf-style calls for suffix-only matching "+
			"(empty => built-in default)",
	)
	fs.StringVar(
		&f.logCallsPrefixes, "logcalls-selector-prefixes", "", "comm"+
			"a-separated list of selector receiver expression "+
			"prefixes to target for log/printf call formatting "+
			"(empty => match any selector prefix in next profile)",
	)
	fs.BoolVar(
		&f.printPlan, "print-plan", false,
		"print resolved pipeline plan and exit",
	)
	fs.BoolVar(
		&f.printLogCalls, "print-logcalls-patterns", false,
		"print log/printf call matching patterns and exit",
	)
	fs.BoolVar(
		&f.traceDSL, "trace-dsl", false,
		"trace applied DSL edits to stderr",
	)
	fs.BoolVar(
		&f.traceDSLReasons, "trace-dsl-reasons", false,
		"trace DSL rule skip/apply reasons to stderr",
	)
	fs.IntVar(
		&f.fixpointIters, "fixpoint-iters", 0,
		"repeat full pipeline until stable (0=auto; default 3)",
	)

	if err := fs.Parse(args); err != nil {

		// The flag package already printed an error message to stderr.
		return 2
	}

	if f.version || isVersionCommand(fs.Args()) {
		return runVersion(stdout)
	}

	cfg, err := buildPipelineConfig(f)
	if err != nil {
		fmt.Fprintln(stderr, err)
		fs.Usage()

		return 2
	}

	if f.printPlan {
		return runPrintPlan(stdout, cfg)
	}
	if f.printLogCalls {
		return runPrintLogCallsPatterns(stdout, cfg)
	}

	if fs.NArg() == 0 {
		fs.Usage()

		return 2
	}

	paths := fs.Args()
	if !f.write && len(paths) != 1 {
		fmt.Fprintln(
			stderr, "llformat: multiple paths require -w/--write",
		)
		fs.Usage()

		return 2
	}

	p := formatter.NewPipeline(cfg)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "read %s: %v\n", path, err)

			return 1
		}

		out := p.Format(data)

		if f.write {
			if code := writeOutputToFile(
				path, out, stderr,
			); code != 0 {
				return code
			}

			continue
		}

		return writeOutputToWriter(out, stdout)
	}

	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprintln(
		w, "usage:",
	)
	fmt.Fprintln(
		w, "  llformat [-w] [--wrap-inline-comments] [--comments "+
			"MODE] [--col N] [--tab N] [--multiline-exclude "+
			"FUNCS] [--logcalls-min-tail-len N] "+
			"[--logcalls-selector-names NAMES] "+
			"[--logcalls-selector-prefixes PREFIXES] "+
			"[--fixpoint-iters N] <path> [path ...]",
	)
	fmt.Fprintln(w, "  llformat --print-plan")
	fmt.Fprintln(w, "  llformat --print-logcalls-patterns")
	fmt.Fprintln(w, "  llformat version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "flags:")
	fmt.Fprintln(
		w, "  -w, --write               write result to (source) "+
			"file instead of stdout",
	)
	fmt.Fprintln(
		w, "  --col N                   column limit for "+
			"formatting (default 80)",
	)
	fmt.Fprintln(
		w, "  --tab N                   tab stop width for column "+
			"calculations (default 8)",
	)
	fmt.Fprintln(
		w, "  --wrap-inline-comments    hoist trailing inline "+
			"comments above for wrapping",
	)
	fmt.Fprintln(
		w, "  --comments MODE           comment formatting mode: "+
			"overflow, prose, or off (default overflow)",
	)
	fmt.Fprintln(
		w, "  --multiline-exclude FUNCS comma-separated function "+
			"names to exclude from multiline formatting",
	)
	fmt.Fprintln(
		w, "  --logcalls-min-tail-len N minimum tail length when "+
			"splitting printf/logcall strings (0 => default)",
	)
	fmt.Fprintln(
		w, "  --logcalls-selector-names NAMES comma-separated "+
			"selector/ident names for printf-style matching "+
			"(example: \"Infof,Errorf\")",
	)
	fmt.Fprintln(
		w, "  --logcalls-selector-prefixes PREFIXES "+
			"comma-separated receiver expression prefixes to "+
			"target (example: \"rpcSLog,zap.L().Sugar()\")",
	)
	fmt.Fprintln(
		w, "  --fixpoint-iters N        repeat full pipeline until "+
			"stable (0=auto; default 3)",
	)
	fmt.Fprintln(
		w, "  --print-plan              print resolved pipeline "+
			"plan and exit",
	)
	fmt.Fprintln(
		w, "  --print-logcalls-patterns print log/printf matching "+
			"patterns and exit",
	)
	fmt.Fprintln(
		w, "  --version                 print version information "+
			"and exit",
	)
	fmt.Fprintln(
		w, "  --trace-dsl               trace applied DSL edits to "+
			"stderr",
	)
	fmt.Fprintln(
		w, "  --trace-dsl-reasons       trace DSL rule skip/apply "+
			"reasons to stderr",
	)
}

func isVersionCommand(args []string) bool {
	return len(args) == 1 && args[0] == "version"
}

func runVersion(w io.Writer) int {
	info := versionInfo()
	fmt.Fprintf(w, "llformat version %s\n", info.Version)
	fmt.Fprintf(w, "commit %s\n", info.Commit)

	return 0
}

type versionDetails struct {
	Version string
	Commit  string
}

var (
	buildVersion = ""
	buildCommit  = ""
)

func versionInfo() versionDetails {
	version := buildVersion
	commit := buildCommit

	if bi, ok := debug.ReadBuildInfo(); ok {
		if version == "" && bi.Main.Version != "" &&
			bi.Main.Version != "(devel)" {

			version = bi.Main.Version
		}

		for _, setting := range bi.Settings {
			switch setting.Key {
			case "vcs.revision":
				if commit == "" {
					commit = shortCommit(setting.Value)
				}

			case "vcs.modified":
				if setting.Value == "true" && version != "" &&
					!strings.HasSuffix(version, "-dirty") {

					version += "-dirty"
				}
			}
		}
	}

	if version == "" {
		version = "dev"
	}
	if commit == "" {
		commit = "unknown"
	}

	return versionDetails{
		Version: version,
		Commit:  commit,
	}
}

func shortCommit(commit string) string {
	if len(commit) <= 12 {
		return commit
	}

	return commit[:12]
}

func buildPipelineConfig(f cliFlags) (formatter.PipelineConfig, error) {
	excludes := parseCommaList(f.multilineExclude)
	logCallsNames := parseCommaList(f.logCallsNames)
	logCallsPrefixes := parseCommaList(f.logCallsPrefixes)

	fixpointIters := f.fixpointIters
	if fixpointIters < 0 {
		return formatter.PipelineConfig{},
			fmt.Errorf("invalid flags: --fixpoint-iters must be " +
				">= 0")
	}
	if fixpointIters == 0 {
		fixpointIters = 3
	}

	cfg := formatter.PipelineConfig{
		ColumnLimit:              f.colLimit,
		TabStop:                  f.tabStop,
		MoveInlineAbove:          f.moveInline,
		CommentMode:              f.commentMode,
		Excludes:                 excludes,
		LogCallsMinTailLen:       f.logCallsMinTailLen,
		LogCallsSelectorNames:    logCallsNames,
		LogCallsSelectorPrefixes: logCallsPrefixes,
		MaxPipelineIterations:    fixpointIters,
		TraceDSL:                 f.traceDSL,
		TraceDSLReasons:          f.traceDSLReasons,
	}

	if err := formatter.ValidatePipelineConfig(cfg); err != nil {
		return formatter.PipelineConfig{}, err
	}

	return cfg, nil
}

func runPrintLogCallsPatterns(w io.Writer, cfg formatter.PipelineConfig) int {
	fmt.Fprintln(w, "log/printf call matching patterns:")
	fmt.Fprintln(w)

	selectorNames := cfg.LogCallsSelectorNames
	if len(selectorNames) == 0 {
		selectorNames = dsl.LogPrintfSelectorNames()
	}
	fmt.Fprintf(
		w, "- printf-style selector names (effective): %s\n",
		strings.Join(
			selectorNames, ", ",
		),
	)
	fmt.Fprintf(
		w, "- canonical exact patterns: %s\n",
		strings.Join(
			dsl.LogPrintfCanonicalPatterns(),
			", ",
		),
	)
	fmt.Fprintf(
		w, "- non-f string-call names (next): %s\n",
		strings.Join(
			dsl.NonFStringLogNames(),
			", ",
		),
	)
	if len(cfg.LogCallsSelectorPrefixes) == 0 {
		fmt.Fprintln(
			w, "- selector prefix allowlist: (none; next "+
				"matches any prefix)",
		)
	} else {
		fmt.Fprintf(
			w, "- selector prefix allowlist: %s\n",
			strings.Join(cfg.LogCallsSelectorPrefixes, ", "),
		)
	}

	return 0
}

func parseCommaList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}

	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}

	return out
}

func writeOutputToFile(path string, out []byte, stderr io.Writer) int {
	if err := os.WriteFile(path, out, 0644); err != nil {
		fmt.Fprintf(stderr, "write %s: %v\n", path, err)

		return 1
	}

	return 0
}

func writeOutputToWriter(out []byte, stdout io.Writer) int {
	if _, err := stdout.Write(out); err != nil {
		return 1
	}

	return 0
}

func runPrintPlan(w io.Writer, cfg formatter.PipelineConfig) int {
	plan := formatter.ResolvePipelinePlan(cfg)

	emit := func(format string, args ...any) bool {
		_, err := fmt.Fprintf(w, format, args...)

		return err == nil
	}

	if plan.DSLMultiLineStyle != "" && !emit(
		"dsl_multiline_style=%s\n", plan.DSLMultiLineStyle,
	) {
		return 1
	}
	if plan.DSLSigsStyle != "" &&
		!emit("dsl_sigs_style=%s\n", plan.DSLSigsStyle) {
		return 1
	}

	if !emit("dsl_sigs_native=%v\n", plan.UseDSLFuncSigsNative) {
		return 1
	}
	if !emit("dsl_blank_lines_native=%v\n", plan.UseDSLBlankLinesNative) {
		return 1
	}
	if !emit(
		"dsl_blank_lines_extra_if_err=%v\n",
		plan.DSLBlankLinesExtraIfErrReturn,
	) {
		return 1
	}
	if !emit("dsl_expr_logical_style=%s\n", plan.DSLExprLogicalStyle) {
		return 1
	}
	if !emit("dsl_expr_arithmetic_style=%s\n", plan.DSLExprArithmeticStyle) {
		return 1
	}
	if !emit("dsl_expr_case_clause_style=%s\n", plan.DSLExprCaseClauseStyle) {
		return 1
	}
	if !emit(
		"dsl_expr_selector_chain_style=%s\n",
		plan.DSLExprSelectorChainStyle,
	) {
		return 1
	}
	if !emit("dsl_call_args_allow=%v\n", plan.AllowDSLCallArgs) {
		return 1
	}
	if !emit("dsl_call_args_auto=%v\n", plan.AutoDSLCallArgs) {
		return 1
	}

	stageModes := map[string]formatter.StageMode{
		"blank-lines":     plan.StagePlan.BlankLines,
		"comments":        plan.StagePlan.Comments,
		"compact-calls":   plan.StagePlan.LogCalls,
		"expressions":     plan.StagePlan.Expressions,
		"multiline-calls": plan.StagePlan.MultiLineCalls,
		"signatures":      plan.StagePlan.Signatures,
	}
	keys := make([]string, 0, len(stageModes))
	for k := range stageModes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if !emit("stage.%s=%s\n", k, stageModes[k]) {
			return 1
		}
	}

	return 0
}
