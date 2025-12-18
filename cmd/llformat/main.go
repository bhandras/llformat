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
		write                  bool
		colLimit               int
		tabStop                int
		moveInline             bool
		multilineExclude       string
		useLegacy              bool
		legacyHardening        bool
		mode                   string
		traceDSL               bool
		traceDSLReasons        bool
		useDSLComments         bool
		useDSLCalls            bool
		useDSLMultiLine        bool
		dslMultiLineStyle      string
		dslCallPolicy          string
		useDSLExpr             bool
		dslExprLogicalStyle    string
		dslExprArithmeticStyle string
		dslExprCaseClauseStyle string
		dslExprSelectorStyle   string
		useDSLSigs             bool
		useDSLSigsNative       bool
		dslSigsStyle           string
		useDSLBlankLines       bool
		useDSLBlankLinesNative bool
		allowDSLCallArgs       bool
		autoDSLCallArgs        bool
	)

	flag.BoolVar(&write, "w", false, "write result to (source) file instead of stdout")
	flag.BoolVar(&write, "write", false, "write result to (source) file instead of stdout")
	flag.IntVar(&colLimit, "col", 80, "column limit for formatting")
	flag.IntVar(&tabStop, "tab", 8, "tab stop width for column calculations")
	flag.BoolVar(&moveInline, "wrap-inline-comments", false, "when formatting comments, hoist trailing inline comments above for wrapping")
	flag.StringVar(&multilineExclude, "multiline-exclude", "", "comma-separated list of function names to exclude from multiline formatting")
	flag.BoolVar(&useLegacy, "legacy", false, "use legacy multi-stage formatter instead of DSL")
	flag.BoolVar(&legacyHardening, "legacy-hardening", false, "enable parse-safe + AST-guided selection in legacy stages (legacy mode only, experimental)")
	flag.StringVar(&mode, "mode", "", "pipeline mode: legacy|dsl-parity|dsl-modern|next (overrides individual DSL toggles when set)")
	flag.BoolVar(&traceDSL, "trace-dsl", false, "print DSL rule application trace to stderr (DSL mode only)")
	flag.BoolVar(&traceDSLReasons, "trace-dsl-reasons", false, "include \"why fired/didn't fire\" reasons in DSL trace output (DSL mode only)")
	flag.BoolVar(&useDSLComments, "dsl-comments", true, "use DSL comment formatter (delegates to legacy; DSL mode only)")
	flag.BoolVar(&useDSLCalls, "dsl-calls", true, "use DSL log/printf call formatter (DSL mode only)")
	flag.BoolVar(&useDSLMultiLine, "dsl-multiline-calls", true, "use DSL multiline call formatter (DSL mode only)")
	flag.StringVar(&dslMultiLineStyle, "dsl-multiline-style", "legacy", "DSL multiline call style: legacy|packed|packed-chain|packed-chain-layout|layout-args|layout-all (DSL mode only)")
	flag.StringVar(&dslCallPolicy, "dsl-call-policy", "legacy", "DSL call policy bundle: legacy|modern (DSL mode only)")
	flag.BoolVar(&useDSLExpr, "dsl-expr", true, "use DSL expression formatter (DSL mode only)")
	flag.StringVar(&dslExprLogicalStyle, "dsl-expr-logical-style", "", "DSL expression logical chain style: legacy|layout (DSL mode only, experimental)")
	flag.StringVar(&dslExprArithmeticStyle, "dsl-expr-arithmetic-style", "", "DSL expression arithmetic chain style: legacy|layout (DSL mode only, experimental)")
	flag.StringVar(&dslExprCaseClauseStyle, "dsl-expr-case-style", "", "DSL expression case clause style: legacy|layout (DSL mode only, experimental)")
	flag.StringVar(&dslExprSelectorStyle, "dsl-expr-selector-style", "", "DSL expression selector chain style: legacy|layout (DSL mode only, experimental)")
	flag.BoolVar(&useDSLSigs, "dsl-sigs", true, "use DSL signature formatter (delegates to legacy; DSL mode only)")
	flag.BoolVar(&useDSLSigsNative, "dsl-sigs-native", false, "use native DSL signature rules (fallback to legacy; DSL mode only, experimental)")
	flag.StringVar(&dslSigsStyle, "dsl-sigs-style", "legacy", "DSL signature style: legacy|dsl (DSL mode only, experimental)")
	flag.BoolVar(&useDSLBlankLines, "dsl-blank-lines", true, "use DSL blank line formatter (DSL mode only)")
	flag.BoolVar(&useDSLBlankLinesNative, "dsl-blank-lines-native", false, "use native DSL blank line rules (fallback to legacy; DSL mode only, experimental)")
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
		if dslExprLogicalStyle == "" {
			dslExprLogicalStyle = "layout"
		}
		if dslExprArithmeticStyle == "" {
			dslExprArithmeticStyle = "layout"
		}
		if dslExprCaseClauseStyle == "" {
			dslExprCaseClauseStyle = "layout"
		}
		if dslExprSelectorStyle == "" {
			dslExprSelectorStyle = "layout"
		}
		useDSLSigs = true
		useDSLSigsNative = true
		if dslSigsStyle == "" || dslSigsStyle == "legacy" {
			dslSigsStyle = "legacy"
		}
		useDSLBlankLines = true
		autoDSLCallArgs = true
		if dslMultiLineStyle == "" || dslMultiLineStyle == "legacy" || dslMultiLineStyle == "packed-chain" {
			dslMultiLineStyle = "packed-chain-layout"
		}
	}

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: llformat [-w] [--wrap-inline-comments] [--col N] [--tab N] [--multiline-exclude FUNCS] [--mode MODE] [--legacy] [--legacy-hardening] [--trace-dsl] [--trace-dsl-reasons] [--dsl-call-policy POLICY] [--dsl-comments] [--dsl-calls] [--dsl-multiline-calls] [--dsl-multiline-style STYLE] [--dsl-expr] [--dsl-expr-logical-style STYLE] [--dsl-expr-arithmetic-style STYLE] [--dsl-expr-case-style STYLE] [--dsl-expr-selector-style STYLE] [--dsl-sigs] [--dsl-sigs-native] [--dsl-sigs-style STYLE] [--dsl-blank-lines] [--dsl-blank-lines-native] [--dsl-allow-call-args] [--dsl-auto-call-args] <path>")
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
	if mode != "" && mode == "legacy" {
		useLegacy = true
		policy = ""
	}

	// Use the unified formatting pipeline
	pipeline := formatter.NewPipeline(formatter.PipelineConfig{
		Mode:                      mode,
		ColumnLimit:               colLimit,
		TabStop:                   tabStop,
		MoveInlineAbove:           moveInline,
		Excludes:                  excludes,
		LegacyHardening:           legacyHardening && useLegacy,
		UseDSLComments:            !useLegacy && useDSLComments,
		UseDSLLogCalls:            !useLegacy && useDSLCalls,
		UseDSLMultiLineCalls:      !useLegacy && useDSLMultiLine,
		DSLMultiLineStyle:         dslMultiLineStyle,
		DSLCallPolicy:             policy,
		UseDSLExpr:                !useLegacy && useDSLExpr,
		DSLExprLogicalStyle:       dslExprLogicalStyle,
		DSLExprArithmeticStyle:    dslExprArithmeticStyle,
		DSLExprCaseClauseStyle:    dslExprCaseClauseStyle,
		DSLExprSelectorChainStyle: dslExprSelectorStyle,
		UseDSLFuncSigs:            !useLegacy && useDSLSigs,
		UseDSLFuncSigsNative:      !useLegacy && useDSLSigsNative,
		DSLSigsStyle:              dslSigsStyle,
		UseDSLBlankLines:          !useLegacy && useDSLBlankLines,
		UseDSLBlankLinesNative:    !useLegacy && useDSLBlankLinesNative,
		TraceDSL:                  traceDSL && !useLegacy,
		TraceDSLReasons:           traceDSLReasons && !useLegacy,
		AllowDSLCallArgs:          allowDSLCallArgs && !useLegacy,
		AutoDSLCallArgs:           autoDSLCallArgs && !useLegacy,
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
