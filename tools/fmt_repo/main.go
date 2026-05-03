package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type multiStringFlag []string

func (m *multiStringFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiStringFlag) Set(v string) error {
	*m = append(*m, v)

	return nil
}

type runConfig struct {
	llformatPath       string
	write              bool
	colLimit           int
	tabStop            int
	moveInline         bool
	multilineExclude   string
	logCallsMinTailLen int
	fixpointIters      int
}

func main() {
	var (
		llformatPath       string
		write              bool
		root               string
		colLimit           int
		tabStop            int
		moveInline         bool
		multilineExclude   string
		logCallsMinTailLen int
		fixpointIters      int
		excludeDirs        multiStringFlag
	)

	flag.StringVar(
		&llformatPath, "llformat", "./bin/llformat",
		"path to llformat binary",
	)
	flag.BoolVar(
		&write, "write", false,
		"write changes in-place (default is check-only)",
	)
	flag.StringVar(&root, "root", ".", "repository root to walk")
	flag.IntVar(&colLimit, "col", 80, "column limit for formatting")
	flag.IntVar(
		&tabStop, "tab", 8, "tab stop width for column calculations",
	)
	flag.BoolVar(
		&moveInline, "wrap-inline-comments", false,
		"hoist trailing inline comments above for wrapping",
	)
	flag.StringVar(
		&multilineExclude, "multiline-exclude", "", "comma-separated"+
			" list of function names to exclude from multiline "+
			"formatting",
	)
	flag.IntVar(
		&logCallsMinTailLen, "logcalls-min-tail-len", 0, "minimum "+
			"tail length when splitting printf/logcall strings "+
			"(0 => default)",
	)
	flag.IntVar(
		&fixpointIters, "fixpoint-iters", 0,
		"repeat full pipeline until stable (0=auto; default 3)",
	)
	flag.Var(
		&excludeDirs, "exclude-dir",
		"directory name to skip during walk (repeatable)",
	)
	flag.Parse()

	excludeDirSet := buildExcludeDirSet(
		defaultExcludeDirs(excludeDirs),
	)
	if err := ensureLLFormatExists(llformatPath); err != nil {
		fmt.Fprintf(
			os.Stderr, "llformat binary not found at %q (run "+
				"`make build` first): %v\n", llformatPath, err,
		)
		os.Exit(2)
	}

	cfg := runConfig{
		llformatPath:       llformatPath,
		write:              write,
		colLimit:           colLimit,
		tabStop:            tabStop,
		moveInline:         moveInline,
		multilineExclude:   multilineExclude,
		logCallsMinTailLen: logCallsMinTailLen,
		fixpointIters:      fixpointIters,
	}

	goFiles, err := collectGoFiles(root, excludeDirSet)
	if err != nil {
		fmt.Fprintf(os.Stderr, "walk %s: %v\n", root, err)
		os.Exit(2)
	}

	sort.Strings(goFiles)

	var changed []string
	for _, path := range goFiles {
		// Don't ever touch golden fixtures or their inputs; they are
		// the spec.
		if shouldSkipPath(path) {
			continue
		}

		isChanged, err := formatGoFile(path, cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		if isChanged {
			changed = append(changed, path)
		}
	}

	if cfg.write {
		return
	}

	if len(changed) > 0 {
		fmt.Fprintf(
			os.Stderr, "llformat would reformat %d file(s):\n",
			len(changed),
		)
		for _, p := range changed {
			fmt.Fprintf(os.Stderr, "  %s\n", p)
		}
		os.Exit(1)
	}
}

func defaultExcludeDirs(excludeDirs []string) []string {
	if len(excludeDirs) > 0 {
		return excludeDirs
	}

	return []string{
		".git",
		"bin",
		"testdata",
		".next_goldens",
		".gocache",
		".gomodcache",
	}
}

func buildExcludeDirSet(excludeDirs []string) map[string]struct{} {
	excludeDirSet := make(map[string]struct{}, len(excludeDirs))
	for _, d := range excludeDirs {
		excludeDirSet[d] = struct{}{}
	}

	return excludeDirSet
}

func ensureLLFormatExists(path string) error {
	_, err := os.Stat(path)

	return err
}

func collectGoFiles(root string,
	excludeDirSet map[string]struct{}) ([]string, error) {

	var goFiles []string
	err := filepath.WalkDir(
		root,
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return skipIfExcludedDir(d, excludeDirSet)
			}
			if !isGoFile(path) {
				return nil
			}

			goFiles = append(goFiles, path)

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return goFiles, nil
}

func shouldSkipPath(path string) bool {
	return strings.HasPrefix(filepath.ToSlash(path), "testdata/")
}

func skipIfExcludedDir(
	d fs.DirEntry,
	excludeDirSet map[string]struct{},
) error {

	if _, ok := excludeDirSet[d.Name()]; ok {
		return fs.SkipDir
	}

	return nil
}

func isGoFile(path string) bool {
	return filepath.Ext(path) == ".go"
}

func formatGoFile(path string, cfg runConfig) (bool, error) {
	args := buildArgs(cfg, path)
	cmd := exec.Command(cfg.llformatPath, args...)
	cmd.Stderr = os.Stderr

	if cfg.write {
		if err := cmd.Run(); err != nil {
			return false, fmt.Errorf("llformat %s: %w", path, err)
		}

		return false, nil
	}

	orig, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}

	formatted, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("llformat %s: %w", path, err)
	}

	return !bytes.Equal(orig, formatted), nil
}

func buildArgs(cfg runConfig, path string) []string {
	args := []string{
		"--col", fmt.Sprint(cfg.colLimit),
		"--tab", fmt.Sprint(cfg.tabStop),
		"--logcalls-min-tail-len", fmt.Sprint(
			cfg.logCallsMinTailLen,
		),
		"--fixpoint-iters", fmt.Sprint(cfg.fixpointIters),
	}

	if cfg.moveInline {
		args = append(args, "--wrap-inline-comments")
	}
	if cfg.multilineExclude != "" {
		args = append(args, "--multiline-exclude", cfg.multilineExclude)
	}
	if cfg.write {
		args = append(args, "--write")
	}

	return append(args, path)
}
