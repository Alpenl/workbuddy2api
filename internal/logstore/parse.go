package logstore

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	credentialRE    = regexp.MustCompile(`(?i)((?:authorization|x-refresh-token|access[_-]?token|refresh[_-]?token|api[_-]?key|password|cookie)["']?\s*[:=]\s*["']?)(?:Bearer\s+)?[^\s,"'};]+`)
	bearerRE        = regexp.MustCompile(`(?i)\bBearer\s+[a-z0-9._~+/=-]+`)
	jwtRE           = regexp.MustCompile(`\beyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+`)
	keyRE           = regexp.MustCompile(`\bsk-[a-zA-Z0-9_-]{12,}`)
	urlCredentialRE = regexp.MustCompile(`(https?://)[^\s/@]+:[^\s/@]+@`)
	uidRE           = regexp.MustCompile(`(?:uid=|^(?:checkin|user-resource|keepalive|refresh) )([a-zA-Z0-9_-]+)`)
	statusRE        = regexp.MustCompile(`(?:http |upstream |status(?: code)?[=: ]+)([1-5][0-9]{2})\b`)
	sourceRE        = regexp.MustCompile(`\bsource=(manual|scheduled)\b`)
	failedRE        = regexp.MustCompile(`\bfailed=[1-9][0-9]*`)
)

func Redact(s string) string {
	s = credentialRE.ReplaceAllString(s, "${1}[REDACTED]")
	s = bearerRE.ReplaceAllString(s, "Bearer [REDACTED]")
	s = jwtRE.ReplaceAllString(s, "[REDACTED]")
	s = keyRE.ReplaceAllString(s, "[REDACTED]")
	return urlCredentialRE.ReplaceAllString(s, "${1}[REDACTED]@")
}

func Parse(line string, at time.Time) Entry {
	line = strings.TrimRight(line, "\r\n")
	if len(line) > 20 {
		if _, err := time.ParseInLocation("2006/01/02 15:04:05", line[:19], time.Local); err == nil {
			line = line[20:]
		}
	}
	e := Entry{Time: at, Level: "info", Category: "system", Message: line}
	switch {
	case strings.HasPrefix(line, "| #"):
		e.Category = "chat"
		parts := strings.Split(line, "|")
		if len(parts) > 6 {
			e.Model = strings.TrimSpace(parts[3])
			e.Status, _ = strconv.Atoi(strings.TrimSpace(parts[5]))
		}
	case strings.HasPrefix(line, "checkin "):
		e.Category = "checkin"
	case strings.HasPrefix(line, "user-resource "):
		e.Category = "credits"
	case strings.HasPrefix(line, "scheduler "):
		e.Category = "scheduler"
		e.Source = "scheduled"
	case strings.HasPrefix(line, "keepalive ") || strings.HasPrefix(line, "refresh "):
		e.Category = "auth"
	case strings.HasPrefix(line, "chat") || strings.HasPrefix(line, "reasoning_effort "):
		e.Category = "chat"
	}
	if m := uidRE.FindStringSubmatch(line); len(m) > 1 && m[1] != "source" {
		e.Account = m[1]
	}
	if m := sourceRE.FindStringSubmatch(line); len(m) > 1 {
		e.Source = m[1]
	}
	if m := statusRE.FindStringSubmatch(line); len(m) > 1 {
		e.Status, _ = strconv.Atoi(m[1])
	}
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "retry_in=") || strings.Contains(lower, "unauthorized; refreshing") || strings.Contains(lower, "downgraded"):
		e.Level = "warn"
	case e.Status >= 400 || strings.Contains(lower, "failed") || strings.Contains(lower, "error") || strings.Contains(lower, "panic") || strings.Contains(lower, "fatal") || strings.Contains(lower, "ok=false"):
		e.Level = "error"
	}
	// A summary with failed=0 is successful, not an error containing "failed".
	if strings.Contains(line, " completed ") {
		e.Level = "info"
		if failedRE.MatchString(line) {
			e.Level = "error"
		}
	}
	return e
}
