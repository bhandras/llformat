package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"

	"github.com/lightninglabs/llformat/formatter"
)

func main() {
	var (
		write              bool
		colLimit           int
		tabStop            int
		moveInline         bool
		multilineExclude   string
		logCallsMinTailLen int
		printPlan          bool
		fixpointIters      int
	)

	printUsage := func() {
		fmt.Fprintln(os.Stderr, "usage: llformat [-w] [--wrap-inline-comments] [--col N] [--tab N] [--multiline-exclude FUNCS] [--logcalls-min-tail-len N] [--fixpoint-iters N] [--print-plan] <path>")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "flags:")
		fmt.Fprintln(os.Stderr, "  -w, --write               write result to (source) file instead of stdout")
		fmt.Fprintln(os.Stderr, "  --col N                   column limit for formatting (default 80)")
		fmt.Fprintln(os.Stderr, "  --tab N                   tab stop width for column calculations (default 8)")
		fmt.Fprintln(os.Stderr, "  --wrap-inline-comments    hoist trailing inline comments above for wrapping")
		fmt.Fprintln(os.Stderr, "  --multiline-exclude FUNCS comma-separated function names to exclude from multiline formatting")
		fmt.Fprintln(os.Stderr, "  --logcalls-min-tail-len N  minimum tail length when splitting printf/logcall strings (0 => default)")
		fmt.Fprintln(os.Stderr, "  --fixpoint-iters N         repeat full pipeline until stable (0=auto; default 3)")
		fmt.Fprintln(os.Stderr, "  --print-plan              print resolved pipeline plan and exit")
	}
	flag.Usage = printUsage

	flag.BoolVar(&write, "w", false, "write result to (source) file instead of stdout")
	flag.BoolVar(&write, "write", false, "write result to (source) file instead of stdout")
	flag.IntVar(&colLimit, "col", 80, "column limit for formatting")
	flag.IntVar(&tabStop, "tab", 8, "tab stop width for column calculations")
	flag.BoolVar(&moveInline, "wrap-inline-comments", false, "when formatting comments, hoist trailing inline comments above for wrapping")
	flag.StringVar(&multilineExclude, "multiline-exclude", "", "comma-separated list of function names to exclude from multiline formatting")
	flag.IntVar(&logCallsMinTailLen, "logcalls-min-tail-len", 0, "minimum tail length when splitting printf/logcall strings in next profile (0 => default)")
	flag.BoolVar(&printPlan, "print-plan", false, "print resolved pipeline plan and exit")
	flag.IntVar(&fixpointIters, "fixpoint-iters", 0, "repeat full pipeline until stable (0=auto; default 3)")
	flag.Parse()

	// Parse multiline exclude list
	var excludes []string
	if multilineExclude != "" {
		excludes = strings.Split(multilineExclude, ",")
		for i := range excludes {
			excludes[i] = strings.TrimSpace(excludes[i])
		}
	}

	// CLI fixpoint defaults:
	// - We prefer a small bounded fixpoint search so users don't have to run
	//   llformat multiple times on large files.
	autoFixpointIters := 3
	if fixpointIters < 0 {
		fmt.Fprintln(os.Stderr, "invalid flags: --fixpoint-iters must be >= 0")
		printUsage()
		os.Exit(2)
	}
	if fixpointIters == 0 {
		fixpointIters = autoFixpointIters
	}

	cfg := formatter.PipelineConfig{
		ColumnLimit:           colLimit,
		TabStop:               tabStop,
		MoveInlineAbove:       moveInline,
		Excludes:              excludes,
		LogCallsMinTailLen:    logCallsMinTailLen,
		MaxPipelineIterations: fixpointIters,
	}

	if err := formatter.ValidatePipelineConfig(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if printPlan {
		plan := formatter.ResolvePipelinePlan(cfg)
		if plan.DSLMultiLineStyle != "" {
			fmt.Fprintf(os.Stdout, "dsl_multiline_style=%s\n", plan.DSLMultiLineStyle)
		}
		if plan.DSLSigsStyle != "" {
			fmt.Fprintf(os.Stdout, "dsl_sigs_style=%s\n", plan.DSLSigsStyle)
		}
		fmt.Fprintf(os.Stdout, "dsl_sigs_native=%v\n", plan.UseDSLFuncSigsNative)
		fmt.Fprintf(os.Stdout, "dsl_blank_lines_native=%v\n", plan.UseDSLBlankLinesNative)
		fmt.Fprintf(os.Stdout, "dsl_blank_lines_extra_if_err=%v\n", plan.DSLBlankLinesExtraIfErrReturn)
		fmt.Fprintf(os.Stdout, "dsl_expr_logical_style=%s\n", plan.DSLExprLogicalStyle)
		fmt.Fprintf(os.Stdout, "dsl_expr_arithmetic_style=%s\n", plan.DSLExprArithmeticStyle)
		fmt.Fprintf(os.Stdout, "dsl_expr_case_clause_style=%s\n", plan.DSLExprCaseClauseStyle)
		fmt.Fprintf(os.Stdout, "dsl_expr_selector_chain_style=%s\n", plan.DSLExprSelectorChainStyle)
		fmt.Fprintf(os.Stdout, "dsl_call_args_allow=%v\n", plan.AllowDSLCallArgs)
		fmt.Fprintf(os.Stdout, "dsl_call_args_auto=%v\n", plan.AutoDSLCallArgs)

		stageModes := map[string]formatter.StageMode{
			"comments":        plan.StagePlan.Comments,
			"compact-calls":   plan.StagePlan.LogCalls,
			"expressions":     plan.StagePlan.Expressions,
			"multiline-calls": plan.StagePlan.MultiLineCalls,
			"signatures":      plan.StagePlan.Signatures,
			"blank-lines":     plan.StagePlan.BlankLines,
		}
		keys := make([]string, 0, len(stageModes))
		for k := range stageModes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(os.Stdout, "stage.%s=%s\n", k, stageModes[k])
		}
		return
	}

	if flag.NArg() != 1 {
		printUsage()
		os.Exit(2)
	}

	path := flag.Arg(0)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", path, err)
		os.Exit(1)
	}

	// Use the unified formatting pipeline
	pipeline := formatter.NewPipeline(cfg)
	out := pipeline.Format(data)

	if write {
		if err := ioutil.WriteFile(path, out, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
			os.Exit(1)
		}
		return
	}

	os.Stdout.Write(out)
}
