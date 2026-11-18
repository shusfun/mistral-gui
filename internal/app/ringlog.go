package app

import (
	"strings"
	"sync"
)

const defaultRingLogCapacity = 64 << 10 // 64 KiB per deployment

// ringLogBuffer retains the most recent engine stdout/stderr bytes across startup and runtime,
// redacting sensitive tokens before storage (M-R3).
type ringLogBuffer struct {
	mu       sync.Mutex
	capacity int
	data     string
}

func newRingLogBuffer(capacity int) *ringLogBuffer {
	if capacity <= 0 {
		capacity = defaultRingLogCapacity
	}
	return &ringLogBuffer{capacity: capacity}
}

func (r *ringLogBuffer) Append(chunk string, secrets ...string) {
	if r == nil || chunk == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	combined := r.data + chunk
	for _, s := range secrets {
		if s != "" {
			combined = strings.ReplaceAll(combined, s, "[redacted]")
		}
	}
	if len(combined) > r.capacity {
		combined = combined[len(combined)-r.capacity:]
	}
	r.data = combined
}

func (r *ringLogBuffer) String() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.data
}

func (r *ringLogBuffer) Tail(maxBytes int) string {
	s := r.String()
	if maxBytes > 0 && len(s) > maxBytes {
		return s[len(s)-maxBytes:]
	}
	return strings.TrimSpace(s)
}

func (r *ringLogBuffer) Reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = ""
}
