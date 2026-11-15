package api

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/catch/catch/backend/internal/httpx"
	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/store"
	"go.mongodb.org/mongo-driver/bson"
)

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)

func maskKey(key string) string {
	if len(key) <= 8 {
		return "ck_••••"
	}
	return "ck_••••••••" + key[len(key)-4:]
}

func (a *API) projectDTO(p model.Project, reveal bool) model.Project {
	if !reveal {
		p.IngestKey = maskKey(p.IngestKey)
	}
	return p
}

func (a *API) listProjects(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	u := currentUser(r)
	projects, err := a.store.ListProjects(r.Context(), t.ID)
	if err != nil {
		a.logError(r, "list projects failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载项目")
		return
	}
	reveal := model.AtLeast(u.Role, model.RoleAdmin)
	out := make([]model.Project, 0, len(projects))
	for _, p := range projects {
		out = append(out, a.projectDTO(p, reveal))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"projects": out})
}

func (a *API) createProject(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	var body struct {
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		Platform string `json:"platform"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 16<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Platform = strings.TrimSpace(body.Platform)
	if body.Name == "" || len(body.Name) > 80 {
		httpx.Error(w, http.StatusBadRequest, "invalid_name", "项目名称长度需在 1-80 之间")
		return
	}
	slug := strings.ToLower(strings.TrimSpace(body.Slug))
	if slug == "" {
		slug = slugify(body.Name)
	}
	if !slugRe.MatchString(slug) {
		httpx.Error(w, http.StatusBadRequest, "invalid_slug", "项目标识只能包含小写字母、数字和连字符")
		return
	}
	count, err := a.store.CountProjects(r.Context(), t.ID)
	if err != nil {
		a.logError(r, "count projects failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法创建项目")
		return
	}
	if count >= 20 {
		httpx.Error(w, http.StatusBadRequest, "project_limit", "每个租户最多 20 个项目")
		return
	}
	p, err := a.store.CreateProject(r.Context(), t.ID, body.Name, slug, body.Platform)
	if err != nil {
		if store.IsDuplicate(err) {
			httpx.Error(w, http.StatusConflict, "slug_taken", "项目标识已存在")
			return
		}
		a.logError(r, "create project failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法创建项目")
		return
	}
	a.logger.Slog.InfoContext(r.Context(), "project created", "projectId", p.ID.Hex(), "name", p.Name, "actor", currentUser(r).Email)
	httpx.JSON(w, http.StatusCreated, a.projectDTO(p, true))
}

func (a *API) updateProject(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	var body struct {
		Name             *string `json:"name"`
		Platform         *string `json:"platform"`
		RepoProvider     *string `json:"repoProvider"`
		RepoURL          *string `json:"repoUrl"`
		RepoBranch       *string `json:"repoBranch"`
		RepoRoot         *string `json:"repoRoot"`
		RepoLinkTemplate *string `json:"repoLinkTemplate"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 16<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	set := bson.M{}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if name == "" || len(name) > 80 {
			httpx.Error(w, http.StatusBadRequest, "invalid_name", "项目名称长度需在 1-80 之间")
			return
		}
		set["name"] = name
	}
	if body.Platform != nil {
		set["platform"] = strings.TrimSpace(*body.Platform)
	}
	if body.RepoURL != nil {
		repoURL, provider, err := normalizeRepoURL(*body.RepoURL, stringValue(body.RepoProvider))
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid_repo_url", err.Error())
			return
		}
		set["repoUrl"] = repoURL
		if provider != "" {
			set["repoProvider"] = provider
		}
	}
	if body.RepoProvider != nil && body.RepoURL == nil {
		p := strings.ToLower(strings.TrimSpace(*body.RepoProvider))
		if p != "" && !validRepoProvider(p) {
			httpx.Error(w, http.StatusBadRequest, "invalid_repo_provider", "源码平台必须是 github、gitlab、bitbucket、gitea 或 custom")
			return
		}
		set["repoProvider"] = p
	}
	if body.RepoBranch != nil {
		branch := strings.TrimSpace(*body.RepoBranch)
		if len(branch) > 200 || strings.ContainsAny(branch, "\r\n") {
			httpx.Error(w, http.StatusBadRequest, "invalid_repo_branch", "分支名称无效")
			return
		}
		set["repoBranch"] = branch
	}
	if body.RepoRoot != nil {
		root := strings.Trim(strings.TrimSpace(*body.RepoRoot), "/")
		if len(root) > 200 || strings.Contains(root, "..") || strings.ContainsAny(root, "\r\n") {
			httpx.Error(w, http.StatusBadRequest, "invalid_repo_root", "源码根目录无效")
			return
		}
		set["repoRoot"] = root
	}
	if body.RepoLinkTemplate != nil {
		tpl := strings.TrimSpace(*body.RepoLinkTemplate)
		if len(tpl) > 1000 {
			httpx.Error(w, http.StatusBadRequest, "invalid_repo_template", "自定义链接模板过长")
			return
		}
		if tpl != "" {
			if !strings.HasPrefix(tpl, "https://") && !strings.HasPrefix(tpl, "http://") {
				httpx.Error(w, http.StatusBadRequest, "invalid_repo_template", "自定义链接模板必须以 http(s):// 开头")
				return
			}
			if !strings.Contains(tpl, "{path}") {
				httpx.Error(w, http.StatusBadRequest, "invalid_repo_template", "自定义链接模板必须包含 {path}")
				return
			}
			if strings.ContainsAny(tpl, "\r\n\t ") {
				httpx.Error(w, http.StatusBadRequest, "invalid_repo_template", "自定义链接模板不能包含空白字符")
				return
			}
		}
		set["repoLinkTemplate"] = tpl
	}
	if len(set) == 0 {
		httpx.Error(w, http.StatusBadRequest, "empty_update", "没有需要更新的字段")
		return
	}
	p, err := a.store.UpdateProject(r.Context(), t.ID, id, set)
	if err != nil {
		if store.IsNoDocuments(err) {
			httpx.Error(w, http.StatusNotFound, "project_not_found", "项目不存在")
			return
		}
		a.logError(r, "update project failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法更新项目")
		return
	}
	httpx.JSON(w, http.StatusOK, a.projectDTO(p, true))
}

func (a *API) rotateProjectKey(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	p, err := a.store.RotateProjectKey(r.Context(), t.ID, id)
	if err != nil {
		if store.IsNoDocuments(err) {
			httpx.Error(w, http.StatusNotFound, "project_not_found", "项目不存在")
			return
		}
		a.logError(r, "rotate project key failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法轮换密钥")
		return
	}
	a.logger.Slog.WarnContext(r.Context(), "project ingest key rotated", "projectId", p.ID.Hex(), "actor", currentUser(r).Email)
	httpx.JSON(w, http.StatusOK, a.projectDTO(p, true))
}

func (a *API) deleteProject(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	if err := a.store.DeleteProject(r.Context(), t.ID, id); err != nil {
		if store.IsNoDocuments(err) {
			httpx.Error(w, http.StatusNotFound, "project_not_found", "项目不存在")
			return
		}
		a.logError(r, "delete project failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法删除项目")
		return
	}
	a.logger.Slog.WarnContext(r.Context(), "project deleted", "projectId", id.Hex(), "actor", currentUser(r).Email)
	w.WriteHeader(http.StatusNoContent)
}

func validRepoProvider(p string) bool {
	switch p {
	case "", "github", "gitlab", "bitbucket", "gitea", "custom":
		return true
	}
	return false
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// normalizeRepoURL validates and canonicalizes a repository web URL. The
// provider is inferred for common hosts when the client did not specify one.
func normalizeRepoURL(raw, explicitProvider string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", strings.ToLower(strings.TrimSpace(explicitProvider)), nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", errors.New("源码仓库地址必须是 http(s) URL")
	}
	if len(raw) > 300 {
		return "", "", errors.New("源码仓库地址过长")
	}
	clean := strings.TrimRight(raw, "/")
	clean = strings.TrimSuffix(clean, ".git")
	provider := strings.ToLower(strings.TrimSpace(explicitProvider))
	if provider != "" && !validRepoProvider(provider) {
		return "", "", errors.New("源码平台必须是 github、gitlab、bitbucket、gitea 或 custom")
	}
	if provider == "" {
		host := strings.ToLower(u.Hostname())
		switch {
		case strings.Contains(host, "github"):
			provider = "github"
		case strings.Contains(host, "gitlab"):
			provider = "gitlab"
		case strings.Contains(host, "bitbucket"):
			provider = "bitbucket"
		case strings.Contains(host, "gitea"):
			provider = "gitea"
		default:
			provider = "custom"
		}
	}
	return clean, provider, nil
}
