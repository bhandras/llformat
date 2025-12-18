package main

import (
	"flag"
	"fmt"
	formatstd "go/format"
	"os"
	"path/filepath"
	"strings"

	"github.com/lightninglabs/llformat/formatter"
)

func main() {
	var (
		path         string
		mode         string
		ruleProfile  string
		col          int
		tab          int
		trace        bool
		traceReasons bool
		ownership    bool
		stageOnly    string
		stageUpto    string
		printStages  bool
		printPlan    bool
		runGofmt     bool
		writeBack    bool
	)

	flag.StringVar(&path, "path", "", "input file to format")
	flag.StringVar(&mode, "mode", "", "pipeline mode: legacy|dsl-parity|dsl-modern|next (empty uses toggle defaults)")
	flag.StringVar(&ruleProfile, "rule-profile", "", "rule profile: parity|modern|next (optional)")
	flag.IntVar(&col, "col", 80, "column limit")
	flag.IntVar(&tab, "tab", 8, "tab width")
	flag.BoolVar(&trace, "trace", false, "enable DSL trace output to stderr (DSL stages only)")
	flag.BoolVar(&traceReasons, "trace-reasons", false, "include trace skip reasons (DSL stages only)")
	flag.BoolVar(&ownership, "ownership", false, "enable ownership registry (prevents stage fighting)")
	flag.StringVar(&stageOnly, "only", "", "run only a single stage by name (e.g. expressions)")
	flag.StringVar(&stageUpto, "upto", "", "run stages up to and including this stage (e.g. multiline-calls)")
	flag.BoolVar(&printStages, "list-stages", false, "print the stage list and exit")
	flag.BoolVar(&printPlan, "print-plan", false, "print the resolved pipeline plan and exit")
	flag.BoolVar(&runGofmt, "gofmt", true, "run gofmt once at the end")
	flag.BoolVar(&writeBack, "write", false, "write result back to input file (refuses testdata/**/output.go)")
	flag.Parse()

	if path == "" {
		// Allow a positional file argument as a convenience.
		if flag.NArg() != 1 {
			fmt.Fprintln(os.Stderr, "usage: repro_dsl --path <file.go> [flags]")
			os.Exit(2)
		}
		path = flag.Arg(0)
	}

	cfg := formatter.PipelineConfig{
		Mode:               mode,
		RuleProfile:        ruleProfile,
		ColumnLimit:        col,
		TabStop:            tab,
		TraceDSL:           trace,
		TraceDSLReasons:    traceReasons,
		UseOwnershipRegistry: ownership,
	}

	if printPlan {
		plan := formatter.ResolvePipelinePlan(cfg)
		fmt.Printf("%+v\n", plan)
		return
	}

	p := formatter.NewPipeline(cfg)
	orderedStages, err := formatter.StageOrder(p.Stages())
	if err != nil {
		fmt.Fprintf(os.Stderr, "stage order: %v\n", err)
		os.Exit(1)
	}

	if stageOnly != "" && stageUpto != "" {
		fmt.Fprintln(os.Stderr, "only one of --only or --upto may be set")
		os.Exit(2)
	}
	if stageOnly != "" && !hasStage(orderedStages, stageOnly) {
		fmt.Fprintf(os.Stderr, "unknown stage for --only: %q\n", stageOnly)
		os.Exit(2)
	}
	if stageUpto != "" && !hasStage(orderedStages, stageUpto) {
		fmt.Fprintf(os.Stderr, "unknown stage for --upto: %q\n", stageUpto)
		os.Exit(2)
	}

	if printStages {
		for _, s := range orderedStages {
			fmt.Println(s.Name)
		}
		return
	}

	in, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", path, err)
		os.Exit(1)
	}

	out := applyStages(in, orderedStages, cfg.UseOwnershipRegistry, stageOnly, stageUpto)
	if runGofmt {
		if formatted, err := formatstd.Source(out); err == nil {
			out = formatted
		}
	}

	if writeBack {
		if isGoldenOutputFile(path) {
			fmt.Fprintf(os.Stderr, "refusing to write golden fixture: %s\n", path)
			os.Exit(2)
		}
		if err := os.WriteFile(path, out, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
			os.Exit(1)
		}
		return
	}

	os.Stdout.Write(out)
}

func applyStages(src []byte, stages []formatter.Stage, ownership bool, onlyStage string, uptoStage string) []byte {
	out := src
	for _, stage := range stages {
		if onlyStage != "" && stage.Name != onlyStage {
			continue
		}

		if stage.Formatter != nil {
			if ownership {
				reg := formatter.BuildOwnershipRegistry(out, stages)
				if aware, ok := stage.Formatter.(formatter.OwnershipAware); ok {
					aware.SetOwnershipRegistry(reg)
				}
			} else if aware, ok := stage.Formatter.(formatter.OwnershipAware); ok {
				aware.SetOwnershipRegistry(nil)
			}

			out = stage.Formatter.FormatFile(out)
		}

		if uptoStage != "" && stage.Name == uptoStage {
			break
		}
	}
	return out
}

func hasStage(stages []formatter.Stage, name string) bool {
	for _, s := range stages {
		if s.Name == name {
			return true
		}
	}
	return false
}

func isGoldenOutputFile(path string) bool {
	clean := filepath.Clean(path)
	parts := strings.Split(clean, string(filepath.Separator))
	for i := 0; i < len(parts); i++ {
		if parts[i] != "testdata" {
			continue
		}
		if filepath.Base(clean) == "output.go" {
			return true
		}
		break
	}
	return false
}
