package app

import (
	"strings"
	"testing"
)

func TestStartupOutputBoundsAndRedactsWithoutCapturingRequests(t *testing.T) {
	a := &App{}
	a.config.Token = "private-token"
	p := &engineProcess{ready: make(chan struct{})}
	w := startupOutput{app: a, process: p}
	_, _ = w.Write([]byte(strings.Repeat("x", 9000) + "private-"))
	_, _ = w.Write([]byte("token"))
	if len(p.startupLog) > 8192 || strings.Contains(p.startupLog, a.config.Token) || !strings.HasSuffix(p.startupLog, "[redacted]") {
		t.Fatal("startup log was not bounded or redacted")
	}
	close(p.ready)
	p.startupLog = ""
	_, _ = w.Write([]byte("private inference output"))
	if p.startupLog != "" {
		t.Fatal("inference output retained after readiness")
	}
}
