package catalog

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOfficialPreferenceAndExactVersionSupplement(t *testing.T) {
	official := true
	var paths []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if strings.Contains(r.URL.Path, Upstream) && !official {
			fmt.Fprint(w, `{"tag_name":"v1","assets":[]}`)
			return
		}
		fmt.Fprintf(w, `{"tag_name":"v1","assets":[{"name":"mistralrs-cpu-x86_64-pc-windows-msvc.zip.sha256","size":80},{"name":"mistralrs-cpu-x86_64-pc-windows-msvc.zip","browser_download_url":"https://github.com/test/asset.zip","size":123,"digest":"sha256:%s"}]}`, strings.Repeat("a", 64))
	}))
	defer s.Close()
	c := Client{HTTP: s.Client(), API: s.URL}
	a, e := c.SelectEngine(context.Background(), "owner/assets", "v1", "windows", "amd64", "cpu")
	if e != nil || a.Source != "official" || len(paths) != 1 {
		t.Fatalf("%+v %v %v", a, e, paths)
	}
	official = false
	paths = nil
	a, e = c.SelectEngine(context.Background(), "owner/assets", "v1", "windows", "amd64", "cpu")
	if e != nil || a.Source != "supplement" || len(paths) != 2 || paths[1] != "/repos/owner/assets/releases/tags/v1" {
		t.Fatalf("%+v %v %v", a, e, paths)
	}
}
func TestMetadataFailureDoesNotSelectSupplement(t *testing.T) {
	count := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { count++; http.Error(w, "rate limited", 403) }))
	defer s.Close()
	_, e := (Client{HTTP: s.Client(), API: s.URL}).SelectEngine(context.Background(), "owner/assets", "", "windows", "amd64", "cpu")
	if e == nil || count != 1 {
		t.Fatal("metadata failure was hidden by fallback")
	}
}

func TestIndependentRuntimeReleasePreservesUpstreamVersion(t *testing.T) {
	var paths []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if strings.Contains(r.URL.Path, Upstream) {
			fmt.Fprint(w, `{"tag_name":"v0.9.5","assets":[]}`)
			return
		}
		fmt.Fprintf(w, `{"tag_name":"runtime-v0.9.5","assets":[{"name":"mistralrs-cuda-x86_64-pc-windows-msvc.zip","digest":"sha256:%s"}]}`, strings.Repeat("b", 64))
	}))
	defer s.Close()
	c := Client{HTTP: s.Client(), API: s.URL, SupplementTagPrefix: "runtime-"}
	a, err := c.SelectEngine(context.Background(), "owner/manager", "", "windows", "amd64", "cuda")
	if err != nil || a.Version != "v0.9.5" || a.Source != "supplement" || len(paths) != 3 || paths[2] != "/repos/owner/manager/releases/tags/runtime-v0.9.5" {
		t.Fatalf("%+v %v %v", a, err, paths)
	}
}
