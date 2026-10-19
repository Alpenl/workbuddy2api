package logstore

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func openTest(t *testing.T, dir string, o Options) *Store {
	t.Helper()
	s, err := Open(dir, o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRetentionPersistenceAndCrashTail(t *testing.T) {
	dir := t.TempDir()
	s := openTest(t, dir, Options{MaxEntries: 3})
	for i := 0; i < 10; i++ {
		if err := s.Record(Entry{Message: fmt.Sprint(i)}); err != nil {
			t.Fatal(err)
		}
	}
	p := s.Query(Query{Limit: 2})
	if p.Retained != 3 || !p.HasMore || p.NextBefore != 9 || p.Entries[0].ID != 10 {
		t.Fatalf("page=%+v", p)
	}
	s.Close()
	f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"id":11`)
	f.Close()
	s = openTest(t, dir, Options{MaxEntries: 3})
	if err := s.Record(Entry{Message: "after restart"}); err != nil {
		t.Fatal(err)
	}
	p = s.Query(Query{})
	if p.Entries[0].ID != 11 || p.Retained != 3 {
		t.Fatalf("recovered=%+v", p)
	}
	s.Close()
	s = openTest(t, dir, Options{MaxEntries: 3})
	if s.Query(Query{}).Entries[0].ID != 11 {
		t.Fatal("append after crash damaged journal")
	}
	info, _ := os.Stat(filepath.Join(dir, "events.jsonl"))
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
}

func TestCorruptCompleteRecordIsRejected(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte("not json\n"), 0600)
	if _, err := Open(dir, Options{}); err == nil {
		t.Fatal("corruption silently accepted")
	}
}

func TestRedactionClassificationAndSources(t *testing.T) {
	var output bytes.Buffer
	dir := t.TempDir()
	s := openTest(t, dir, Options{Output: &output, Secrets: []string{"private-console-key"}})
	for _, line := range []string{
		"2026/10/19 09:00:00 checkin source=scheduled started\n",
		"user-resource abc123 attempt=1/3 failed: http 500 retry_in=5s\n",
		"checkin abc123 ok=true credits=3394\n",
		"checkin source=scheduled completed total=1 succeeded=1 failed=0\n",
		"user-resource def456 failed: http 401\n",
		`upstream error: private-console-key access_token=access-secret refreshToken="refresh-secret" Authorization: Bearer auth-secret eyJabc.def.ghi sk-abcdefghijklmnop https://user:pass@example.com`,
	} {
		s.Write([]byte(line))
	}
	p := s.Query(Query{})
	if p.Entries[4].Source != "scheduled" || p.Entries[4].Level != "warn" || p.Entries[4].Status != 500 || p.Entries[4].Account != "abc123" {
		t.Fatalf("retry=%+v", p.Entries[4])
	}
	if p.Entries[2].Level != "info" || p.Entries[1].Source != "" {
		t.Fatal("summary/source leaked to later operations")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	for _, secret := range []string{"private-console-key", "access-secret", "refresh-secret", "auth-secret", "eyJabc.def.ghi", "sk-abcdefghijklmnop", "user:pass"} {
		if strings.Contains(string(raw), secret) || strings.Contains(output.String(), secret) || strings.Contains(p.Entries[0].Message, secret) {
			t.Fatalf("secret leaked: %s", secret)
		}
	}
}

func TestFiltersAndCursorSurviveConcurrentAppend(t *testing.T) {
	s := openTest(t, t.TempDir(), Options{})
	at := time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		s.Record(Entry{Time: at.Add(time.Duration(i) * time.Second), Level: "error", Category: "chat", Source: "manual", Account: "ABC123", Model: "deepseek-v4-flash", Status: 500, Message: "Upstream unavailable"})
	}
	q := Query{Level: "error", Category: "chat", Source: "manual", Account: "abc", Model: "V4", Text: "UNAVAILABLE", Status: "5xx", Since: at, Until: at.Add(4 * time.Second), Limit: 2}
	p := s.Query(q)
	if p.Total != 5 || len(p.Entries) != 2 || p.NextBefore != 4 {
		t.Fatalf("first=%+v", p)
	}
	s.Record(Entry{Message: "new arrival"})
	q.Before = p.NextBefore
	p = s.Query(q)
	if p.Total != 5 || p.Entries[0].ID != 3 || p.Entries[1].ID != 2 {
		t.Fatalf("cursor=%+v", p)
	}
	q.Status = "401"
	if s.Query(q).Total != 0 {
		t.Fatal("status filter ignored")
	}
}

func TestImportOrdersTimestampsAndRejectsDuplicate(t *testing.T) {
	s := openTest(t, t.TempDir(), Options{})
	input := "2026-10-19T01:00:02Z checkin abc123 ok=true\n2026-10-19T01:00:01Z checkin source=scheduled started\n2026-10-19T01:00:03Z checkin source=scheduled completed total=1 succeeded=1 failed=0\n"
	n, err := s.ImportDocker(strings.NewReader(input))
	if err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	p := s.Query(Query{})
	if p.Entries[1].Source != "scheduled" || p.Entries[0].Time.Second() != 3 {
		t.Fatal(p)
	}
	if _, err = s.ImportDocker(strings.NewReader(input)); err == nil {
		t.Fatal("duplicate import accepted")
	}
	other := openTest(t, t.TempDir(), Options{})
	if _, err = other.ImportDocker(strings.NewReader("missing timestamp\n")); err == nil {
		t.Fatal("bad import accepted")
	}
}

func TestByteBoundsAndConcurrentReaders(t *testing.T) {
	s := openTest(t, t.TempDir(), Options{MaxBytes: 2000})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				s.Record(Entry{Message: strings.Repeat("界", 200), Model: strings.Repeat("界", 200)})
				s.Query(Query{})
			}
		}()
	}
	wg.Wait()
	p := s.Query(Query{})
	if p.Retained > 3 || p.Retained < 1 || !p.StorageOK {
		t.Fatalf("bounds=%+v", p)
	}
	s.Record(Entry{Message: strings.Repeat("界", maxMessageBytes)})
	e := s.Query(Query{}).Entries[0]
	if !utf8.ValidString(e.Message) || len(e.Message) > maxMessageBytes+30 {
		t.Fatal("invalid truncation")
	}
}
