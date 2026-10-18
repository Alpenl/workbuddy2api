package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"workbuddy2api/internal/auth"
	"workbuddy2api/internal/pool"
	"workbuddy2api/internal/upstream"
)

func TestNextFire(t *testing.T) {
	loc := time.Local
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, loc)
	next := nextFire(now, []int{9, 21})
	if next.Hour() != 21 || next.Day() != 27 {
		t.Errorf("next=%v want 21:00 same day", next)
	}
	now = time.Date(2026, 7, 27, 22, 0, 0, 0, loc)
	next = nextFire(now, []int{9, 21})
	if next.Hour() != 9 || next.Day() != 28 {
		t.Errorf("next=%v want 09:00 next day", next)
	}
	now = time.Date(2026, 7, 27, 9, 0, 0, 0, loc)
	next = nextFire(now, []int{9})
	if next.Day() != 28 {
		t.Errorf("exact match should roll to next day: %v", next)
	}
}

func TestNextFireMergesSchedules(t *testing.T) {
	now := time.Date(2026, 7, 27, 20, 0, 0, 0, time.Local)
	next := nextFire(now, []int{9, 21, 22})
	if next.Hour() != 21 {
		t.Errorf("next=%v want 21 (earliest of 21/22)", next)
	}
}

// fakeUpstream 同时模拟 billing 与 refresh。
type fakeUpstream struct {
	checkinCalls   atomic.Int32
	refreshCalls   atomic.Int32
	resourceRemain int64
}

func (f *fakeUpstream) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/daily-checkin"):
			f.checkinCalls.Add(1)
			w.Write([]byte(`{"code":0,"msg":"ok","data":{}}`))
		case strings.HasSuffix(r.URL.Path, "/get-user-resource"):
			w.Write([]byte(`{"code":0,"data":{"Response":{"Data":{"Accounts":[{"CycleCapacitySize":100,"CycleCapacityRemain":` +
				jsonI64(f.resourceRemain) + `,"CycleCapacityUsed":0}]}}}}`))
		case strings.HasSuffix(r.URL.Path, "/token/refresh"):
			f.refreshCalls.Add(1)
			w.Write([]byte(`{"code":0,"data":{"accessToken":"new","expiresIn":3600}}`))
		default:
			http.Error(w, "not found", 404)
		}
	}))
}

func jsonI64(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestRunCheckinReenablesCoolingAccount(t *testing.T) {
	f := &fakeUpstream{resourceRemain: 500}
	srv := f.server()
	defer srv.Close()

	p := pool.New("")
	a := &auth.Auth{UID: "u1", AccessToken: "at", RefreshToken: "rt", ExpiresAt: 9999999999}
	p.Add(a)
	p.Cooldown("u1", pool.CoolHard, time.Hour, "余额不足")

	up := &upstream.Client{
		HTTP:            srv.Client(),
		ChatBaseCN:      srv.URL,
		BillingBaseCN:   srv.URL,
		ChatBaseGlobal:  srv.URL,
		BillingBaseGlob: srv.URL,
	}
	s := New(Config{
		Pool:           p,
		Upstream:       up,
		CheckinHours:   []int{9, 21},
		KeepaliveHours: []int{22},
	})
	report := s.RunCheckinNow()
	if f.checkinCalls.Load() != 1 {
		t.Errorf("checkin calls=%d", f.checkinCalls.Load())
	}
	st, _ := p.Status("u1")
	if st.Cooling {
		t.Errorf("account should be reenabled after checkin with credits: %+v", st)
	}
	if st.Credits != 500 {
		t.Errorf("credits=%d want 500", st.Credits)
	}
	if report.Total != 1 || report.Succeeded != 1 || report.Failed != 0 || len(report.Accounts) != 1 {
		t.Fatalf("report=%+v", report)
	}
}

func TestRefreshCreditsNow(t *testing.T) {
	f := &fakeUpstream{resourceRemain: 321}
	srv := f.server()
	defer srv.Close()
	p := pool.New("")
	p.Add(&auth.Auth{UID: "u1", AccessToken: "at", RefreshToken: "rt", ExpiresAt: 9999999999})
	up := &upstream.Client{HTTP: srv.Client(), ChatBaseCN: srv.URL, BillingBaseCN: srv.URL, ChatBaseGlobal: srv.URL, BillingBaseGlob: srv.URL}
	report := New(Config{Pool: p, Upstream: up}).RefreshCreditsNow()
	status, _ := p.Status("u1")
	if status.Credits != 321 || report.Succeeded != 1 || report.Failed != 0 {
		t.Fatalf("status=%+v report=%+v", status, report)
	}
}

func TestRunKeepaliveRefreshesTokens(t *testing.T) {
	f := &fakeUpstream{}
	srv := f.server()
	defer srv.Close()

	p := pool.New("")
	a := &auth.Auth{UID: "u1", AccessToken: "old", RefreshToken: "rt", ExpiresAt: 1}
	p.Add(a)

	up := &upstream.Client{
		HTTP:            srv.Client(),
		ChatBaseCN:      srv.URL,
		BillingBaseCN:   srv.URL,
		ChatBaseGlobal:  srv.URL,
		BillingBaseGlob: srv.URL,
	}
	s := New(Config{Pool: p, Upstream: up})
	s.RunKeepaliveNow()
	if f.refreshCalls.Load() != 1 {
		t.Errorf("refresh calls=%d", f.refreshCalls.Load())
	}
	if a.AccessToken != "new" {
		t.Errorf("token not updated: %s", a.AccessToken)
	}
}

func TestRunKeepaliveSessionDeadDisables(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"code":12153,"msg":"Offline user session not found"}`))
	}))
	defer srv.Close()

	p := pool.New("")
	a := &auth.Auth{UID: "u1", AccessToken: "old", RefreshToken: "rt", ExpiresAt: 1}
	p.Add(a)

	up := &upstream.Client{
		HTTP:            srv.Client(),
		ChatBaseCN:      srv.URL,
		BillingBaseCN:   srv.URL,
		ChatBaseGlobal:  srv.URL,
		BillingBaseGlob: srv.URL,
	}
	s := New(Config{Pool: p, Upstream: up})
	s.RunKeepaliveNow()
	st, _ := p.Status("u1")
	if !st.Disabled {
		t.Errorf("should disable session-dead account: %+v", st)
	}
}

func TestCheckinErrorDoesNotCrash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`boom`))
	}))
	defer srv.Close()

	p := pool.New("")
	p.Add(&auth.Auth{UID: "u1", AccessToken: "at", RefreshToken: "rt", ExpiresAt: 9999999999})
	up := &upstream.Client{
		HTTP:            srv.Client(),
		ChatBaseCN:      srv.URL,
		BillingBaseCN:   srv.URL,
		ChatBaseGlobal:  srv.URL,
		BillingBaseGlob: srv.URL,
	}
	s := New(Config{Pool: p, Upstream: up})
	s.retryDelays = []time.Duration{0, 0}
	// 不应 panic
	s.RunCheckinNow()
	s.RunKeepaliveNow()
	_ = errors.New("unused")
}

func TestCheckinRecoveryAndReporting(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		body        string
		failures    int
		expired     bool
		wantCalls   int
		wantRefresh int
		wantOK      bool
	}{
		{"server recovers", 500, `{"code":10001,"msg":"签到失败，请稍后重试"}`, 2, false, 3, 0, true},
		{"server remains down", 500, `{"code":10001,"msg":"签到失败，请稍后重试"}`, 10, false, 3, 0, false},
		{"rate limit recovers", 429, `{"code":1,"msg":"try later"}`, 1, false, 2, 0, true},
		{"already checked in", 400, `{"code":10001,"msg":"今天已签到，请明天再来"}`, 10, false, 1, 0, true},
		{"business failure is not already", 400, `{"code":10001,"msg":"签到失败，请稍后重试"}`, 10, false, 1, 0, false},
		{"expired auth refreshed first", 200, `{"code":0}`, 0, true, 1, 1, true},
		{"revoked access token recovered", 401, `<html>Unauthorized</html>`, 1, false, 2, 1, true},
		{"persistent unauthorized bounded", 401, `<html>Unauthorized</html>`, 10, false, 2, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls, refreshes atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/token/refresh"):
					refreshes.Add(1)
					w.Write([]byte(`{"code":0,"data":{"accessToken":"fresh","refreshToken":"rotated","expiresIn":3600}}`))
				case strings.HasSuffix(r.URL.Path, "/daily-checkin"):
					n := calls.Add(1)
					if (tc.expired || refreshes.Load() > 0) && r.Header.Get("Authorization") != "Bearer fresh" {
						t.Error("check-in used stale token after refresh")
					}
					if int(n) <= tc.failures {
						w.WriteHeader(tc.status)
						w.Write([]byte(tc.body))
					} else {
						w.Write([]byte(`{"code":0,"data":{}}`))
					}
				case strings.HasSuffix(r.URL.Path, "/get-user-resource"):
					w.Write([]byte(`{"code":0,"data":{"Response":{"Data":{"Accounts":[{"CycleCapacitySize":100,"CycleCapacityRemain":80}]}}}}`))
				default:
					http.Error(w, "unexpected endpoint", 404)
				}
			}))
			defer srv.Close()
			p := pool.New("")
			a := &auth.Auth{UID: "u1", AccessToken: "old", RefreshToken: "rt", ExpiresAt: 9999999999, FilePath: filepath.Join(t.TempDir(), "auth.json")}
			if tc.expired {
				a.ExpiresAt = 1
			}
			p.Add(a)
			up := &upstream.Client{HTTP: srv.Client(), ChatBaseCN: srv.URL, BillingBaseCN: srv.URL}
			s := New(Config{Pool: p, Upstream: up})
			s.retryDelays = []time.Duration{0, 0}
			report := s.RunCheckinNow()
			if int(calls.Load()) != tc.wantCalls || int(refreshes.Load()) != tc.wantRefresh {
				t.Fatalf("checkin=%d refresh=%d; want %d/%d", calls.Load(), refreshes.Load(), tc.wantCalls, tc.wantRefresh)
			}
			if report.Total != 1 || report.Accounts[0].OK != tc.wantOK || (report.Succeeded == 1) != tc.wantOK || report.Succeeded+report.Failed != 1 {
				t.Fatalf("report=%+v wantOK=%t", report, tc.wantOK)
			}
			if !tc.wantOK && !strings.Contains(report.Accounts[0].Message, "checkin:") {
				t.Fatalf("missing checkin failure: %+v", report)
			}
			// Balance refresh must remain useful without hiding a failed check-in.
			st, _ := p.Status(a.UID)
			if st.Credits != 80 {
				t.Fatalf("credits=%d", st.Credits)
			}
			if tc.wantRefresh > 0 {
				raw, err := os.ReadFile(a.FilePath)
				if err != nil {
					t.Fatal(err)
				}
				saved, err := auth.Parse(raw)
				if err != nil || saved.AccessToken != "fresh" || saved.RefreshToken != "rotated" {
					t.Fatal("refreshed credentials were not persisted")
				}
				info, err := os.Stat(a.FilePath)
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatal("auth file is not owner-only")
				}
			}
		})
	}
}

func TestCheckinCreditFailureDoesNotRepeatCheckin(t *testing.T) {
	var checkins, resources atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/daily-checkin") {
			checkins.Add(1)
			w.Write([]byte(`{"code":0}`))
			return
		}
		resources.Add(1)
		http.Error(w, `{"code":10000,"msg":"API request failed with status code: 500"}`, 500)
	}))
	defer srv.Close()
	p := pool.New("")
	p.Add(&auth.Auth{UID: "u1", AccessToken: "at", RefreshToken: "rt", ExpiresAt: 9999999999})
	s := New(Config{Pool: p, Upstream: &upstream.Client{HTTP: srv.Client(), BillingBaseCN: srv.URL}})
	s.retryDelays = []time.Duration{0, 0}
	report := s.RunCheckinNow()
	if report.Failed != 1 || checkins.Load() != 1 || resources.Load() != 3 || !strings.Contains(report.Accounts[0].Message, "user-resource:") {
		t.Fatalf("report=%+v calls=%d/%d", report, checkins.Load(), resources.Load())
	}
}

func TestScheduledCheckinRecoversTransientFailure(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/daily-checkin") && calls.Add(1) == 1 {
			http.Error(w, "temporary upstream error", 500)
			return
		}
		w.Write([]byte(`{"code":0,"data":{}}`))
	}))
	defer srv.Close()
	p := pool.New("")
	p.Add(&auth.Auth{UID: "u1", AccessToken: "at", RefreshToken: "rt", ExpiresAt: 9999999999})
	s := New(Config{Pool: p, Upstream: &upstream.Client{HTTP: srv.Client(), BillingBaseCN: srv.URL}})
	s.retryDelays = []time.Duration{0, 0}
	// Dispatch a historical slot to cover a late timer wakeup as well as the scheduled path.
	s.runDue(context.Background(), time.Date(2026, 10, 18, 9, 0, 0, 0, time.Local))
	if calls.Load() != 2 {
		t.Fatalf("checkin calls=%d want 2", calls.Load())
	}
}

func TestRetryCancellation(t *testing.T) {
	s := New(Config{})
	s.retryDelays = []time.Duration{time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err := s.retry(ctx, "checkin", "u1", func() error {
		calls++
		cancel()
		return &upstream.Error{Kind: upstream.ErrServer, Status: 500}
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestCheckinRefreshFailureStopsBilling(t *testing.T) {
	for _, sessionDead := range []bool{false, true} {
		t.Run(fmt.Sprint("session_dead=", sessionDead), func(t *testing.T) {
			var billingCalls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/token/refresh") {
					if sessionDead {
						http.Error(w, `{"code":12153,"msg":"Offline user session not found"}`, 401)
					} else {
						w.Write([]byte(`{"code":0,"data":{"accessToken":"fresh","expiresIn":3600}}`))
					}
					return
				}
				billingCalls.Add(1)
				w.Write([]byte(`{"code":0}`))
			}))
			defer srv.Close()
			p := pool.New("")
			// A directory cannot be replaced by the auth file: deterministic persistence failure.
			a := &auth.Auth{UID: "u1", AccessToken: "old", RefreshToken: "rt", ExpiresAt: 1, FilePath: t.TempDir()}
			p.Add(a)
			s := New(Config{Pool: p, Upstream: &upstream.Client{HTTP: srv.Client(), ChatBaseCN: srv.URL, BillingBaseCN: srv.URL}})
			s.retryDelays = []time.Duration{0, 0}
			report := s.RunCheckinNow()
			if report.Failed != 1 || report.Succeeded != 0 || billingCalls.Load() != 0 {
				t.Fatalf("report=%+v billing=%d", report, billingCalls.Load())
			}
			st, _ := p.Status(a.UID)
			if st.Disabled != sessionDead {
				t.Fatalf("disabled=%t", st.Disabled)
			}
			_ = os.Remove(a.FilePath + ".tmp")
		})
	}
}
