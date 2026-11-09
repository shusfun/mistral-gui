package catalog

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type rewriteTransport struct {
	base      *url.URL
	transport http.RoundTripper
}

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c := req.Clone(req.Context())
	u := *req.URL
	u.Scheme = r.base.Scheme
	u.Host = r.base.Host
	c.URL = &u
	return r.transport.RoundTrip(c)
}
func TestRepositoryPaginationAndCompleteGGUFShards(t *testing.T) {
	revision := strings.Repeat("a", 40)
	digest := strings.Repeat("b", 64)
	pages := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/tree/") {
			fmt.Fprintf(w, `{"sha":%q,"config":{"model_type":"qwen3"}}`, revision)
			return
		}
		pages++
		part := 1
		if r.URL.Query().Get("cursor") == "next" {
			part = 2
		} else {
			w.Header().Set("Link", fmt.Sprintf(`<https://huggingface.co/api/models/Qwen/Qwen3-0.6B-GGUF/tree/%s?cursor=next>; rel="next"`, revision))
		}
		fmt.Fprintf(w, `[{"type":"file","path":"q4/model-%05d-of-00002.gguf","size":123,"lfs":{"oid":%q}}]`, part, digest)
	}))
	defer s.Close()
	base, _ := url.Parse(s.URL)
	c := Client{HTTP: &http.Client{Transport: rewriteTransport{base, http.DefaultTransport}}}
	assets, e := c.RepositoryModels(context.Background(), "Qwen/Qwen3-0.6B-GGUF")
	if e != nil || pages != 2 || len(assets) != 1 || len(assets[0].Files) != 2 || assets[0].Size != 246 {
		t.Fatalf("%+v pages=%d err=%v", assets, pages, e)
	}
	if !strings.Contains(assets[0].Files[0].URL, revision) {
		t.Fatal("download revision not pinned")
	}
}
func TestIncompleteGGUFShardsAreNotInstallable(t *testing.T) {
	assets := []Asset{{Format: "gguf", Files: []ModelFile{{Path: "model-00001-of-00002.gguf"}}}}
	if len(groupGGUFShards("owner/model", assets)) != 0 {
		t.Fatal("incomplete shard set accepted")
	}
}
