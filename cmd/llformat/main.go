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
		write            bool
		colLimit         int
		tabStop          int
		moveInline       bool
		multilineExclude string
		useLegacy        bool
		traceDSL         bool
		allowDSLCallArgs bool
		autoDSLCallArgs  bool
	)

	flag.BoolVar(&write, "w", false, "write result to (source) file instead of stdout")
	flag.BoolVar(&write, "write", false, "write result to (source) file instead of stdout")
	flag.IntVar(&colLimit, "col", 80, "column limit for formatting")
	flag.IntVar(&tabStop, "tab", 8, "tab stop width for column calculations")
	flag.BoolVar(&moveInline, "wrap-inline-comments", false, "when formatting comments, hoist trailing inline comments above for wrapping")
	flag.StringVar(&multilineExclude, "multiline-exclude", "", "comma-separated list of function names to exclude from multiline formatting")
	flag.BoolVar(&useLegacy, "legacy", false, "use legacy multi-stage formatter instead of DSL")
	flag.BoolVar(&traceDSL, "trace-dsl", false, "print DSL rule application trace to stderr (DSL mode only)")
	flag.BoolVar(&allowDSLCallArgs, "dsl-allow-call-args", false, "allow DSL expression formatter to break long logical chains inside call arguments (DSL mode only, experimental)")
	flag.BoolVar(&autoDSLCallArgs, "dsl-auto-call-args", false, "allow DSL expression formatter to break long logical chains inside call arguments only for calls excluded from multiline formatting (DSL mode only, experimental)")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: llformat [-w] [--wrap-inline-comments] [--col N] [--tab N] [--multiline-exclude FUNCS] [--legacy] [--trace-dsl] [--dsl-allow-call-args] [--dsl-auto-call-args] <path>")
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

	// Use the unified formatting pipeline
	pipeline := formatter.NewPipeline(formatter.PipelineConfig{
		ColumnLimit:      colLimit,
		TabStop:          tabStop,
		MoveInlineAbove:  moveInline,
		Excludes:         excludes,
		UseDSLExpr:       !useLegacy,
		TraceDSL:         traceDSL && !useLegacy,
		AllowDSLCallArgs: allowDSLCallArgs && !useLegacy,
		AutoDSLCallArgs:  autoDSLCallArgs && !useLegacy,
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
