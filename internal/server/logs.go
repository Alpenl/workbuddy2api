package server

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"workbuddy2api/internal/logstore"
)

func (h *Handler) logs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.cfg.Logs == nil {
		writeOpenAIError(w, 503, "logs_unavailable", "Log journal is unavailable")
		return
	}
	q, err := parseLogQuery(r)
	if err != nil {
		writeOpenAIError(w, 400, "invalid_log_filter", err.Error())
		return
	}
	writeJSON(w, 200, h.cfg.Logs.Query(q))
}

func parseLogQuery(r *http.Request) (logstore.Query, error) {
	v := r.URL.Query()
	q := logstore.Query{Level: v.Get("level"), Category: v.Get("category"), Source: v.Get("source"), Account: v.Get("account"), Model: v.Get("model"), Text: v.Get("q"), Status: v.Get("status"), Limit: 100}
	for k, allowed := range map[string]string{"level": "info warn error", "category": "system scheduler checkin credits auth chat", "source": "manual scheduled"} {
		if value := v.Get(k); value != "" && !slices.Contains(strings.Fields(allowed), value) {
			return q, fmt.Errorf("invalid %s", k)
		}
	}
	for _, k := range []string{"q", "account", "model"} {
		if len(v.Get(k)) > 512 {
			return q, fmt.Errorf("%s exceeds 512 bytes", k)
		}
	}
	if value := v.Get("limit"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 200 {
			return q, fmt.Errorf("limit must be between 1 and 200")
		}
		q.Limit = n
	}
	if value := v.Get("before"); value != "" {
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil || n == 0 {
			return q, fmt.Errorf("before must be a positive cursor")
		}
		q.Before = n
	}
	for k, target := range map[string]*time.Time{"since": &q.Since, "until": &q.Until} {
		if value := v.Get(k); value != "" {
			at, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return q, fmt.Errorf("%s must be an RFC3339 timestamp", k)
			}
			*target = at
		}
	}
	if !q.Since.IsZero() && !q.Until.IsZero() && q.Since.After(q.Until) {
		return q, fmt.Errorf("since must not be later than until")
	}
	if q.Status != "" && q.Status != "2xx" && q.Status != "3xx" && q.Status != "4xx" && q.Status != "5xx" {
		n, err := strconv.Atoi(q.Status)
		if err != nil || n < 100 || n > 599 {
			return q, fmt.Errorf("invalid HTTP status")
		}
	}
	return q, nil
}
