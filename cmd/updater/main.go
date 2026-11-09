package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/shusfun/mistral-gui/internal/update"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	path := flag.String("request", "", "update request")
	flag.Parse()
	b, e := os.ReadFile(*path)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	var r update.ApplyRequest
	if json.Unmarshal(b, &r) != nil {
		os.Exit(1)
	}
	// Fixed sibling paths prevent a tampered request from selecting unrelated trees.
	if !filepath.IsAbs(r.Target) || r.Staged != r.Target+".next" || (!strings.HasSuffix(r.Target, "mistral-gui.exe") && !strings.HasSuffix(r.Target, "mistral-gui.app")) {
		os.Exit(1)
	}
	if e = update.Apply(r); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
