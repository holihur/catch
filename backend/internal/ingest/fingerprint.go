package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	uuidRe    = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	hexRe     = regexp.MustCompile(`\b[0-9a-fA-F]{12,}\b`)
	numRe     = regexp.MustCompile(`\b\d+(\.\d+)?\b`)
	ipRe      = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)
	quotedRe  = regexp.MustCompile("(['\"`])[^'\"`]{1,120}(['\"`])")
	spaceRe   = regexp.MustCompile(`\s+`)
	emailRe   = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	frameFile = regexp.MustCompile(`(?:\(|at\s+|@)([^()\s]+?):(\d+)(?::\d+)?(?:\)|$)`)
)

// NormalizeMessage removes volatile values so that "user 123 not found" and
// "user 456 not found" group into the same issue.
func NormalizeMessage(msg string) string {
	s := strings.TrimSpace(strings.ToLower(msg))
	s = uuidRe.ReplaceAllString(s, "<uuid>")
	s = ipRe.ReplaceAllString(s, "<ip>")
	s = hexRe.ReplaceAllString(s, "<hex>")
	s = emailRe.ReplaceAllString(s, "<email>")
	s = quotedRe.ReplaceAllString(s, "<str>")
	s = numRe.ReplaceAllString(s, "<n>")
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// ErrorType extracts a leading error class such as "TypeError" or
// "MongoServerError" from the message.
func ErrorType(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	first := msg
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	// "TypeError: x is undefined" or "rpc error: ..."
	if i := strings.Index(first, ":"); i > 0 && i < 80 && !strings.ContainsAny(first[:i], " /\\") {
		return strings.TrimSpace(first[:i])
	}
	// Go panic style: "panic: runtime error: ..."
	fields := strings.Fields(first)
	if len(fields) > 0 && len(fields[0]) < 60 && !strings.ContainsAny(fields[0], "\\/") {
		return strings.TrimSuffix(fields[0], ":")
	}
	return ""
}

// Culprit returns the most relevant stack frame: the first frame that looks
// like application code (skipping node internals and library paths).
func Culprit(stack string) string {
	lines := strings.Split(strings.TrimSpace(stack), "\n")
	for _, line := range lines {
		l := strings.TrimSpace(line)
		if l == "" || !strings.Contains(l, ":") {
			continue
		}
		lower := strings.ToLower(l)
		if strings.Contains(lower, "node_modules") ||
			strings.Contains(lower, "/usr/local/go/") ||
			strings.Contains(lower, "runtime/") ||
			strings.Contains(lower, "node:internal") ||
			strings.Contains(lower, "<anonymous>") {
			continue
		}
		if m := frameFile.FindStringSubmatch(l); len(m) == 3 {
			file := m[1]
			if idx := strings.LastIndex(file, "/"); idx >= 0 {
				file = file[idx+1:]
			}
			loc := file + ":" + m[2]
			if strings.HasPrefix(l, "at ") {
				rest := strings.TrimPrefix(l, "at ")
				if i := strings.Index(rest, " ("); i > 0 {
					loc = rest[:i] + " (" + loc + ")"
				}
			}
			return truncate(loc, 240)
		}
	}
	// Fallback: first non-empty line, sanitized.
	for _, line := range lines {
		l := strings.TrimSpace(line)
		if l != "" {
			return truncate(l, 240)
		}
	}
	return ""
}

// Fingerprint builds a stable issue fingerprint. Volatile IDs and numbers are
// normalized, and the top application frame participates in grouping.
func Fingerprint(source, errType, message, culprit string) string {
	if errType == "" {
		errType = ErrorType(message)
	}
	parts := []string{
		strings.ToLower(strings.TrimSpace(source)),
		strings.ToLower(errType),
		NormalizeMessage(message),
		NormalizeMessage(culprit),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:16])
}

// Title produces a compact human-readable issue title.
func Title(errType, message string) string {
	msg := strings.TrimSpace(message)
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = strings.TrimSpace(msg[:i])
	}
	if errType != "" && !strings.HasPrefix(strings.ToLower(msg), strings.ToLower(errType)) {
		return truncate(errType+": "+msg, 200)
	}
	if msg == "" {
		msg = "Unknown error"
	}
	return truncate(msg, 200)
}

// DefaultLevel maps frontend/backend log levels into Catch's fixed set.
func DefaultLevel(level string, kind string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "trace", "debug":
		return "debug"
	case "info", "notice", "log":
		return "info"
	case "warn", "warning":
		return "warn"
	case "error", "err":
		return "error"
	case "fatal", "panic", "critical", "crit":
		return "fatal"
	}
	if kind == "error" {
		return "error"
	}
	return "info"
}

// Truncate caps a string at n bytes without breaking UTF-8.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return fmt.Sprintf("%s…", s[:n])
}

// Truncate is the exported variant used by the ingest handler.
func Truncate(s string, n int) string { return truncate(s, n) }
