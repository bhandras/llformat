package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/lightninglabs/llformat/formatter"
)

func main() {
	var (
		write             bool
		colLimit          int
		tabStop           int
		moveInline        bool
		multilineExclude  string
		useLegacy         bool
		traceDSL          bool
		useDSLComments    bool
		useDSLCalls       bool
		useDSLMultiLine   bool
		dslMultiLineStyle string
		dslCallPolicy     string
		useDSLExpr        bool
		useDSLSigs        bool
		useDSLBlankLines  bool
		allowDSLCallArgs  bool
		autoDSLCallArgs   bool
	)

	flag.BoolVar(&write, "w", false, "write result to (source) file instead of stdout")
	flag.BoolVar(&write, "write", false, "write result to (source) file instead of stdout")
	flag.IntVar(&colLimit, "col", 80, "column limit for formatting")
	flag.IntVar(&tabStop, "tab", 8, "tab stop width for column calculations")
	flag.BoolVar(&moveInline, "wrap-inline-comments", false, "when formatting comments, hoist trailing inline comments above for wrapping")
	flag.StringVar(&multilineExclude, "multiline-exclude", "", "comma-separated list of function names to exclude from multiline formatting")
	flag.BoolVar(&useLegacy, "legacy", false, "use legacy multi-stage formatter instead of DSL")
	flag.BoolVar(&traceDSL, "trace-dsl", false, "print DSL rule application trace to stderr (DSL mode only)")
	flag.BoolVar(&useDSLComments, "dsl-comments", true, "use DSL comment formatter (delegates to legacy; DSL mode only)")
	flag.BoolVar(&useDSLCalls, "dsl-calls", true, "use DSL log/printf call formatter (DSL mode only)")
	flag.BoolVar(&useDSLMultiLine, "dsl-multiline-calls", true, "use DSL multiline call formatter (DSL mode only)")
	flag.StringVar(&dslMultiLineStyle, "dsl-multiline-style", "legacy", "DSL multiline call style: legacy|packed|packed-chain (DSL mode only)")
	flag.StringVar(&dslCallPolicy, "dsl-call-policy", "legacy", "DSL call policy bundle: legacy|modern (DSL mode only)")
	flag.BoolVar(&useDSLExpr, "dsl-expr", true, "use DSL expression formatter (DSL mode only)")
	flag.BoolVar(&useDSLSigs, "dsl-sigs", true, "use DSL signature formatter (delegates to legacy; DSL mode only)")
	flag.BoolVar(&useDSLBlankLines, "dsl-blank-lines", true, "use DSL blank line formatter (DSL mode only)")
	flag.BoolVar(&allowDSLCallArgs, "dsl-allow-call-args", false, "allow DSL expression formatter to break long logical chains inside call arguments (DSL mode only, experimental)")
	flag.BoolVar(&autoDSLCallArgs, "dsl-auto-call-args", false, "allow DSL expression formatter to break long logical chains inside call arguments only for calls excluded from multiline formatting (DSL mode only, experimental)")
	flag.Parse()

	// Policy bundle is applied in the pipeline, but for CLI ergonomics we also
	// ensure "modern" implies the relevant DSL stages are enabled.
	if dslCallPolicy == "modern" {
		useDSLComments = true
		useDSLCalls = true
		useDSLMultiLine = true
		useDSLExpr = true
		useDSLSigs = true
		useDSLBlankLines = true
		if dslMultiLineStyle == "" || dslMultiLineStyle == "legacy" {
			dslMultiLineStyle = "packed-chain"
		}
	}

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: llformat [-w] [--wrap-inline-comments] [--col N] [--tab N] [--multiline-exclude FUNCS] [--legacy] [--trace-dsl] [--dsl-call-policy POLICY] [--dsl-comments] [--dsl-calls] [--dsl-multiline-calls] [--dsl-multiline-style STYLE] [--dsl-expr] [--dsl-sigs] [--dsl-blank-lines] [--dsl-allow-call-args] [--dsl-auto-call-args] <path>")
		os.Exit(2)
	}

	path := flag.Arg(0)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", path, err)
		os.Exit(1)
	}

	// Parse multiline exclude list
	var excludes []string
	if multilineExclude != "" {
		excludes = strings.Split(multilineExclude, ",")
		for i := range excludes {
			excludes[i] = strings.TrimSpace(excludes[i])
		}
	}

	policy := dslCallPolicy
	if useLegacy {
		policy = ""
	}

	// Use the unified formatting pipeline
	pipeline := formatter.NewPipeline(formatter.PipelineConfig{
		ColumnLimit:          colLimit,
		TabStop:              tabStop,
		MoveInlineAbove:      moveInline,
		Excludes:             excludes,
		UseDSLComments:       !useLegacy && useDSLComments,
		UseDSLLogCalls:       !useLegacy && useDSLCalls,
		UseDSLMultiLineCalls: !useLegacy && useDSLMultiLine,
		DSLMultiLineStyle:    dslMultiLineStyle,
		DSLCallPolicy:        policy,
		UseDSLExpr:           !useLegacy && useDSLExpr,
		UseDSLFuncSigs:       !useLegacy && useDSLSigs,
		UseDSLBlankLines:     !useLegacy && useDSLBlankLines,
		TraceDSL:             traceDSL && !useLegacy,
		AllowDSLCallArgs:     allowDSLCallArgs && !useLegacy,
		AutoDSLCallArgs:      autoDSLCallArgs && !useLegacy,
	})
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
