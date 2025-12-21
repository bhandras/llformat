package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lightninglabs/llformat/formatter"
)

func main() {
	var (
		outDir = flag.String("out", ".next_goldens", "output directory for generated next goldens (not committed)")
		col    = flag.Int("col", 80, "column limit")
		tab    = flag.Int("tab", 8, "tab stop")
	)
	flag.Parse()

	cases, err := listTestdataCases("testdata")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(cases) == 0 {
		fmt.Fprintln(os.Stderr, "no testdata cases found under ./testdata")
		os.Exit(1)
	}

	p := formatter.NewPipeline(formatter.PipelineConfig{
		ColumnLimit: *col,
		TabStop:     *tab,
	})

	for _, dirName := range cases {
		inPath := filepath.Join("testdata", dirName, "input.go")
		in, err := os.ReadFile(inPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", inPath, err)
			os.Exit(1)
		}

		got := p.Format(in)

		// Mirror the testdata layout in the output directory so it's easy to
		// compare/copy.
		outPath := filepath.Join(*outDir, "testdata", dirName, "output_next.go")
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", filepath.Dir(outPath), err)
			os.Exit(1)
		}

		if err := os.WriteFile(outPath, got, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", outPath, err)
			os.Exit(1)
		}

		fmt.Println(outPath)
	}
}

func listTestdataCases(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var cases []string
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		name := ent.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		inPath := filepath.Join(root, name, "input.go")
		if _, err := os.Stat(inPath); err == nil {
			cases = append(cases, name)
		}
	}

	sort.Strings(cases)
	return cases, nil
}
