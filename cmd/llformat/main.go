package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/lightninglabs/llformat/internal/format"
)

func main() {
	var (
		write bool
	)

	flag.BoolVar(&write, "w", false, "write result to (source) file instead of stdout")
	flag.BoolVar(&write, "write", false, "write result to (source) file instead of stdout")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: llformat [-w] <path>")
		os.Exit(2)
	}

	path := flag.Arg(0)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", path, err)
		os.Exit(1)
	}

	out := format.FormatFile(data)

	if write {
		if err := ioutil.WriteFile(path, out, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
			os.Exit(1)
		}
		return
	}

	os.Stdout.Write(out)
}
