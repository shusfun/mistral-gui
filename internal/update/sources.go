package update

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type observationKey struct{}
type observation struct {
	trace string
	phase func(string, string)
}

// WithTrace links metadata, source changes and downloads to the caller's operation.
func WithTrace(ctx context.Context, trace string, phase func(string, string)) context.Context {
	if len(trace) != 55 || !strings.HasPrefix(trace, "00-") {
		var root [16]byte
		var span [8]byte
		_, _ = rand.Read(root[:])
		_, _ = rand.Read(span[:])
		trace = "00-" + hex.EncodeToString(root[:]) + "-" + hex.EncodeToString(span[:]) + "-01"
	}
	return context.WithValue(ctx, observationKey{}, observation{trace, phase})
}
func Trace(ctx context.Context) string {
	o, _ := ctx.Value(observationKey{}).(observation)
	return o.trace
}
func Phase(ctx context.Context, phase, source string) {
	o, _ := ctx.Value(observationKey{}).(observation)
	if o.phase != nil {
		o.phase(phase, source)
	}
	trace := strings.Split(o.trace, "-")
	root := ""
	if len(trace) == 4 {
		root = trace[1]
	}
	slog.Info("update."+phase, "traceId", root, "source", source, "code.file.path", "internal/update/sources.go")
}

// URLs only uses explicitly configured mirrors, retaining the official URL first.
func URLs(raw string, mirrors []string) []string {
	urls := []string{raw}
	for _, m := range mirrors {
		u, err := url.Parse(m)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			continue
		}
		candidate := strings.TrimRight(m, "/") + "/" + raw
		found := false
		for _, previous := range urls {
			if previous == candidate {
				found = true
			}
		}
		if !found {
			urls = append(urls, candidate)
		}
	}
	return urls
}
func sourceHost(raw string) string {
	u, _ := url.Parse(raw)
	if u == nil {
		return "unknown"
	}
	return u.Host
}

type metadataStatusError int

func (e metadataStatusError) Error() string { return fmt.Sprintf("metadata_http_%d", int(e)) }
func transient(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var n net.Error
	if errors.As(err, &n) && n.Timeout() {
		return true
	}
	var status metadataStatusError
	return errors.As(err, &status) && (status == 408 || status == 429 || status >= 500)
}
func eachSource(ctx context.Context, raw string, mirrors []string, fetch func(string) error) error {
	var failures []error
	for i, candidate := range URLs(raw, mirrors) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if i > 0 {
			Phase(ctx, "switching_source", sourceHost(candidate))
		}
		for attempt := 0; attempt < 2; attempt++ {
			if attempt > 0 {
				Phase(ctx, "retrying", sourceHost(candidate))
			}
			start := time.Now()
			Phase(ctx, "checking", sourceHost(candidate))
			err := fetch(candidate)
			// Raw transport errors may contain proxy credentials. Log only a classified outcome.
			outcome := "success"
			if err != nil {
				outcome = "failed"
			}
			cause := ""
			if err != nil {
				cause = "invalid_metadata"
				for _, code := range []string{"update_signature_invalid", "manifest_expired", "manifest_invalid", "update_asset_invalid", "version_invalid", "update_key_unconfigured", "metadata_too_large"} {
					if err.Error() == code {
						cause = code
					}
				}
				if transient(err) {
					cause = "temporary_network_failure"
				}
				var status metadataStatusError
				if errors.As(err, &status) {
					cause = status.Error()
				}
			}
			slog.Info("update.source_result", "traceId", strings.Split(Trace(ctx)+"-", "-")[1], "traceparent", Trace(ctx), "source", sourceHost(candidate), "attempt", attempt+1, "outcome", outcome, "cause", cause, "durationMs", time.Since(start).Milliseconds(), "code.file.path", "internal/update/sources.go")
			if err == nil {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			failures = append(failures, err)
			if !transient(err) {
				break
			}
		}
	}
	// Preserve typed causes for callers/tests, without leaking URLs into user-facing messages.
	return &SourceError{causes: errors.Join(failures...)}
}

type SourceError struct{ causes error }

func (e *SourceError) Error() string { return "update_sources_failed" }
func (e *SourceError) Unwrap() error { return e.causes }

func FetchSignedSources(ctx context.Context, client *http.Client, raw, key string, mirrors []string) (Manifest, error) {
	var manifest Manifest
	err := eachSource(ctx, raw, mirrors, func(candidate string) error {
		var err error
		manifest, err = FetchSigned(ctx, client, candidate, key)
		return err
	})
	return manifest, err
}
func FetchVerifiedSources(ctx context.Context, client *http.Client, raw string, mirrors []string, limit int64, verify func([]byte) error) ([]byte, error) {
	var body []byte
	err := eachSource(ctx, raw, mirrors, func(candidate string) error {
		var err error
		body, err = get(ctx, client, candidate, limit)
		if err != nil {
			return err
		}
		Phase(ctx, "verifying", sourceHost(candidate))
		return verify(body)
	})
	return body, err
}
