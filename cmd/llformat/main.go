package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/lightninglabs/llformat/formatter"
)

func main() {
	var (
		write      bool
		colLimit   int
		tabStop    int
		moveInline bool
	)

	flag.BoolVar(&write, "w", false, "write result to (source) file instead of stdout")
	flag.BoolVar(&write, "write", false, "write result to (source) file instead of stdout")
	flag.IntVar(&colLimit, "col", 80, "column limit for formatting")
	flag.IntVar(&tabStop, "tab", 8, "tab stop width for column calculations")
	flag.BoolVar(&moveInline, "wrap-inline-comments", false, "when formatting comments, hoist trailing inline comments above for wrapping")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: llformat [-w] [--comments] [--wrap-inline-comments] [--col N] [--tab N] <path>")
		os.Exit(2)
	}

	path := flag.Arg(0)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", path, err)
		os.Exit(1)
	}

	out := data

	// Optionally format standalone comments first, then apply left-flow call formatting.
	cf := formatter.NewCommentFormatter(formatter.CommentConfig{
		ColumnLimit:     colLimit,
		TabStop:         tabStop,
		MoveInlineAbove: moveInline,
	})
	out = cf.FormatFile(out)

	lf := formatter.NewLeftFlowFormatter(formatter.Config{
		ColumnLimit: colLimit,
		TabStop:     tabStop,
	})
	out = lf.FormatFile(out)

	if write {
		if err := ioutil.WriteFile(path, out, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
			os.Exit(1)
		}
		return
	}

	os.Stdout.Write(out)
}
