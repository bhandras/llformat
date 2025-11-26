package main

import (
    "flag"
    "fmt"
    "os"
    "path/filepath"
    "github.com/lightninglabs/llformat/formatter"
)

func main() {
    var (
        path       string
        col        int
        tab        int
    )
    flag.StringVar(&path, "path", filepath.Join("testdata", "multiline", "input.go"), "input file to format")
    flag.IntVar(&col, "col", 80, "column limit")
    flag.IntVar(&tab, "tab", 8, "tab width")
    flag.Parse()

    in, err := os.ReadFile(path)
    if err != nil {
        panic(err)
    }
    f := formatter.NewCompactCallFormatter(formatter.Config{
        ColumnLimit:        col,
        TabStop:            tab,
        FallbackNonTargets: true,
    })
    out := f.FormatFile(in)
    fmt.Print(string(out))
}
