package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/lightninglabs/llformat/formatter"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type cliFlags struct {
	write              bool
	colLimit           int
	tabStop            int
	moveInline         bool
	multilineExclude   string
	logCallsMinTailLen int
	printPlan          bool
	fixpointIters      int
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
		&f.multilineExclude, "multiline-exclude", "", "comma-separate"+
			"d list of function names to exclude from multiline "+
			"formatting",
	)
	fs.IntVar(
		&f.logCallsMinTailLen, "logcalls-min-tail-len", 0, "minimum "+
			"tail length when splitting printf/logcall strings "+
			"in next profile (0 => default)",
	)
	fs.BoolVar(
		&f.printPlan, "print-plan", false,
		"print resolved pipeline plan and exit",
	)
	fs.IntVar(
		&f.fixpointIters, "fixpoint-iters", 0,
		"repeat full pipeline until stable (0=auto; default 3)",
	)

	if err := fs.Parse(args); err != nil {

		// The flag package already printed an error message to stderr.
		return 2
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

	if fs.NArg() != 1 {
		fs.Usage()

		return 2
	}

	path := fs.Arg(0)
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "read %s: %v\n", path, err)

		return 1
	}

	out := formatter.NewPipeline(cfg).Format(data)

	if f.write {
		return writeOutputToFile(path, out, stderr)
	}

	return writeOutputToWriter(out, stdout)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(
		w, "usage: llformat [-w] [--wrap-inline-comments] [--col N] "+
			"[--tab N] [--multiline-exclude FUNCS] "+
			"[--logcalls-min-tail-len N] [--fixpoint-iters N] "+
			"[--print-plan] <path>",
	)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "flags:")
	fmt.Fprintln(
		w, "  -w, --write               write result to (source) "+
			"file instead of stdout",
	)
	fmt.Fprintln(
		w, "  --col N                   column limit for formatting "+
			"(default 80)",
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
		w, "  --multiline-exclude FUNCS comma-separated function "+
			"names to exclude from multiline formatting",
	)
	fmt.Fprintln(
		w, "  --logcalls-min-tail-len N minimum tail length when "+
			"splitting printf/logcall strings (0 => default)",
	)
	fmt.Fprintln(
		w, "  --fixpoint-iters N        repeat full pipeline until "+
			"stable (0=auto; default 3)",
	)
	fmt.Fprintln(
		w, "  --print-plan              print resolved pipeline "+
			"plan and exit",
	)
}

func buildPipelineConfig(f cliFlags) (formatter.PipelineConfig, error) {
	excludes := parseCommaList(f.multilineExclude)

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
		ColumnLimit:           f.colLimit,
		TabStop:               f.tabStop,
		MoveInlineAbove:       f.moveInline,
		Excludes:              excludes,
		LogCallsMinTailLen:    f.logCallsMinTailLen,
		MaxPipelineIterations: fixpointIters,
	}

	if err := formatter.ValidatePipelineConfig(cfg); err != nil {
		return formatter.PipelineConfig{}, err
	}

	return cfg, nil
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
