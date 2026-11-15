package api

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/catch/catch/backend/internal/httpx"
	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/store"
	gosourcemap "github.com/go-sourcemap/sourcemap"
)

const (
	maxSourceMapBytes          = 20 << 20
	maxCompressedSourceMapSize = 12 << 20
)

func (a *API) listSourceMaps(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	projectID, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	if _, err := a.store.ProjectByID(r.Context(), t.ID, projectID); err != nil {
		if store.IsNoDocuments(err) {
			httpx.Error(w, http.StatusNotFound, "project_not_found", "项目不存在")
			return
		}
		a.logError(r, "load project failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载项目")
		return
	}
	release := strings.TrimSpace(r.URL.Query().Get("release"))
	maps, err := a.store.ListSourceMaps(r.Context(), t.ID, projectID, release)
	if err != nil {
		a.logError(r, "list source maps failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载 Source Map")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"sourceMaps": maps})
}

func (a *API) getSourceFile(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	projectID, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	if _, err := a.store.ProjectByID(r.Context(), t.ID, projectID); err != nil {
		if store.IsNoDocuments(err) {
			httpx.Error(w, http.StatusNotFound, "project_not_found", "项目不存在")
			return
		}
		a.logError(r, "load project failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载项目")
		return
	}
	release := strings.TrimSpace(r.URL.Query().Get("release"))
	sourcePath := strings.TrimSpace(r.URL.Query().Get("path"))
	if release == "" || sourcePath == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_source", "需要 release 和 path 参数")
		return
	}
	line, _ := strconv.Atoi(r.URL.Query().Get("line"))
	content, resolved, ok, err := a.sourcemaps.SourceFile(r.Context(), t.ID, projectID, release, sourcePath)
	if err != nil {
		a.logError(r, "load source file failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法读取源码")
		return
	}
	if !ok {
		httpx.Error(w, http.StatusNotFound, "source_content_unavailable", "Source Map 未内嵌该文件源码，请打开仓库查看")
		return
	}
	const maxSourceContent = 4 << 20
	truncated := false
	if len(content) > maxSourceContent {
		content = content[:maxSourceContent] + "\n… (truncated)"
		truncated = true
	}
	language := strings.TrimPrefix(filepath.Ext(resolved), ".")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"path": resolved, "content": content, "line": line,
		"language": language, "release": release, "truncated": truncated,
	})
}

func (a *API) uploadSourceMap(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	projectID, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	project, err := a.store.ProjectByID(r.Context(), t.ID, projectID)
	if err != nil {
		if store.IsNoDocuments(err) {
			httpx.Error(w, http.StatusNotFound, "project_not_found", "项目不存在")
			return
		}
		a.logError(r, "load project failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载项目")
		return
	}

	release, file, data, err := readSourceMapUpload(w, r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_sourcemap_upload", err.Error())
		return
	}
	release = strings.TrimSpace(release)
	if release == "" || len(release) > 200 {
		httpx.Error(w, http.StatusBadRequest, "invalid_release", "请提供 1-200 字符的 release")
		return
	}
	file = normalizeBundleFile(file)
	if file == "" || len(file) > 300 {
		httpx.Error(w, http.StatusBadRequest, "invalid_bundle_file", "请提供有效的 bundle 文件名，例如 assets/index-abc.js")
		return
	}
	if len(data) == 0 {
		httpx.Error(w, http.StatusBadRequest, "empty_sourcemap", "Source Map 内容不能为空")
		return
	}

	var meta struct {
		Version  int               `json:"version"`
		Sources  []string          `json:"sources"`
		Mappings string            `json:"mappings"`
		Sections []json.RawMessage `json:"sections"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_sourcemap", "Source Map 不是合法 JSON")
		return
	}
	if meta.Version != 3 || (meta.Mappings == "" && len(meta.Sections) == 0) || (len(meta.Sources) == 0 && len(meta.Sections) == 0) {
		httpx.Error(w, http.StatusBadRequest, "invalid_sourcemap", "仅支持包含 sources/mappings 的 Source Map v3")
		return
	}
	if _, err := gosourcemap.Parse(file, data); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_sourcemap", "Source Map 解析失败: "+err.Error())
		return
	}

	var compressed bytes.Buffer
	zw, err := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法压缩 Source Map")
		return
	}
	if _, err := zw.Write(data); err != nil {
		_ = zw.Close()
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法压缩 Source Map")
		return
	}
	if err := zw.Close(); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法压缩 Source Map")
		return
	}
	if compressed.Len() > maxCompressedSourceMapSize {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "sourcemap_too_large", "Source Map 压缩后超过 12MB")
		return
	}

	sm, err := a.store.UpsertSourceMap(r.Context(), model.SourceMap{
		TenantID: t.ID, ProjectID: project.ID,
		Release: release, File: file,
		Size: int64(len(data)), SourceCount: len(meta.Sources),
		Content: compressed.Bytes(),
	})
	if err != nil {
		a.logError(r, "save source map failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法保存 Source Map")
		return
	}
	a.sourcemaps.Invalidate(t.ID, project.ID, release)
	a.logger.Slog.InfoContext(r.Context(), "source map uploaded",
		"projectId", project.ID.Hex(), "release", release, "file", file,
		"size", len(data), "sourceCount", len(meta.Sources), "actor", currentUser(r).Email)
	httpx.JSON(w, http.StatusCreated, map[string]any{"sourceMap": sm})
}

func (a *API) deleteSourceMap(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	projectID, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	mapID, err := pathID(r, "mapId")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	sm, err := a.store.DeleteSourceMap(r.Context(), t.ID, projectID, mapID)
	if err != nil {
		if store.IsNoDocuments(err) {
			httpx.Error(w, http.StatusNotFound, "sourcemap_not_found", "Source Map 不存在")
			return
		}
		a.logError(r, "delete source map failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法删除 Source Map")
		return
	}
	a.sourcemaps.Invalidate(t.ID, projectID, sm.Release)
	a.logger.Slog.InfoContext(r.Context(), "source map deleted",
		"projectId", projectID.Hex(), "release", sm.Release, "file", sm.File, "actor", currentUser(r).Email)
	w.WriteHeader(http.StatusNoContent)
}

func readSourceMapUpload(w http.ResponseWriter, r *http.Request) (release, file string, data []byte, err error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSourceMapBytes)
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			return "", "", nil, errors.New("无法解析上传表单")
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		release = r.FormValue("release")
		file = r.FormValue("file")
		f, hdr, ferr := r.FormFile("map")
		if ferr != nil {
			return "", "", nil, errors.New("请上传 .map 文件（字段名 map）")
		}
		defer f.Close()
		data, err = io.ReadAll(io.LimitReader(f, maxSourceMapBytes+1))
		if err != nil {
			return "", "", nil, errors.New("无法读取 Source Map 文件")
		}
		if len(data) > maxSourceMapBytes {
			return "", "", nil, errors.New("Source Map 超过 20MB 限制")
		}
		if strings.TrimSpace(file) == "" {
			file = strings.TrimSuffix(hdr.Filename, ".map")
		}
		return release, file, data, nil
	}

	release = firstNonEmpty(r.Header.Get("X-Release"), r.URL.Query().Get("release"))
	file = firstNonEmpty(r.Header.Get("X-File"), r.URL.Query().Get("file"), r.URL.Query().Get("bundle"))
	data, err = io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return "", "", nil, errors.New("Source Map 超过 20MB 限制")
		}
		return "", "", nil, errors.New("无法读取 Source Map")
	}
	return release, file, data, nil
}

func normalizeBundleFile(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		raw = u.Path
	}
	raw = strings.ReplaceAll(raw, "\\", "/")
	raw = strings.Split(raw, "?")[0]
	raw = strings.Split(raw, "#")[0]
	raw = strings.TrimPrefix(raw, "./")
	raw = strings.TrimLeft(raw, "/")
	raw = strings.TrimSuffix(raw, ".map")
	return raw
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
