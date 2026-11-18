package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type SyncedRevision struct {
	Source      string `json:"source"`
	Translation string `json:"translation"`
	SourceLang  string `json:"sourceLang"`
	TargetLang  string `json:"targetLang"`
	Domain      string `json:"domain"`
	Revision    int    `json:"revision"`
	Time        int64  `json:"time"`
}

type SyncedPack struct {
	ID         string            `json:"id"`
	Version    int               `json:"version"`
	Priority   int               `json:"priority"`
	SourceLang string            `json:"sourceLang"`
	TargetLang string            `json:"targetLang"`
	Domain     string            `json:"domain"`
	Entries    map[string]string `json:"entries"`
}

type syncedDictionaryDisk struct {
	Revisions map[string]SyncedRevision `json:"revisions"`
	Packs     map[string]SyncedPack     `json:"packs"`
	SyncedAt  *time.Time                `json:"synced_at,omitempty"`
}

type DictionaryStore struct {
	mu   sync.RWMutex
	path string
	data syncedDictionaryDisk
}

func newDictionaryStore(dataDir string) *DictionaryStore {
	s := &DictionaryStore{
		path: filepath.Join(dataDir, "synced-dictionary.json"),
		data: syncedDictionaryDisk{
			Revisions: map[string]SyncedRevision{},
			Packs:     map[string]SyncedPack{},
		},
	}
	if b, err := os.ReadFile(s.path); err == nil && len(b) > 0 {
		var loaded syncedDictionaryDisk
		if json.Unmarshal(b, &loaded) == nil {
			if loaded.Revisions == nil {
				loaded.Revisions = map[string]SyncedRevision{}
			}
			if loaded.Packs == nil {
				loaded.Packs = map[string]SyncedPack{}
			}
			s.data = loaded
		}
	}
	return s
}

func normalizeDictSource(raw string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n"))
}

func normalizeDictLang(raw string) string {
	t := strings.TrimSpace(raw)
	if t == "" {
		return "auto"
	}
	if t == "*" || strings.EqualFold(t, "auto") {
		if t == "*" {
			return "*"
		}
		return "auto"
	}
	return NormalizeRuleLang(t)
}

func dictRevKey(source, srcLang, tgtLang string) string {
	return source + "\x00" + srcLang + "\x00" + tgtLang
}

func (s *DictionaryStore) persistLocked() error {
	b, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%s.tmp", s.path, randomToken()[:8])
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (s *DictionaryStore) Status() map[string]any {
	if s == nil {
		return map[string]any{"revisions": 0, "pack_entries": 0, "packs": 0}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	packEntries := 0
	for _, p := range s.data.Packs {
		packEntries += len(p.Entries)
	}
	out := map[string]any{
		"revisions":    len(s.data.Revisions),
		"pack_entries": packEntries,
		"packs":        len(s.data.Packs),
	}
	if s.data.SyncedAt != nil {
		out["synced_at"] = s.data.SyncedAt.Format(time.RFC3339)
	}
	return out
}

func (s *DictionaryStore) Clear() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = syncedDictionaryDisk{
		Revisions: map[string]SyncedRevision{},
		Packs:     map[string]SyncedPack{},
	}
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *DictionaryStore) Lookup(source, sourceLang, targetLang string) (string, string, bool) {
	if s == nil {
		return "", "", false
	}
	normSrc := normalizeDictSource(source)
	if normSrc == "" {
		return "", "", false
	}
	srcL := normalizeDictLang(sourceLang)
	tgtL := normalizeDictLang(targetLang)
	if tgtL == "auto" || tgtL == "*" {
		tgtL = "zh-CN"
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. User Revisions take highest priority
	candidates := []string{srcL, "auto", "*"}
	for _, cand := range candidates {
		if rev, ok := s.data.Revisions[dictRevKey(normSrc, cand, tgtL)]; ok && rev.Translation != "" {
			return rev.Translation, "revision", true
		}
	}
	if srcL == "auto" {
		var best *SyncedRevision
		for _, rev := range s.data.Revisions {
			if rev.Source == normSrc && (rev.TargetLang == tgtL || rev.TargetLang == "*") && rev.Translation != "" {
				if best == nil || rev.Time > best.Time {
					rCopy := rev
					best = &rCopy
				}
			}
		}
		if best != nil {
			return best.Translation, "revision", true
		}
	}

	// 2. Enabled Dictionary Packs ordered by Priority DESC, ID ASC
	if len(s.data.Packs) == 0 {
		return "", "", false
	}
	packs := make([]SyncedPack, 0, len(s.data.Packs))
	for _, p := range s.data.Packs {
		if p.TargetLang == tgtL || p.TargetLang == "*" {
			if srcL == "auto" || p.SourceLang == "auto" || p.SourceLang == "*" || p.SourceLang == srcL {
				packs = append(packs, p)
			}
		}
	}
	sort.Slice(packs, func(i, j int) bool {
		if packs[i].Priority != packs[j].Priority {
			return packs[i].Priority > packs[j].Priority
		}
		return packs[i].ID < packs[j].ID
	})
	for _, p := range packs {
		if tr, ok := p.Entries[normSrc]; ok && tr != "" {
			return tr, "dictionary", true
		}
	}
	return "", "", false
}

type syncPayload struct {
	RevisionsSnapshot bool             `json:"revisionsSnapshot"`
	Revisions         []SyncedRevision `json:"revisions"`
	Packs             []struct {
		ID         string `json:"id"`
		Version    int    `json:"version"`
		Priority   int    `json:"priority"`
		SourceLang string `json:"sourceLang"`
		TargetLang string `json:"targetLang"`
		Domain     string `json:"domain"`
		Unchanged  bool   `json:"unchanged"`
		Entries    []struct {
			Source      string `json:"source"`
			Translation string `json:"translation"`
		} `json:"entries"`
	} `json:"packs"`
}

func (a *App) dictionarySync(w http.ResponseWriter, r *http.Request) {
	root := a.root()
	var req syncPayload
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20))
	if err := dec.Decode(&req); err != nil {
		fail(w, 400, "invalid_json")
		return
	}
	if len(req.Revisions) > 20000 || len(req.Packs) > 64 {
		fail(w, 400, "sync_limit_exceeded")
		return
	}

	store := root.ensureDictionaryStore()
	store.mu.Lock()
	previous := store.data
	nextRevisions := make(map[string]SyncedRevision)
	if !req.RevisionsSnapshot {
		for k, v := range store.data.Revisions {
			nextRevisions[k] = v
		}
	}
	store.data.Revisions = nextRevisions
	for _, rev := range req.Revisions {
		src := normalizeDictSource(rev.Source)
		tr := strings.TrimSpace(rev.Translation)
		if src == "" || tr == "" || len(src) > 24000 || len(tr) > 48000 {
			continue
		}
		rev.Source = src
		rev.Translation = tr
		rev.SourceLang = normalizeDictLang(rev.SourceLang)
		rev.TargetLang = normalizeDictLang(rev.TargetLang)
		if rev.TargetLang == "auto" || rev.TargetLang == "*" {
			rev.TargetLang = "zh-CN"
		}
		key := dictRevKey(rev.Source, rev.SourceLang, rev.TargetLang)
		if prev, exists := store.data.Revisions[key]; !exists || rev.Time >= prev.Time {
			store.data.Revisions[key] = rev
		}
	}

	nextPacks := make(map[string]SyncedPack, len(req.Packs))
	for _, p := range req.Packs {
		id := strings.TrimSpace(p.ID)
		if id == "" || len(p.Entries) > 50000 {
			continue
		}
		if p.Unchanged {
			if existing, ok := store.data.Packs[id]; ok && existing.Version == p.Version {
				existing.Priority = p.Priority
				nextPacks[id] = existing
				continue
			}
		}
		entries := make(map[string]string, len(p.Entries))
		for _, e := range p.Entries {
			sKey := normalizeDictSource(e.Source)
			tVal := strings.TrimSpace(e.Translation)
			if sKey != "" && tVal != "" && len(sKey) <= 24000 && len(tVal) <= 48000 {
				entries[sKey] = tVal
			}
		}
		nextPacks[id] = SyncedPack{
			ID:         id,
			Version:    p.Version,
			Priority:   p.Priority,
			SourceLang: normalizeDictLang(p.SourceLang),
			TargetLang: normalizeDictLang(p.TargetLang),
			Domain:     p.Domain,
			Entries:    entries,
		}
	}
	store.data.Packs = nextPacks
	now := time.Now().UTC()
	store.data.SyncedAt = &now
	err := store.persistLocked()
	if err != nil {
		store.data = previous
	}
	store.mu.Unlock()
	if err != nil {
		fail(w, 500, "settings_write_failed")
		return
	}
	writeJSON(w, 200, store.Status())
}

func (a *App) dictionaryClear(w http.ResponseWriter, r *http.Request) {
	store := a.root().ensureDictionaryStore()
	if err := store.Clear(); err != nil {
		fail(w, 500, "settings_write_failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) dictionaryStatus(w http.ResponseWriter, r *http.Request) {
	store := a.root().ensureDictionaryStore()
	writeJSON(w, 200, store.Status())
}

func (a *App) ensureDictionaryStore() *DictionaryStore {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.dictionary == nil {
		a.dictionary = newDictionaryStore(a.dataDir)
	}
	return a.dictionary
}
