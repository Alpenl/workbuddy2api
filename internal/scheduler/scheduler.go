// Package scheduler 定时任务：每日签到（09/21点）+ token keepalive（22点）。
// 签到成功后重新查余额，余额 > 0 的冷却账号自动解冻。
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"workbuddy2api/internal/auth"
	"workbuddy2api/internal/pool"
	"workbuddy2api/internal/upstream"
)

// Config 调度器依赖。
type Config struct {
	Pool           *pool.Pool
	Upstream       *upstream.Client
	CheckinHours   []int // 默认 [9, 21]
	KeepaliveHours []int // 默认 [22]
}

// Scheduler 调度器。
type Scheduler struct {
	cfg         Config
	mu          sync.Mutex
	retryDelays []time.Duration
}

// AccountResult is a sanitized result for a maintenance operation.
type AccountResult struct {
	UID      string `json:"uid"`
	Nickname string `json:"nickname,omitempty"`
	Credits  int64  `json:"credits"`
	OK       bool   `json:"ok"`
	Message  string `json:"message,omitempty"`
}

// OperationReport summarizes a manual or scheduled account operation.
type OperationReport struct {
	Total     int             `json:"total"`
	Succeeded int             `json:"succeeded"`
	Failed    int             `json:"failed"`
	Accounts  []AccountResult `json:"accounts"`
}

// New 构建。
func New(cfg Config) *Scheduler {
	if len(cfg.CheckinHours) == 0 {
		cfg.CheckinHours = []int{9, 21}
	}
	if len(cfg.KeepaliveHours) == 0 {
		cfg.KeepaliveHours = []int{22}
	}
	return &Scheduler{cfg: cfg, retryDelays: []time.Duration{5 * time.Second, 30 * time.Second}}
}

// nextFire 返回 now 之后最近的一个整点触发时间；hours 为本地小时（0-23）。
func nextFire(now time.Time, hours []int) time.Time {
	var earliest time.Time
	for _, h := range hours {
		t := time.Date(now.Year(), now.Month(), now.Day(), h, 0, 0, 0, now.Location())
		if !t.After(now) {
			t = t.Add(24 * time.Hour)
		}
		if earliest.IsZero() || t.Before(earliest) {
			earliest = t
		}
	}
	return earliest
}

// Run 主循环，阻塞直到 ctx 取消。
func (s *Scheduler) Run(ctx context.Context) {
	all := append(append([]int{}, s.cfg.CheckinHours...), s.cfg.KeepaliveHours...)
	for {
		next := nextFire(time.Now(), all)
		log.Printf("scheduler next=%s checkin_hours=%v keepalive_hours=%v", next.Format(time.RFC3339), s.cfg.CheckinHours, s.cfg.KeepaliveHours)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.runDue(ctx, next)
		}
	}
}

// Dispatch the scheduled slot even if the timer wakes late.
func (s *Scheduler) runDue(ctx context.Context, scheduled time.Time) {
	if contains(s.cfg.CheckinHours, scheduled.Hour()) {
		s.runCheckin(ctx, "scheduled")
	}
	if ctx.Err() == nil && contains(s.cfg.KeepaliveHours, scheduled.Hour()) {
		s.RunKeepaliveNow()
	}
}

func contains(hours []int, h int) bool {
	for _, v := range hours {
		if v == h {
			return true
		}
	}
	return false
}

// RunCheckinNow 立即对所有账号执行签到 + 余额刷新 + 解冻。
// 冷却中的账号也参与（签到就是为了解冻它们）；禁用的跳过。
func (s *Scheduler) RunCheckinNow() OperationReport {
	return s.runCheckin(context.Background(), "manual")
}

func (s *Scheduler) runCheckin(ctx context.Context, source string) OperationReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	report := OperationReport{Accounts: []AccountResult{}}
	log.Printf("checkin source=%s started", source)
	defer func() {
		log.Printf("checkin source=%s completed total=%d succeeded=%d failed=%d", source, report.Total, report.Succeeded, report.Failed)
	}()
	for _, st := range s.cfg.Pool.List() {
		if st.Disabled {
			continue
		}
		a := s.cfg.Pool.AuthByUID(st.UID)
		if a == nil || a.RefreshToken == "" {
			continue
		}
		report.Total++
		result := AccountResult{UID: st.UID, Nickname: st.Nickname, Credits: st.Credits}
		if err := s.prepareAuth(ctx, a); err != nil {
			result.Message = "refresh: " + err.Error()
			log.Printf("checkin %s: %s", st.UID, result.Message)
			report.Failed++
			report.Accounts = append(report.Accounts, result)
			continue
		}
		checkinErr := s.billing(ctx, a, "checkin", func() error {
			err := s.cfg.Upstream.DailyCheckin(a)
			if alreadyCheckedIn(err) {
				return nil
			}
			return err
		})
		if checkinErr != nil {
			log.Printf("checkin %s: %v", st.UID, checkinErr)
			result.Message = "checkin: " + checkinErr.Error()
		}
		var remain int64
		creditErr := s.billing(ctx, a, "user-resource", func() error {
			var err error
			remain, err = s.cfg.Upstream.UserResource(a)
			return err
		})
		if creditErr != nil {
			log.Printf("user-resource %s: %v", st.UID, creditErr)
			if result.Message != "" {
				result.Message += "; "
			}
			result.Message += "user-resource: " + creditErr.Error()
		} else {
			s.cfg.Pool.ReenableIfCredits(st.UID, remain)
			result.Credits = remain
		}
		result.OK = checkinErr == nil && creditErr == nil
		if result.OK {
			report.Succeeded++
		} else {
			report.Failed++
		}
		log.Printf("checkin %s: ok=%t credits=%d", st.UID, result.OK, result.Credits)
		report.Accounts = append(report.Accounts, result)
	}
	return report
}

// The same upstream business code (10001) also means "check-in failed".
// Only an explicit already-checked-in message is an idempotent success.
func alreadyCheckedIn(err error) bool {
	var ue *upstream.Error
	if !errors.As(err, &ue) || ue.Kind != upstream.ErrClient || (ue.Status != 200 && ue.Status != 400 && ue.Status != 409) {
		return false
	}
	msg := strings.ToLower(ue.Msg)
	return strings.Contains(msg, "今天已签到") || strings.Contains(msg, "今日已签到") ||
		strings.Contains(msg, "already checked in") || strings.Contains(msg, "already checked-in")
}

func (s *Scheduler) refreshAuth(a *auth.Auth) error {
	if err := s.cfg.Upstream.RefreshToken(a); err != nil {
		var ue *upstream.Error
		if errors.As(err, &ue) && ue.Kind == upstream.ErrSessionDead {
			s.cfg.Pool.Disable(a.UID, "refresh session dead")
		}
		return err
	}
	if err := a.SaveAtomic(); err != nil {
		return fmt.Errorf("save refreshed auth: %w", err)
	}
	return nil
}

func (s *Scheduler) prepareAuth(ctx context.Context, a *auth.Auth) error {
	if !a.NeedsRefresh(2 * time.Minute) {
		return nil
	}
	return s.retry(ctx, "refresh", a.UID, func() error { return s.refreshAuth(a) })
}

func (s *Scheduler) billing(ctx context.Context, a *auth.Auth, operation string, call func() error) error {
	refreshed := false
	return s.retry(ctx, operation, a.UID, func() error {
		err := call()
		var ue *upstream.Error
		if !refreshed && errors.As(err, &ue) && ue.Status == http.StatusUnauthorized && ue.Kind != upstream.ErrSessionDead && a.RefreshToken != "" {
			refreshed = true
			log.Printf("%s %s: unauthorized; refreshing auth once", operation, a.UID)
			if refreshErr := s.refreshAuth(a); refreshErr != nil {
				return fmt.Errorf("refresh: %w", refreshErr)
			}
			return call()
		}
		return err
	})
}

func (s *Scheduler) retry(ctx context.Context, operation, uid string, call func() error) error {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := call()
		if err == nil || attempt >= len(s.retryDelays) || !transient(err) {
			return err
		}
		delay := s.retryDelays[attempt]
		log.Printf("%s %s: attempt=%d failed: %v; retry_in=%s", operation, uid, attempt+1, err, delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func transient(err error) bool {
	var ue *upstream.Error
	if errors.As(err, &ue) {
		return ue.Kind != upstream.ErrSessionDead && (ue.Status == http.StatusTooManyRequests || ue.Status >= 500)
	}
	var ne net.Error
	return errors.As(err, &ne)
}

// RefreshCreditsNow refreshes balances without performing a check-in.
func (s *Scheduler) RefreshCreditsNow() OperationReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	report := OperationReport{Accounts: []AccountResult{}}
	for _, st := range s.cfg.Pool.List() {
		if st.Disabled {
			continue
		}
		a := s.cfg.Pool.AuthByUID(st.UID)
		if a == nil || a.AccessToken == "" {
			continue
		}
		report.Total++
		result := AccountResult{UID: st.UID, Nickname: st.Nickname, Credits: st.Credits}
		remain, err := s.cfg.Upstream.UserResource(a)
		if err != nil {
			result.Message = err.Error()
			report.Failed++
			report.Accounts = append(report.Accounts, result)
			continue
		}
		s.cfg.Pool.ReenableIfCredits(st.UID, remain)
		result.Credits = remain
		result.OK = true
		report.Succeeded++
		report.Accounts = append(report.Accounts, result)
	}
	return report
}

// RunKeepaliveNow 立即对所有账号刷新 token；session 死亡的自动禁用。
func (s *Scheduler) RunKeepaliveNow() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, st := range s.cfg.Pool.List() {
		if st.Disabled {
			continue
		}
		a := s.cfg.Pool.AuthByUID(st.UID)
		if a == nil || a.RefreshToken == "" {
			continue
		}
		if err := s.cfg.Upstream.RefreshToken(a); err != nil {
			log.Printf("keepalive %s: %v", st.UID, err)
			var ue *upstream.Error
			if errors.As(err, &ue) && ue.Kind == upstream.ErrSessionDead {
				s.cfg.Pool.Disable(st.UID, "12153 session dead")
			}
			continue
		}
		if err := a.SaveAtomic(); err != nil {
			log.Printf("keepalive %s save: %v", st.UID, err)
		}
	}
}
