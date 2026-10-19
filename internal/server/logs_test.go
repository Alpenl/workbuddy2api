package server

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
	"workbuddy2api/internal/logstore"
)

func TestLogsEndpointAuthFiltersAndValidation(t *testing.T) {
	s, err := logstore.Open(t.TempDir(), logstore.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Record(logstore.Entry{Level: "warn", Category: "credits", Source: "scheduled", Status: 500, Message: "retry", Account: "abc123"})
	h := NewHandler(Config{Pool: testPoolWith(), APIKey: "test-secret", Logs: s})
	call := func(query, key string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/admin/logs"+query, nil)
		if key != "" {
			r.Header.Set("Authorization", "Bearer "+key)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, key := range []string{"", "wrong"} {
		w := call("", key)
		if w.Code != 401 || strings.Contains(w.Body.String(), "retry") {
			t.Fatalf("auth %d", w.Code)
		}
	}
	w := call("?source=scheduled&status=5xx&account=abc&limit=1", "test-secret")
	var p logstore.Page
	json.Unmarshal(w.Body.Bytes(), &p)
	if w.Code != 200 || p.Total != 1 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("response %d %s", w.Code, w.Body)
	}
	for _, query := range []string{"level=debug", "level=info+warn", "source=bad", "category=other", "limit=0", "limit=201", "before=-1", "before=0", "since=yesterday", "since=2026-10-20T00:00:00Z&until=2026-10-19T00:00:00Z", "status=999", "q=" + url.QueryEscape(strings.Repeat("x", 513))} {
		if got := call("?"+query, "test-secret").Code; got != 400 {
			t.Errorf("%s: %d", query, got)
		}
	}
}

func TestChatJournalCapturesFullModelAndRedacts(t *testing.T) {
	withChatLog(t)
	s, err := logstore.Open(t.TempDir(), logstore.Options{Secrets: []string{"private-console-key"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	out := captureStdout(t, func() {
		logChatRow(time.Millisecond, time.Second, "deepseek-v4-flash-private-console-key", "stream", "abc123456789", 200, 5, s)
	})
	p := s.Query(logstore.Query{})
	if p.Total != 1 || p.Entries[0].Category != "chat" || p.Entries[0].Status != 200 || p.Entries[0].Account != "abc12345" || !strings.Contains(p.Entries[0].Model, "deepseek-v4-flash") {
		t.Fatalf("entry=%+v", p)
	}
	if strings.Contains(out, "private-console-key") || strings.Contains(p.Entries[0].Model, "private-console-key") {
		t.Fatal("chat key leaked")
	}
}
