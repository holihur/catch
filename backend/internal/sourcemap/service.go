// Package sourcemap resolves generated JavaScript stack frames back to original
// source files using uploaded Source Map v3 files.
package sourcemap

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/store"
	"github.com/go-sourcemap/sourcemap"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const releaseCacheTTL = 30 * time.Second

var generatedLocationRE = regexp.MustCompile(`((?:https?://|file://)?[^\s()]+?\.(?:js|mjs|cjs|jsx|ts|tsx)):(\d+):(\d+)`)

type releaseEntry struct {
	maps      []model.SourceMap
	expiresAt time.Time
}

type consumerEntry struct {
	consumer  *sourcemap.Consumer
	sources   map[string]string
	updatedAt time.Time
}

// Service resolves frames and caches release lookups and parsed source maps.
type Service struct {
	store *store.Store

	mu        sync.Mutex
	releases  map[string]releaseEntry
	consumers sync.Map // mapID/updatedAt -> *consumerEntry
}

func New(st *store.Store) *Service {
	return &Service{store: st, releases: map[string]releaseEntry{}}
}

// Invalidate clears release metadata after an upload/delete. Parsed consumers
// are keyed by map id + updatedAt, so updated maps are picked up automatically.
func (s *Service) Invalidate(tenantID, projectID primitive.ObjectID, release string) {
	prefix := tenantID.Hex() + ":" + projectID.Hex() + ":"
	s.mu.Lock()
	defer s.mu.Unlock()
	for key := range s.releases {
		if strings.HasPrefix(key, prefix) && (release == "" || key == prefix+release) {
			delete(s.releases, key)
		}
	}
}

// ResolveStack rewrites generated file:line:column positions to original source
// positions. It returns changed=false when no map matched any frame.
func (s *Service) ResolveStack(ctx context.Context, tenantID, projectID primitive.ObjectID, release, stack string) (string, bool, error) {
	if strings.TrimSpace(stack) == "" {
		return stack, false, nil
	}
	maps, err := s.mapsForRelease(ctx, tenantID, projectID, release)
	if err != nil {
		return stack, false, err
	}
	if len(maps) == 0 {
		return stack, false, nil
	}

	lines := strings.Split(stack, "\n")
	changed := false
	for i, line := range lines {
		match := generatedLocationRE.FindStringSubmatchIndex(line)
		if match == nil {
			continue
		}
		rawFile := line[match[2]:match[3]]
		genFile := normalizeGeneratedPath(rawFile)
		if genFile == "" {
			continue
		}
		sm, ok := matchSourceMap(maps, genFile)
		if !ok {
			continue
		}
		entry, err := s.consumerFor(ctx, sm)
		if err != nil {
			return stack, changed, err
		}
		lineNo := atoi(line[match[4]:match[5]])
		column := atoi(line[match[6]:match[7]])
		if lineNo <= 0 || column <= 0 {
			continue
		}
		src, _, srcLine, srcColumn, ok := entry.consumer.Source(lineNo, column)
		if !ok && column > 1 {
			src, _, srcLine, srcColumn, ok = entry.consumer.Source(lineNo, column-1)
		}
		if !ok || srcLine <= 0 {
			continue
		}
		src = cleanSourcePath(src)
		if src == "" || strings.Contains(src, "node_modules/") {
			continue
		}
		replacement := fmt.Sprintf("%s:%d:%d", src, srcLine, srcColumn+1)
		lines[i] = line[:match[0]] + replacement + line[match[1]:]
		changed = true
	}
	if !changed {
		return stack, false, nil
	}
	return strings.Join(lines, "\n"), true, nil
}

func (s *Service) mapsForRelease(ctx context.Context, tenantID, projectID primitive.ObjectID, release string) ([]model.SourceMap, error) {
	key := tenantID.Hex() + ":" + projectID.Hex() + ":" + release
	now := time.Now()
	s.mu.Lock()
	entry, ok := s.releases[key]
	s.mu.Unlock()
	if ok && now.Before(entry.expiresAt) {
		return entry.maps, nil
	}
	maps, err := s.store.SourceMapsForRelease(ctx, tenantID, projectID, release)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.releases[key] = releaseEntry{maps: maps, expiresAt: now.Add(releaseCacheTTL)}
	s.mu.Unlock()
	return maps, nil
}

func (s *Service) consumerFor(ctx context.Context, sm model.SourceMap) (*consumerEntry, error) {
	key := sm.ID.Hex() + ":" + sm.UpdatedAt.UTC().Format(time.RFC3339Nano)
	if v, ok := s.consumers.Load(key); ok {
		return v.(*consumerEntry), nil
	}
	full, err := s.store.SourceMapContent(ctx, sm.TenantID, sm.ProjectID, sm.ID)
	if err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(full.Content))
	if err != nil {
		return nil, fmt.Errorf("source map gzip: %w", err)
	}
	raw, err := io.ReadAll(zr)
	_ = zr.Close()
	if err != nil {
		return nil, fmt.Errorf("source map decompress: %w", err)
	}
	consumer, err := sourcemap.Parse(sm.File, raw)
	if err != nil {
		return nil, fmt.Errorf("source map parse: %w", err)
	}
	entry := &consumerEntry{
		consumer:  consumer,
		sources:   extractSources(raw),
		updatedAt: sm.UpdatedAt,
	}
	s.consumers.Store(key, entry)
	return entry, nil
}

// SourceFile returns embedded sourcesContent for an original source path.
func (s *Service) SourceFile(ctx context.Context, tenantID, projectID primitive.ObjectID, release, path string) (content, resolvedPath string, ok bool, err error) {
	requested := cleanSourcePath(path)
	if requested == "" {
		return "", "", false, nil
	}
	maps, err := s.mapsForRelease(ctx, tenantID, projectID, release)
	if err != nil {
		return "", "", false, err
	}
	for _, sm := range maps {
		entry, err := s.consumerFor(ctx, sm)
		if err != nil {
			continue
		}
		for source, body := range entry.sources {
			if source == requested || strings.HasSuffix(source, "/"+requested) || strings.HasSuffix(requested, "/"+source) {
				return body, source, true, nil
			}
		}
	}
	return "", "", false, nil
}

type rawSourceMap struct {
	Sources        []string `json:"sources"`
	SourcesContent []string `json:"sourcesContent"`
	SourceRoot     string   `json:"sourceRoot"`
	Sections       []struct {
		Map json.RawMessage `json:"map"`
	} `json:"sections"`
}

func extractSources(raw []byte) map[string]string {
	out := map[string]string{}
	extractSourcesInto(raw, "", out)
	return out
}

func extractSourcesInto(raw []byte, inheritedRoot string, out map[string]string) {
	var sm rawSourceMap
	if err := json.Unmarshal(raw, &sm); err != nil {
		return
	}
	root := sm.SourceRoot
	if root == "" {
		root = inheritedRoot
	}
	for i, source := range sm.Sources {
		if i >= len(sm.SourcesContent) || sm.SourcesContent[i] == "" {
			continue
		}
		content := sm.SourcesContent[i]
		candidates := []string{cleanSourcePath(source)}
		if root != "" {
			candidates = append(candidates, cleanSourcePath(root+"/"+source))
		}
		for _, candidate := range candidates {
			if candidate != "" {
				out[candidate] = content
			}
		}
	}
	for _, section := range sm.Sections {
		if len(section.Map) > 0 {
			extractSourcesInto(section.Map, root, out)
		}
	}
}

func matchSourceMap(maps []model.SourceMap, generated string) (model.SourceMap, bool) {
	base := pathBase(generated)
	// Exact path first, then a unique basename match so a map uploaded as
	// "index-abc.js" works with "assets/index-abc.js" in the stack.
	for _, sm := range maps {
		if normalizeGeneratedPath(sm.File) == generated {
			return sm, true
		}
	}
	var match model.SourceMap
	count := 0
	for _, sm := range maps {
		if pathBase(normalizeGeneratedPath(sm.File)) == base {
			match = sm
			count++
		}
	}
	if count == 1 {
		return match, true
	}
	return model.SourceMap{}, false
}

func normalizeGeneratedPath(raw string) string {
	p := strings.TrimSpace(raw)
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.Split(p, "?")[0]
	p = strings.Split(p, "#")[0]
	if i := strings.Index(p, "://"); i >= 0 {
		rest := p[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			p = rest[j:]
		} else {
			p = ""
		}
	}
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimLeft(p, "/")
	return p
}

func cleanSourcePath(raw string) string {
	p := strings.TrimSpace(raw)
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.Split(p, "?")[0]
	p = strings.Split(p, "#")[0]
	if i := strings.Index(p, "://"); i >= 0 {
		rest := p[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			p = rest[j:]
		} else {
			p = ""
		}
	}
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimLeft(p, "/")
	if len(p) >= 3 && p[1] == ':' && p[2] == '/' {
		p = p[3:]
	}
	parts := strings.Split(p, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			continue
		}
		out = append(out, part)
	}
	return strings.Join(out, "/")
}

func pathBase(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
