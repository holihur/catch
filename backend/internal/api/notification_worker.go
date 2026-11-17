package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/catch/catch/backend/internal/model"
	"go.mongodb.org/mongo-driver/bson"
)

type notificationRule struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Metric        string  `json:"metric"`
	Service       string  `json:"service,omitempty"`
	Aggregation   string  `json:"aggregation"`
	Operator      string  `json:"operator"`
	Threshold     float64 `json:"threshold"`
	WindowSeconds int     `json:"windowSeconds"`
	Severity      string  `json:"severity"`
}

type notificationPayload struct {
	Version      int               `json:"version"`
	Event        string            `json:"event"`
	Alert        model.Alert       `json:"alert"`
	Rule         notificationRule  `json:"rule"`
	Tenant       map[string]string `json:"tenant,omitempty"`
	Project      map[string]string `json:"project,omitempty"`
	DashboardURL string            `json:"dashboardUrl"`
}

var notificationHTTPClient = &http.Client{Timeout: 15 * time.Second}

func (a *API) StartNotificationWorker(ctx context.Context) {
	interval := a.cfg.NotifyInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		a.processDueNotifications(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.processDueNotifications(ctx)
			}
		}
	}()
}

func (a *API) processDueNotifications(ctx context.Context) {
	deliveries, err := a.store.ClaimNotificationDeliveries(ctx, 50)
	if err != nil {
		a.logger.Slog.WarnContext(ctx, "notification worker: list due failed", "error", err)
		return
	}
	for _, delivery := range deliveries {
		a.processNotificationDelivery(ctx, delivery)
	}
}

func (a *API) processNotificationDelivery(ctx context.Context, delivery model.NotificationDelivery) {
	channel, err := a.store.NotificationChannelByID(ctx, delivery.TenantID, delivery.ChannelID)
	if err != nil {
		a.finishDelivery(ctx, delivery, 0, errors.New("通知渠道不存在"), false)
		return
	}
	if !channel.Enabled {
		a.finishDelivery(ctx, delivery, 0, errors.New("通知渠道已暂停"), false)
		return
	}
	statusCode, sendErr := a.dispatchNotification(ctx, *channel, []byte(delivery.Payload), delivery.ID.Hex())
	a.finishDelivery(ctx, delivery, statusCode, sendErr, sendErr == nil)
}

func (a *API) finishDelivery(ctx context.Context, delivery model.NotificationDelivery, statusCode int, sendErr error, success bool) {
	now := time.Now().UTC()
	attempts := delivery.Attempts
	if attempts < 1 {
		attempts = 1
	}
	set := bson.M{
		"attempts":   attempts,
		"statusCode": statusCode,
		"updatedAt":  now,
	}
	if success {
		set["status"] = "sent"
		set["sentAt"] = now
		set["error"] = ""
	} else {
		msg := ""
		if sendErr != nil {
			msg = sendErr.Error()
		}
		if len(msg) > 800 {
			msg = msg[:800]
		}
		set["error"] = msg
		if attempts >= a.cfg.NotifyAttempts {
			set["status"] = "failed"
		} else {
			backoff := time.Duration(1<<min(attempts, 6)) * 30 * time.Second
			if backoff > time.Hour {
				backoff = time.Hour
			}
			set["status"] = "pending"
			set["nextAttemptAt"] = now.Add(backoff)
		}
	}
	if err := a.store.UpdateNotificationDelivery(ctx, delivery.ID, set); err != nil {
		a.logger.Slog.WarnContext(ctx, "notification worker: update delivery failed", "deliveryId", delivery.ID.Hex(), "error", err)
	}
}

func (a *API) dispatchNotification(ctx context.Context, channel model.NotificationChannel, body []byte, deliveryID string) (int, error) {
	switch channel.Type {
	case "webhook":
		return a.sendWebhook(ctx, channel, body, deliveryID)
	case "email":
		return 0, a.sendEmail(ctx, channel, body)
	default:
		return 0, fmt.Errorf("不支持的通知渠道类型: %s", channel.Type)
	}
}

func (a *API) sendWebhook(ctx context.Context, channel model.NotificationChannel, body []byte, deliveryID string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, channel.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Catch-Alert/1.0")
	event := "alert"
	var payload notificationPayload
	if json.Unmarshal(body, &payload) == nil && payload.Event != "" {
		event = payload.Event
	}
	req.Header.Set("X-Catch-Event", event)
	if deliveryID != "" {
		req.Header.Set("X-Catch-Delivery", deliveryID)
	}
	if channel.WebhookSecretEnc != "" {
		secret, err := a.box.Decrypt(channel.WebhookSecretEnc)
		if err != nil {
			return 0, fmt.Errorf("解密 webhook 密钥失败")
		}
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		req.Header.Set("X-Catch-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	client := notificationHTTPClient
	if a.notifyClient != nil {
		client = a.notifyClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("webhook 返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return resp.StatusCode, nil
}

func (a *API) sendEmail(ctx context.Context, channel model.NotificationChannel, body []byte) error {
	if a.cfg.SMTPHost == "" {
		return errors.New("SMTP 未配置，无法发送邮件")
	}
	if len(channel.Recipients) == 0 {
		return errors.New("邮件渠道没有收件人")
	}
	var payload notificationPayload
	_ = json.Unmarshal(body, &payload)
	subject := fmt.Sprintf("[Catch][%s] %s", payload.Event, payload.Rule.Name)
	subject = sanitizeMailHeader(subject)
	from := a.cfg.SMTPFrom
	if from == "" {
		from = "catch@localhost"
	}
	toHeader := strings.Join(channel.Recipients, ", ")
	var msg strings.Builder
	msg.WriteString("From: " + sanitizeMailHeader(from) + "\r\n")
	msg.WriteString("To: " + sanitizeMailHeader(toHeader) + "\r\n")
	msg.WriteString("Subject: " + subject + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(payload.Alert.Message + "\r\n\r\n")
	msg.WriteString("级别: " + payload.Alert.Severity + "\r\n")
	msg.WriteString("指标: " + payload.Alert.Metric + "\r\n")
	msg.WriteString("当前值: " + strconv.FormatFloat(payload.Alert.Value, 'f', -1, 64) + "\r\n")
	msg.WriteString("阈值: " + strconv.FormatFloat(payload.Alert.Threshold, 'f', -1, 64) + "\r\n")
	msg.WriteString("状态: " + payload.Event + "\r\n")
	if payload.DashboardURL != "" {
		msg.WriteString("\r\n查看: " + payload.DashboardURL + "\r\n")
	}
	_ = ctx // net/smtp does not expose a context-aware API
	return a.smtpSend(from, channel.Recipients, []byte(msg.String()))
}

func (a *API) smtpSend(from string, to []string, msg []byte) error {
	addr := net.JoinHostPort(a.cfg.SMTPHost, a.cfg.SMTPPort)
	host := a.cfg.SMTPHost
	var client *smtp.Client
	var err error
	if a.cfg.SMTPTLSMode == "tls" {
		conn, derr := tls.Dial("tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if derr != nil {
			return derr
		}
		client, err = smtp.NewClient(conn, host)
	} else {
		client, err = smtp.Dial(addr)
		if err == nil && a.cfg.SMTPTLSMode == "starttls" {
			if ok, _ := client.Extension("STARTTLS"); !ok {
				_ = client.Close()
				return errors.New("SMTP 服务器不支持 STARTTLS")
			}
			if err = client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
				_ = client.Close()
				return err
			}
		}
	}
	if err != nil {
		return err
	}
	defer client.Close()
	if a.cfg.SMTPUsername != "" {
		auth := smtp.PlainAuth("", a.cfg.SMTPUsername, a.cfg.SMTPPassword, host)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	envelopeFrom := from
	if parsed, err := mailParseAddress(from); err == nil {
		envelopeFrom = parsed
	}
	if err := client.Mail(envelopeFrom); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// mailParseAddress accepts both bare addresses and display-name addresses.
func mailParseAddress(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("empty address")
	}
	if i := strings.LastIndex(raw, "<"); i >= 0 {
		raw = strings.TrimSuffix(raw[i+1:], ">")
	}
	return raw, nil
}

func sanitizeMailHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

func notifyRule(rule model.AlertRule) notificationRule {
	return notificationRule{
		ID: rule.ID.Hex(), Name: rule.Name, Metric: rule.Metric, Service: rule.Service,
		Aggregation: rule.Aggregation, Operator: rule.Operator, Threshold: rule.Threshold,
		WindowSeconds: rule.WindowSeconds, Severity: rule.Severity,
	}
}

func (a *API) enqueueAlertNotifications(ctx context.Context, rule model.AlertRule, alert model.Alert, event string) {
	if len(rule.ChannelIDs) == 0 {
		return
	}
	tenant, _ := a.store.TenantByID(ctx, rule.TenantID)
	project, _ := a.store.ProjectByID(ctx, rule.TenantID, rule.ProjectID)
	payload := notificationPayload{
		Version: 1, Event: event, Alert: alert, Rule: notifyRule(rule),
		Tenant:       map[string]string{"id": tenant.ID.Hex(), "slug": tenant.Slug, "name": tenant.Name},
		Project:      map[string]string{"id": project.ID.Hex(), "slug": project.Slug, "name": project.Name},
		DashboardURL: strings.TrimRight(a.cfg.AppURL, "/") + "/alerts",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		a.logger.Slog.WarnContext(ctx, "notification: marshal payload failed", "error", err)
		return
	}
	now := time.Now().UTC()
	for _, channelID := range rule.ChannelIDs {
		channel, err := a.store.NotificationChannelByID(ctx, rule.TenantID, channelID)
		if err != nil || channel == nil || !channel.Enabled {
			continue
		}
		delivery := &model.NotificationDelivery{
			TenantID: rule.TenantID, ProjectID: rule.ProjectID, AlertID: alert.ID, RuleID: rule.ID,
			ChannelID: channel.ID, ChannelName: channel.Name, ChannelType: channel.Type,
			Event: event, Status: "pending", Payload: string(body),
			NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
		}
		if err := a.store.EnqueueNotificationDelivery(ctx, delivery); err != nil {
			a.logger.Slog.WarnContext(ctx, "notification: enqueue failed", "channelId", channelID.Hex(), "error", err)
		}
	}
}

func (a *API) resolveRuleAlertsRespectingSilence(ctx context.Context, rule model.AlertRule) {
	notify := true
	if silenced, err := a.store.IsRuleSilenced(ctx, rule.TenantID, rule.ProjectID, rule.ID, time.Now().UTC()); err == nil {
		notify = !silenced
	}
	a.resolveRuleAlerts(ctx, rule, notify)
}

func (a *API) resolveRuleAlerts(ctx context.Context, rule model.AlertRule, notify bool) {
	alert, err := a.store.FiringAlert(ctx, rule.ID)
	if err != nil || alert == nil {
		return
	}
	now := time.Now().UTC()
	if err := a.store.ResolveAlerts(ctx, rule.ID, now); err != nil {
		return
	}
	alert.State = "resolved"
	alert.ResolvedAt = &now
	alert.UpdatedAt = now
	if notify {
		a.enqueueAlertNotifications(ctx, rule, *alert, "resolved")
	}
}
