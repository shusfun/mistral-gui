package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	dir := flag.String("dir", "runtime-dist", "directory containing archives to sign")
	flag.Parse()

	key, err := base64.StdEncoding.DecodeString(os.Getenv("UPDATE_SIGNING_KEY"))
	if err != nil || len(key) != ed25519.PrivateKeySize {
		fmt.Fprintln(os.Stderr, "UPDATE_SIGNING_KEY must be a base64 Ed25519 private key")
		os.Exit(1)
	}
	public := base64.StdEncoding.EncodeToString(ed25519.PrivateKey(key).Public().(ed25519.PublicKey))
	if public != os.Getenv("UPDATE_PUBLIC_KEY") {
		fmt.Fprintln(os.Stderr, "UPDATE_PUBLIC_KEY does not match signing key")
		os.Exit(1)
	}
	entries, err := os.ReadDir(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".sig") {
			continue
		}
		p := filepath.Join(*dir, entry.Name())
		f, err := os.Open(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, h.Sum(nil))) + "\n"
		if err := os.WriteFile(p+".sig", []byte(sig), 0644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
