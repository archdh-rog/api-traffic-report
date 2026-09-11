// Command apitrafficreport reads a JSONL API access log and prints a single
// JSON traffic report to stdout. See README.md for the full specification.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

func main() {
	limit := flag.Int("limit", 5, "max requests allowed per client within --window before flagging a rate-limit violation")
	window := flag.Duration("window", 10*time.Second, "sliding window size used for rate-limit detection")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <path-to.jsonl>\n\nReads a JSONL API request log and prints a JSON traffic report to stdout.\nIf <path> is omitted or \"-\", reads from stdin.\n\nFlags:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	var in io.Reader
	if flag.NArg() == 0 || flag.Arg(0) == "-" {
		in = os.Stdin
	} else {
		f, err := os.Open(flag.Arg(0))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot open input file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		in = f
	}

	report := buildReport(in, *limit, *window)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot encode report: %v\n", err)
		os.Exit(1)
	}
}
