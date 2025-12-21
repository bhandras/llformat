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
		&multilineExclude, "multiline-exclude", "", "comma-separated "+
			"list of function names to exclude from multiline "+
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

	if len(excludeDirs) == 0 {
		excludeDirs = []string{
			".git",
			"bin",
			"testdata",
			".next_goldens",
			".gocache",
			".gomodcache",
		}
	}
	excludeDirSet := make(map[string]struct{}, len(excludeDirs))
	for _, d := range excludeDirs {
		excludeDirSet[d] = struct{}{}
	}

	if _, err := os.Stat(llformatPath); err != nil {
		fmt.Fprintf(
			os.Stderr, "llformat binary not found at %q (run "+
				"`make build` first): %v\n", llformatPath, err,
		)
		os.Exit(2)
	}

	var goFiles []string
	if err := filepath.WalkDir(
		root,
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				if _, ok := excludeDirSet[d.Name()]; ok {
					return fs.SkipDir
				}

				return nil
			}

			if filepath.Ext(path) != ".go" {
				return nil
			}

			goFiles = append(goFiles, path)

			return nil
		},
	); err != nil {

		fmt.Fprintf(os.Stderr, "walk %s: %v\n", root, err)
		os.Exit(2)
	}

	sort.Strings(goFiles)

	var changed []string
	for _, path := range goFiles {
		// Don't ever touch golden fixtures or their inputs; they are
		// the spec.
		if strings.HasPrefix(filepath.ToSlash(path), "testdata/") {
			continue
		}

		args := []string{
			"--col", fmt.Sprint(colLimit),
			"--tab", fmt.Sprint(tabStop),
			"--logcalls-min-tail-len", fmt.Sprint(
				logCallsMinTailLen,
			),
			"--fixpoint-iters", fmt.Sprint(fixpointIters),
		}
		if moveInline {
			args = append(args, "--wrap-inline-comments")
		}
		if multilineExclude != "" {
			args = append(
				args, "--multiline-exclude", multilineExclude,
			)
		}
		if write {
			args = append(args, "--write")
		}
		args = append(args, path)

		cmd := exec.Command(llformatPath, args...)
		cmd.Stderr = os.Stderr

		if write {
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(
					os.Stderr, "llformat %s: %v\n", path,
					err,
				)
				os.Exit(1)
			}
			continue
		}

		orig, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", path, err)
			os.Exit(1)
		}

		formatted, err := cmd.Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "llformat %s: %v\n", path, err)
			os.Exit(1)
		}

		if !bytes.Equal(orig, formatted) {
			changed = append(changed, path)
		}
	}

	if write {
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
