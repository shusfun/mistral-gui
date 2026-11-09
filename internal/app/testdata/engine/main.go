package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	args := os.Args[1:]
	value := func(key string) string {
		for i, s := range args {
			if s == key && i+1 < len(args) {
				return args[i+1]
			}
		}
		return ""
	}
	if len(args) == 0 || args[0] != "serve" || value("--host") != "127.0.0.1" {
		os.Exit(2)
	}
	cache := value("--hf-cache")
	_ = os.MkdirAll(cache, 0700)
	f, _ := os.OpenFile(filepath.Join(cache, "starts"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if f != nil {
		_, _ = f.WriteString("start\n")
		_ = f.Close()
	}
	b, _ := os.ReadFile(value("-f"))
	if string(b) == "crash" {
		fmt.Fprintln(os.Stderr, "unsupported GGUF architecture: fixture")
		os.Exit(3)
	}
	if string(b) == "stdout-crash" {
		fmt.Fprintln(os.Stdout, "CUDA initialization failed: fixture")
		os.Exit(4)
	}
	if string(b) == "silent-crash" {
		os.Exit(5)
	}
	http.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"data":[{"id":"engine-model"}]}`) })
	http.HandleFunc("/ui", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "upstream UI fixture") })
	http.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if json.NewDecoder(r.Body).Decode(&p) != nil || p.Model != "engine-model" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || !strings.HasPrefix(r.Header.Get("traceparent"), "00-") {
			http.Error(w, "invalid proxy", 400)
			return
		}
		w.Header().Set("X-Fixture-Trace", r.Header.Get("traceparent"))
		if p.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"你好"},"finish_reason":"stop"}]}`)
	})
	if err := http.ListenAndServe("127.0.0.1:"+value("--port"), nil); err != nil {
		os.Exit(4)
	}
}
