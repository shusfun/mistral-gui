package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type ModelFile struct {
	Path    string `json:"path"`
	URL     string `json:"url"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256,omitempty"`
	GitSHA1 string `json:"git_sha1,omitempty"`
}
type Repository struct {
	ID         string `json:"id"`
	Family     string `json:"family"`
	Format     string `json:"format"`
	Compatible bool   `json:"compatible"`
}

var Repositories = []Repository{
	{ModelRepo, "HY-MT", "gguf", true},
	{"Qwen/Qwen3-0.6B-GGUF", "Qwen3", "gguf", true}, {"Qwen/Qwen3-1.7B-GGUF", "Qwen3", "gguf", true}, {"Qwen/Qwen3-4B-GGUF", "Qwen3", "gguf", true}, {"Qwen/Qwen3-8B-GGUF", "Qwen3", "gguf", true},
	{"Qwen/Qwen3-0.6B", "Qwen3", "safetensors", true}, {"Qwen/Qwen3-1.7B", "Qwen3", "safetensors", true}, {"Qwen/Qwen3-4B", "Qwen3", "safetensors", true}, {"Qwen/Qwen3-8B", "Qwen3", "safetensors", true},
	{"HuggingFaceTB/SmolLM3-3B", "SmolLM3", "safetensors", true}, {"unsloth/SmolLM3-3B-GGUF", "SmolLM3", "gguf", true},
}

func ValidRepository(s string) bool {
	return regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$`).MatchString(s)
}
func SafeModelPath(s string) bool {
	return s != "" && !strings.Contains(s, "\\") && !strings.Contains(s, ":") && !strings.HasPrefix(s, "/") && path.Clean(s) == s && s != ".." && !strings.HasPrefix(s, "../")
}
func ModelID(repo, variant string) string {
	sum := sha256.Sum256([]byte(repo + "\n" + variant))
	return "model-" + hex.EncodeToString(sum[:12])
}
func (c Client) RepositoryModels(ctx context.Context, repo string) ([]Asset, error) {
	if !ValidRepository(repo) {
		return nil, errors.New("invalid_repository")
	}
	var info struct {
		SHA    string `json:"sha"`
		Config struct {
			ModelType string `json:"model_type"`
		} `json:"config"`
	}
	if err := c.get(ctx, "https://huggingface.co/api/models/"+repo, &info); err != nil {
		return nil, err
	}
	if !regexp.MustCompile(`^[a-f0-9]{40,64}$`).MatchString(info.SHA) {
		return nil, errors.New("invalid_model_revision")
	}
	known := false
	for _, r := range Repositories {
		if r.ID == repo {
			known = true
		}
	}
	if !known {
		switch info.Config.ModelType {
		case "qwen3", "qwen2", "llama", "mistral", "gemma", "gemma2", "gemma3_text", "phi3", "phi", "hy_mt":
		default:
			return nil, errors.New("model_architecture_unverified")
		}
	}
	type treeFile struct {
		Type string `json:"type"`
		Path string `json:"path"`
		Size int64  `json:"size"`
		OID  string `json:"oid"`
		LFS  *struct {
			OID string `json:"oid"`
		} `json:"lfs"`
	}
	var files []treeFile
	next := "https://huggingface.co/api/models/" + repo + "/tree/" + info.SHA + "?recursive=true&limit=1000"
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	for page := 0; next != ""; page++ {
		if page >= 100 {
			return nil, errors.New("model_catalog_too_large")
		}
		req, e := http.NewRequestWithContext(ctx, "GET", next, nil)
		if e != nil {
			return nil, e
		}
		resp, e := client.Do(req)
		if e != nil {
			return nil, e
		}
		var entries []treeFile
		e = json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&entries)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("metadata_http_%d", resp.StatusCode)
		}
		if e != nil {
			return nil, e
		}
		files = append(files, entries...)
		next = ""
		for _, link := range strings.Split(resp.Header.Get("Link"), ",") {
			if strings.Contains(link, `rel="next"`) {
				start := strings.Index(link, "<")
				end := strings.Index(link, ">")
				if start >= 0 && end > start {
					candidate := link[start+1 : end]
					u, e := url.Parse(candidate)
					if e != nil || u.Scheme != "https" || u.Host != "huggingface.co" || !strings.HasPrefix(u.Path, "/api/models/"+repo+"/tree/"+info.SHA) {
						return nil, errors.New("invalid_catalog_pagination")
					}
					next = candidate
				}
			}
		}
	}
	var ggufs []Asset
	source := "official"
	if !known {
		source = "custom"
	} else if strings.HasPrefix(repo, "unsloth/") {
		source = "community"
	}
	var snapshot []ModelFile
	var total int64
	hasConfig, hasTokenizer, hasWeights := false, false, false
	for _, f := range files {
		if f.Type != "file" || !SafeModelPath(f.Path) {
			continue
		}
		file := ModelFile{Path: f.Path, Size: f.Size, GitSHA1: f.OID, URL: "https://huggingface.co/" + repo + "/resolve/" + info.SHA + "/" + f.Path + "?download=true"}
		if f.LFS != nil {
			file.SHA256 = f.LFS.OID
			file.GitSHA1 = ""
		}
		if strings.HasSuffix(f.Path, ".gguf") {
			if strings.Contains(strings.ToLower(path.Base(f.Path)), "mmproj") {
				continue
			}
			if len(file.SHA256) != 64 {
				continue
			}
			id := ModelID(repo, f.Path)
			if repo == ModelRepo && !strings.Contains(f.Path, "/") {
				id = f.Path
			}
			ggufs = append(ggufs, Asset{ID: id, Name: repo + " / " + f.Path, Kind: "model", Version: info.SHA, Backend: "any", Repository: repo, Format: "gguf", URL: file.URL, SHA256: file.SHA256, Size: file.Size, Source: source, Files: []ModelFile{file}})
			continue
		}
		base := path.Base(f.Path)
		weight := strings.HasSuffix(base, ".safetensors")
		support := base == "config.json" || strings.HasPrefix(base, "tokenizer") || strings.HasPrefix(base, "special_tokens") || base == "vocab.json" || base == "merges.txt" || base == "generation_config.json" || base == "added_tokens.json" || base == "chat_template.jinja" || base == "model.safetensors.index.json"
		if weight || support {
			if len(file.SHA256) != 64 && len(file.GitSHA1) != 40 {
				return nil, errors.New("model_file_digest_missing")
			}
			snapshot = append(snapshot, file)
			total += f.Size
			if base == "config.json" {
				hasConfig = true
			}
			if base == "tokenizer.json" || base == "tokenizer.model" {
				hasTokenizer = true
			}
			if weight {
				hasWeights = true
			}
		}
	}
	if hasConfig && hasTokenizer && hasWeights {
		b, _ := json.Marshal(snapshot)
		sum := sha256.Sum256(b)
		ggufs = append(ggufs, Asset{ID: ModelID(repo, "safetensors"), Name: repo + " / safetensors", Kind: "model", Version: info.SHA, Backend: "any", Repository: repo, Format: "safetensors", SHA256: hex.EncodeToString(sum[:]), Size: total, Source: source, Files: snapshot})
	}
	ggufs = groupGGUFShards(repo, ggufs)
	sort.Slice(ggufs, func(i, j int) bool { return ggufs[i].Name < ggufs[j].Name })
	return ggufs, nil
}

func groupGGUFShards(repo string, assets []Asset) []Asset {
	pattern := regexp.MustCompile(`^(.*)-([0-9]{5})-of-([0-9]{5})\.gguf$`)
	groups := map[string][]Asset{}
	var result []Asset
	for _, a := range assets {
		if a.Format != "gguf" || len(a.Files) != 1 {
			result = append(result, a)
			continue
		}
		m := pattern.FindStringSubmatch(a.Files[0].Path)
		if m == nil {
			result = append(result, a)
			continue
		}
		groups[m[1]+"-of-"+m[3]] = append(groups[m[1]+"-of-"+m[3]], a)
	}
	for _, parts := range groups {
		sort.Slice(parts, func(i, j int) bool { return parts[i].Files[0].Path < parts[j].Files[0].Path })
		m := pattern.FindStringSubmatch(parts[0].Files[0].Path)
		n, _ := strconv.Atoi(m[3])
		if n != len(parts) {
			continue
		}
		a := parts[0]
		a.Size = 0
		a.Files = nil
		valid := true
		for i, p := range parts {
			x := pattern.FindStringSubmatch(p.Files[0].Path)
			index, _ := strconv.Atoi(x[2])
			if index != i+1 {
				valid = false
				break
			}
			a.Files = append(a.Files, p.Files[0])
			a.Size += p.Size
		}
		if !valid {
			continue
		}
		a.ID = ModelID(repo, m[1]+".gguf")
		a.Name = repo + " / " + m[1] + ".gguf"
		b, _ := json.Marshal(a.Files)
		sum := sha256.Sum256(b)
		a.SHA256 = hex.EncodeToString(sum[:])
		result = append(result, a)
	}
	return result
}
