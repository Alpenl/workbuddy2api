// Package logstore keeps a bounded, persistent journal of application logs.
package logstore

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	DefaultMaxEntries = 20000
	DefaultMaxBytes   = 20 << 20
	maxMessageBytes   = 16 << 10
)

type Entry struct {
	ID       uint64    `json:"id"`
	Time     time.Time `json:"time"`
	Level    string    `json:"level"`
	Category string    `json:"category"`
	Source   string    `json:"source,omitempty"`
	Account  string    `json:"account,omitempty"`
	Model    string    `json:"model,omitempty"`
	Status   int       `json:"status,omitempty"`
	Message  string    `json:"message"`
}

type Options struct {
	MaxEntries int
	MaxBytes   int
	Output     io.Writer
	Secrets    []string
}

type Query struct {
	Level, Category, Source, Account, Model, Text, Status string
	Since, Until                                          time.Time
	Before                                                uint64
	Limit                                                 int
}

type Page struct {
	Entries    []Entry    `json:"entries"`
	Total      int        `json:"total"`
	Retained   int        `json:"retained"`
	HasMore    bool       `json:"has_more"`
	NextBefore uint64     `json:"next_before,omitempty"`
	Oldest     *time.Time `json:"oldest,omitempty"`
	Newest     *time.Time `json:"newest,omitempty"`
	Accounts   []string   `json:"accounts"`
	Models     []string   `json:"models"`
	MaxEntries int        `json:"max_entries"`
	MaxBytes   int        `json:"max_bytes"`
	StorageOK  bool       `json:"storage_ok"`
}

type stored struct {
	entry Entry
	size  int
}

type Store struct {
	mu                                   sync.RWMutex
	path                                 string
	file                                 *os.File
	items                                []stored
	start, bytes, diskBytes, diskEntries int
	nextID                               uint64
	options                              Options
	storageOK                            bool
	checkinSource                        string
}

func Open(dir string, options Options) (*Store, error) {
	if options.MaxEntries <= 0 {
		options.MaxEntries = DefaultMaxEntries
	}
	if options.MaxBytes <= 0 {
		options.MaxBytes = DefaultMaxBytes
	}
	if options.Output == nil {
		options.Output = io.Discard
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "events.jsonl"), options: options, storageOK: true}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	s.file = f
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return nil, err
	}
	// A process crash can leave only the last record incomplete. Truncate that
	// tail before appending; never silently discard corrupt complete records.
	reader := bufio.NewReader(f)
	var offset int64
	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr == io.EOF {
			if len(line) > 0 {
				if err := f.Truncate(offset); err != nil {
					f.Close()
					return nil, err
				}
			}
			break
		}
		if readErr != nil {
			f.Close()
			return nil, readErr
		}
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil || entry.ID <= s.nextID || entry.Time.IsZero() {
			f.Close()
			return nil, fmt.Errorf("invalid log journal record at byte %d", offset)
		}
		s.nextID = entry.ID
		s.remember(entry, len(line))
		s.diskBytes += len(line)
		s.diskEntries++
		offset += int64(len(line))
	}
	if s.needsCompact() {
		if err := s.compact(); err != nil {
			f.Close()
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) Redact(message string) string {
	for _, secret := range s.options.Secrets {
		if len(secret) >= 8 {
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
		}
	}
	return Redact(message)
}

func (s *Store) remember(e Entry, size int) {
	s.items = append(s.items, stored{e, size})
	s.bytes += size
	for len(s.items)-s.start > s.options.MaxEntries || (s.bytes > s.options.MaxBytes && len(s.items)-s.start > 1) {
		s.bytes -= s.items[s.start].size
		s.items[s.start] = stored{}
		s.start++
	}
	if s.start > 1024 && s.start*2 >= len(s.items) {
		s.items = append([]stored(nil), s.items[s.start:]...)
		s.start = 0
	}
}

func (s *Store) needsCompact() bool {
	return s.diskBytes > s.options.MaxBytes*2 || s.diskEntries > s.options.MaxEntries*2
}

func (s *Store) compact() error {
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	w := bufio.NewWriter(f)
	total := 0
	for _, item := range s.items[s.start:] {
		raw, _ := json.Marshal(item.entry)
		raw = append(raw, '\n')
		if _, err = w.Write(raw); err != nil {
			f.Close()
			return err
		}
		total += len(raw)
	}
	if err = w.Flush(); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(tmp, s.path); err != nil {
		return err
	}
	newFile, err := os.OpenFile(s.path, os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	s.file.Close()
	s.file = newFile
	s.diskBytes = total
	s.diskEntries = len(s.items) - s.start
	return nil
}

func (s *Store) recordLocked(e Entry) error {
	e.Message = s.Redact(e.Message)
	e.Account = bounded(s.Redact(e.Account), 256)
	e.Model = bounded(s.Redact(e.Model), 256)
	if len(e.Message) > maxMessageBytes {
		e.Message = string(bytes.ToValidUTF8([]byte(e.Message[:maxMessageBytes]), []byte("�"))) + " … [truncated]"
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	s.nextID++
	e.ID = s.nextID
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	s.remember(e, len(raw))
	if s.file == nil {
		s.storageOK = false
		return fmt.Errorf("log store is closed")
	}
	previousBytes := s.diskBytes
	n, err := s.file.Write(raw)
	s.diskBytes += n
	s.diskEntries++
	if err == nil && n != len(raw) {
		err = io.ErrShortWrite
	}
	if err != nil && n > 0 && n < len(raw) {
		// Remove a partial line so a later successful write cannot corrupt the journal.
		if truncateErr := s.file.Truncate(int64(previousBytes)); truncateErr == nil {
			s.diskBytes = previousBytes
		}
	}
	if err == nil && s.needsCompact() {
		err = s.compact()
	}
	if err != nil {
		s.storageOK = false
		fmt.Fprintln(s.options.Output, "log journal write failed; recent logs remain in memory")
	}
	return err
}

func bounded(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return string(bytes.ToValidUTF8([]byte(value[:max]), []byte("�")))
}

// Record stores a structured event without duplicating the caller's console output.
func (s *Store) Record(e Entry) error { s.mu.Lock(); defer s.mu.Unlock(); return s.recordLocked(e) }

// Write is the standard logger's sink. A single Write remains a single event,
// including multiline upstream responses; secrets are removed before both outputs.
func (s *Store) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := Parse(string(p), time.Now())
	s.attributeSource(&e)
	e.Message = s.Redact(e.Message)
	fmt.Fprintf(s.options.Output, "%s %s\n", e.Time.Local().Format("2006/01/02 15:04:05"), e.Message)
	_ = s.recordLocked(e) // Logging failure must not fail an otherwise valid API request.
	return len(p), nil
}

func (s *Store) attributeSource(e *Entry) {
	if strings.HasPrefix(e.Message, "checkin source=") && strings.HasSuffix(e.Message, "started") {
		s.checkinSource = e.Source
	}
	if e.Source == "" && (e.Category == "checkin" || e.Category == "credits") {
		e.Source = s.checkinSource
	}
	if strings.HasPrefix(e.Message, "checkin source=") && strings.Contains(e.Message, " completed ") {
		s.checkinSource = ""
	}
}

// ImportDocker imports a captured `docker logs --timestamps` stream once into
// an empty journal. It requires a timestamp per line, and preserves those times.
func (s *Store) ImportDocker(r io.Reader) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nextID != 0 {
		return 0, fmt.Errorf("historical import requires an empty journal")
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	var entries []Entry
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			return 0, fmt.Errorf("missing Docker timestamp")
		}
		at, err := time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid Docker timestamp")
		}
		e := Parse(parts[1], at)
		e.Time = at
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Time.Before(entries[j].Time) })
	for i, e := range entries {
		s.attributeSource(&e)
		if err := s.recordLocked(e); err != nil {
			return i, err
		}
	}
	return len(entries), s.file.Sync()
}

func (s *Store) Query(q Query) Page {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 100
	}
	p := Page{Entries: []Entry{}, Accounts: []string{}, Models: []string{}, Retained: len(s.items) - s.start, MaxEntries: s.options.MaxEntries, MaxBytes: s.options.MaxBytes, StorageOK: s.storageOK}
	accounts, models := map[string]bool{}, map[string]bool{}
	for i := len(s.items) - 1; i >= s.start; i-- {
		e := s.items[i].entry
		if e.Account != "" {
			accounts[e.Account] = true
		}
		if e.Model != "" {
			models[e.Model] = true
		}
		if p.Oldest == nil || e.Time.Before(*p.Oldest) {
			v := e.Time
			p.Oldest = &v
		}
		if p.Newest == nil || e.Time.After(*p.Newest) {
			v := e.Time
			p.Newest = &v
		}
		if !matches(e, q) {
			continue
		}
		p.Total++
		if q.Before > 0 && e.ID >= q.Before {
			continue
		}
		if len(p.Entries) < q.Limit {
			p.Entries = append(p.Entries, e)
		} else {
			p.HasMore = true
		}
	}
	if p.HasMore && len(p.Entries) > 0 {
		p.NextBefore = p.Entries[len(p.Entries)-1].ID
	}
	for a := range accounts {
		p.Accounts = append(p.Accounts, a)
	}
	sort.Strings(p.Accounts)
	for m := range models {
		p.Models = append(p.Models, m)
	}
	sort.Strings(p.Models)
	return p
}

func matches(e Entry, q Query) bool {
	if q.Level != "" && q.Level != e.Level || q.Category != "" && q.Category != e.Category || q.Source != "" && q.Source != e.Source {
		return false
	}
	if !q.Since.IsZero() && e.Time.Before(q.Since) || !q.Until.IsZero() && e.Time.After(q.Until) {
		return false
	}
	for _, pair := range [][2]string{{e.Account, q.Account}, {e.Model, q.Model}, {e.Message, q.Text}} {
		if !strings.Contains(strings.ToLower(pair[0]), strings.ToLower(pair[1])) {
			return false
		}
	}
	if q.Status != "" {
		if strings.HasSuffix(q.Status, "xx") {
			return fmt.Sprint(e.Status/100) == q.Status[:1]
		}
		return fmt.Sprint(e.Status) == q.Status
	}
	return true
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Sync()
	closeErr := s.file.Close()
	s.file = nil
	if err != nil {
		return err
	}
	return closeErr
}
