package api

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// webhookPolicy blocks webhook targets that resolve to private/link-local
// addresses unless the host/CIDR is explicitly allowlisted.
type webhookPolicy struct {
	allowPrivate bool
	hosts        map[string]bool
	networks     []*net.IPNet
}

func newWebhookPolicy(allowPrivate bool, entries []string) *webhookPolicy {
	p := &webhookPolicy{allowPrivate: allowPrivate, hosts: map[string]bool{}}
	for _, entry := range entries {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(entry); err == nil {
			p.networks = append(p.networks, network)
			continue
		}
		if ip := net.ParseIP(entry); ip != nil {
			bits := 128
			if ip.To4() != nil {
				bits = 32
				ip = ip.To4()
			}
			p.networks = append(p.networks, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		p.hosts[strings.TrimSuffix(entry, ".")] = true
	}
	return p
}

func (p *webhookPolicy) configured() bool {
	return p != nil && (p.allowPrivate || len(p.hosts) > 0 || len(p.networks) > 0)
}

func (p *webhookPolicy) hostAllowed(host string) bool {
	if p == nil {
		return false
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return p.allowPrivate || p.hosts[host]
}

func (p *webhookPolicy) ipAllowed(ip net.IP) bool {
	if p == nil {
		return false
	}
	if p.allowPrivate {
		return true
	}
	for _, network := range p.networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func isPrivateDestination(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast()
}

// CheckURL validates a webhook target before it is saved.
func (p *webhookPolicy) CheckURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("Webhook URL 必须是完整的 http/https 地址")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" {
		return errors.New("Webhook URL 缺少主机名")
	}
	if p.hostAllowed(host) {
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("无法解析 Webhook 主机: %s", host)
	}
	for _, ip := range ips {
		if isPrivateDestination(ip) && !p.ipAllowed(ip) {
			return fmt.Errorf("Webhook 不允许访问内网地址 %s；生产环境请配置 NOTIFY_PRIVATE_ALLOWLIST 或 NOTIFY_ALLOW_PRIVATE", ip.String())
		}
	}
	return nil
}

func (p *webhookPolicy) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	// Explicitly allowlisted hostnames are dialed by name so internal DNS and
	// split-horizon records keep working. CIDR-only allowlists still resolve
	// and dial by validated IP below.
	if p.hostAllowed(host) {
		return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, fmt.Errorf("resolve webhook host: %w", err)
	}
	var lastErr error
	for _, ip := range ips {
		if isPrivateDestination(ip) && !p.ipAllowed(ip) {
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("webhook target %s resolves only to blocked private addresses", host)
	}
	return nil, lastErr
}

func newNotificationHTTPClient(policy *webhookPolicy) *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           policy.dialContext,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConns:          20,
	}
	return &http.Client{Timeout: 20 * time.Second, Transport: transport}
}
