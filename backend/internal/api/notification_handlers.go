package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/catch/catch/backend/internal/httpx"
	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/store"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func normalizeRecipients(values []string) ([]string, string) {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			addr, err := mail.ParseAddress(part)
			if err != nil || addr.Address == "" {
				return nil, fmt.Sprintf("邮箱地址无效: %s", part)
			}
			key := strings.ToLower(addr.Address)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, addr.Address)
		}
	}
	if len(out) == 0 {
		return nil, "至少填写一个收件人"
	}
	if len(out) > 20 {
		return nil, "最多 20 个收件人"
	}
	return out, ""
}

func validateNotificationChannel(channel *model.NotificationChannel) string {
	channel.Name = strings.TrimSpace(channel.Name)
	if channel.Name == "" || len(channel.Name) > 100 {
		return "渠道名称长度需在 1-100 之间"
	}
	switch channel.Type {
	case "webhook":
		channel.WebhookURL = strings.TrimSpace(channel.WebhookURL)
		u, err := url.Parse(channel.WebhookURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "Webhook URL 必须是完整的 http/https 地址"
		}
		if len(channel.WebhookURL) > 2048 {
			return "Webhook URL 过长"
		}
		if len(channel.WebhookSecretEnc) > 4096 {
			return "Webhook 签名密钥过长"
		}
		channel.Recipients = nil
	case "email":
		recipients, msg := normalizeRecipients(channel.Recipients)
		if msg != "" {
			return msg
		}
		channel.Recipients = recipients
		channel.WebhookURL = ""
		channel.WebhookSecretEnc = ""
	default:
		return "渠道类型必须是 webhook 或 email"
	}
	return ""
}

func (a *API) listNotificationChannels(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	channels, err := a.store.ListNotificationChannels(r.Context(), t.ID)
	if err != nil {
		a.logError(r, "list notification channels failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载通知渠道")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"channels":       channels,
		"smtpConfigured": a.cfg.SMTPHost != "",
	})
}

func (a *API) createNotificationChannel(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	var body struct {
		Name          string   `json:"name"`
		Type          string   `json:"type"`
		Enabled       *bool    `json:"enabled"`
		WebhookURL    string   `json:"webhookUrl"`
		WebhookSecret string   `json:"secret"`
		Recipients    []string `json:"recipients"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 16<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	channel := &model.NotificationChannel{
		TenantID: t.ID, Name: body.Name, Type: strings.ToLower(strings.TrimSpace(body.Type)),
		Enabled: enabled, WebhookURL: body.WebhookURL, Recipients: body.Recipients,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if body.WebhookSecret != "" {
		enc, err := a.box.Encrypt(body.WebhookSecret)
		if err != nil {
			a.logError(r, "encrypt webhook secret failed", err)
			httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法保存签名密钥")
			return
		}
		channel.WebhookSecretEnc = enc
	}
	if msg := validateNotificationChannel(channel); msg != "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_channel", msg)
		return
	}
	if channel.Type == "webhook" {
		if err := a.notifyPolicy.CheckURL(channel.WebhookURL); err != nil {
			httpx.Error(w, http.StatusBadRequest, "unsafe_webhook", err.Error())
			return
		}
	}
	if err := a.store.CreateNotificationChannel(r.Context(), channel); err != nil {
		a.logError(r, "create notification channel failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法创建通知渠道")
		return
	}
	a.logger.Slog.InfoContext(r.Context(), "notification channel created", "channelId", channel.ID.Hex(), "type", channel.Type, "actor", currentUser(r).Email)
	httpx.JSON(w, http.StatusCreated, channel)
}

func (a *API) updateNotificationChannel(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	channel, err := a.store.NotificationChannelByID(r.Context(), t.ID, id)
	if err != nil {
		if store.IsNoDocuments(err) {
			httpx.Error(w, http.StatusNotFound, "channel_not_found", "通知渠道不存在")
			return
		}
		a.logError(r, "load notification channel failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载通知渠道")
		return
	}
	var body struct {
		Name          *string   `json:"name"`
		Type          *string   `json:"type"`
		Enabled       *bool     `json:"enabled"`
		WebhookURL    *string   `json:"webhookUrl"`
		WebhookSecret *string   `json:"secret"`
		Recipients    *[]string `json:"recipients"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 16<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if body.Name != nil {
		channel.Name = *body.Name
	}
	if body.Type != nil {
		channel.Type = strings.ToLower(strings.TrimSpace(*body.Type))
	}
	if body.Enabled != nil {
		channel.Enabled = *body.Enabled
	}
	if body.WebhookURL != nil {
		channel.WebhookURL = *body.WebhookURL
	}
	if body.WebhookSecret != nil {
		enc, err := a.box.Encrypt(*body.WebhookSecret)
		if err != nil {
			a.logError(r, "encrypt webhook secret failed", err)
			httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法保存签名密钥")
			return
		}
		channel.WebhookSecretEnc = enc
	}
	if body.Recipients != nil {
		channel.Recipients = *body.Recipients
	}
	if msg := validateNotificationChannel(channel); msg != "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_channel", msg)
		return
	}
	if channel.Type == "webhook" {
		if err := a.notifyPolicy.CheckURL(channel.WebhookURL); err != nil {
			httpx.Error(w, http.StatusBadRequest, "unsafe_webhook", err.Error())
			return
		}
	}
	set := bson.M{
		"name": channel.Name, "type": channel.Type, "enabled": channel.Enabled,
		"webhookUrl": channel.WebhookURL, "webhookSecretEnc": channel.WebhookSecretEnc,
		"recipients": channel.Recipients,
	}
	updated, err := a.store.UpdateNotificationChannel(r.Context(), t.ID, id, set)
	if err != nil {
		a.logError(r, "update notification channel failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法更新通知渠道")
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

func (a *API) deleteNotificationChannel(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	if err := a.store.DeleteNotificationChannel(r.Context(), t.ID, id); err != nil {
		a.logError(r, "delete notification channel failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法删除通知渠道")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) testNotificationChannel(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	channel, err := a.store.NotificationChannelByID(r.Context(), t.ID, id)
	if err != nil {
		if store.IsNoDocuments(err) {
			httpx.Error(w, http.StatusNotFound, "channel_not_found", "通知渠道不存在")
			return
		}
		a.logError(r, "load notification channel failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载通知渠道")
		return
	}
	if channel.Type == "webhook" {
		if err := a.notifyPolicy.CheckURL(channel.WebhookURL); err != nil {
			httpx.Error(w, http.StatusBadRequest, "unsafe_webhook", err.Error())
			return
		}
	}
	payload := notificationPayload{
		Version: 1, Event: "test",
		Rule:         notificationRule{Name: "Catch 测试通知", Metric: "catch.test", Threshold: 1, Operator: "gt", Severity: "warning"},
		Alert:        model.Alert{RuleName: "Catch 测试通知", Metric: "catch.test", Severity: "warning", Message: "这是一条来自 Catch 的测试通知", State: "test", Value: 1, Threshold: 1},
		DashboardURL: strings.TrimRight(a.cfg.AppURL, "/") + "/alerts",
	}
	body, _ := json.Marshal(payload)
	statusCode, sendErr := a.dispatchNotification(r.Context(), *channel, body, "")
	if sendErr != nil {
		httpx.Error(w, http.StatusBadGateway, "delivery_failed", sendErr.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true, "statusCode": statusCode})
}

func (a *API) listNotificationDeliveries(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	projectID, err := a.projectFilter(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_project", err.Error())
		return
	}
	var pid primitive.ObjectID
	if projectID != nil {
		pid = *projectID
	}
	alertID := primitive.NilObjectID
	if raw := strings.TrimSpace(r.URL.Query().Get("alertId")); raw != "" {
		id, err := primitive.ObjectIDFromHex(raw)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid_alert_id", "告警 ID 无效")
			return
		}
		alertID = id
	}
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	switch status {
	case "", "all", "pending", "processing", "sent", "failed":
	default:
		httpx.Error(w, http.StatusBadRequest, "invalid_status", "status 无效")
		return
	}
	limit := int64(50)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := parseInt(raw); err == nil {
			limit = int64(n)
		}
	}
	items, err := a.store.ListNotificationDeliveries(r.Context(), t.ID, pid, alertID, status, limit)
	if err != nil {
		a.logError(r, "list notification deliveries failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载通知投递记录")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"deliveries": items})
}

func parseInt(raw string) (int, error) {
	var n int
	_, err := fmt.Sscanf(raw, "%d", &n)
	return n, err
}
