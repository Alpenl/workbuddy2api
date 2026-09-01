package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbuddy2api/internal/auth"
	"workbuddy2api/internal/oauth"
	"workbuddy2api/internal/pool"
	"workbuddy2api/internal/scheduler"
	"workbuddy2api/internal/upstream"
)

type fakeOAuthClient struct {
	start oauth.State
	polls int
}

func (f *fakeOAuthClient) Start(context.Context) (oauth.State, error) {
	return f.start, nil
}

func (f *fakeOAuthClient) Poll(context.Context, string) (*auth.Auth, error) {
	f.polls++
	if f.polls == 1 {
		return nil, oauth.ErrPending
	}
	return &auth.Auth{
		UID:          "user-1",
		Nickname:     "Alpen",
		EnterpriseID: "enterprise-1",
		AccessToken:  "private-access-token",
		RefreshToken: "private-refresh-token",
		ExpiresAt:    9999999999,
	}, nil
}

type fakeMaintenance struct {
	checkinCalls int
	creditCalls  int
}

func (f *fakeMaintenance) RunCheckinNow() scheduler.OperationReport {
	f.checkinCalls++
	return scheduler.OperationReport{Total: 1, Succeeded: 1, Accounts: []scheduler.AccountResult{{UID: "user-1", OK: true, Credits: 90}}}
}

func (f *fakeMaintenance) RefreshCreditsNow() scheduler.OperationReport {
	f.creditCalls++
	return scheduler.OperationReport{Total: 1, Succeeded: 1, Accounts: []scheduler.AccountResult{{UID: "user-1", OK: true, Credits: 80}}}
}

func adminUpstream() *upstream.Client {
	return &upstream.Client{
		HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body := `{"code":0,"data":{}}`
			if strings.HasSuffix(r.URL.Path, "/get-user-resource") {
				body = `{"code":0,"data":{"Response":{"Data":{"Accounts":[{"CycleCapacitySize":100,"CycleCapacityRemain":77}]}}}}`
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})},
		ChatBaseCN:      "https://fake.example",
		BillingBaseCN:   "https://fake.example",
		ChatBaseGlobal:  "https://fake.example",
		BillingBaseGlob: "https://fake.example",
	}
}

func authenticatedRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestOAuthAdminFlowPersistsAndAddsAccount(t *testing.T) {
	authDir := t.TempDir()
	p := pool.New("")
	p.Add(&auth.Auth{UID: "user-1", AccessToken: "expired"})
	p.Disable("user-1", "session dead")
	oauthClient := &fakeOAuthClient{start: oauth.State{State: "upstream-state", AuthURL: "https://login.example/authorize"}}
	h := NewHandler(Config{
		Pool:     p,
		Upstream: adminUpstream(),
		OAuth:    oauthClient,
		AuthDir:  authDir,
		Region:   "cn",
		APIKey:   "secret",
	})

	unauthorized := httptest.NewRecorder()
	h.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/api/admin/oauth/start", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized code=%d", unauthorized.Code)
	}

	startRecorder := httptest.NewRecorder()
	h.ServeHTTP(startRecorder, authenticatedRequest(http.MethodPost, "/api/admin/oauth/start", ""))
	if startRecorder.Code != http.StatusOK {
		t.Fatalf("start code=%d body=%s", startRecorder.Code, startRecorder.Body)
	}
	var started struct {
		FlowID  string `json:"flow_id"`
		AuthURL string `json:"auth_url"`
	}
	if err := json.Unmarshal(startRecorder.Body.Bytes(), &started); err != nil {
		t.Fatalf("start response: %v", err)
	}
	if started.FlowID == "" || started.AuthURL != "https://login.example/authorize" {
		t.Fatalf("start=%+v", started)
	}

	pollBody := `{"flow_id":"` + started.FlowID + `"}`
	pendingRecorder := httptest.NewRecorder()
	h.ServeHTTP(pendingRecorder, authenticatedRequest(http.MethodPost, "/api/admin/oauth/poll", pollBody))
	if pendingRecorder.Code != http.StatusAccepted {
		t.Fatalf("pending code=%d body=%s", pendingRecorder.Code, pendingRecorder.Body)
	}

	completedRecorder := httptest.NewRecorder()
	h.ServeHTTP(completedRecorder, authenticatedRequest(http.MethodPost, "/api/admin/oauth/poll", pollBody))
	if completedRecorder.Code != http.StatusOK {
		t.Fatalf("completed code=%d body=%s", completedRecorder.Code, completedRecorder.Body)
	}
	responseBody := completedRecorder.Body.String()
	if strings.Contains(responseBody, "private-access-token") || strings.Contains(responseBody, "private-refresh-token") {
		t.Fatalf("OAuth tokens leaked in response: %s", responseBody)
	}
	status, ok := p.Status("user-1")
	if !ok || status.Disabled || status.Credits != 77 || status.Nickname != "Alpen" {
		t.Fatalf("status=%+v ok=%v", status, ok)
	}
	credentialPath := filepath.Join(authDir, "workbuddy-user-1.json")
	info, err := os.Stat(credentialPath)
	if err != nil {
		t.Fatalf("credential stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credential mode=%#o", info.Mode().Perm())
	}
	raw, err := os.ReadFile(credentialPath)
	if err != nil {
		t.Fatalf("credential read: %v", err)
	}
	parsed, err := auth.Parse(raw)
	if err != nil || parsed.AccessToken != "private-access-token" {
		t.Fatalf("saved credential=%+v err=%v", parsed, err)
	}
}

func TestMaintenanceAdminEndpoints(t *testing.T) {
	maintenance := &fakeMaintenance{}
	h := NewHandler(Config{
		Pool:        pool.New(""),
		Upstream:    upstream.New(),
		Maintenance: maintenance,
		APIKey:      "secret",
	})

	for _, endpoint := range []string{"/api/admin/checkin", "/api/admin/credits/refresh"} {
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, authenticatedRequest(http.MethodPost, endpoint, ""))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s code=%d body=%s", endpoint, recorder.Code, recorder.Body)
		}
		if !strings.Contains(recorder.Body.String(), `"succeeded":1`) {
			t.Fatalf("%s body=%s", endpoint, recorder.Body)
		}
	}
	if maintenance.checkinCalls != 1 || maintenance.creditCalls != 1 {
		t.Fatalf("calls checkin=%d credits=%d", maintenance.checkinCalls, maintenance.creditCalls)
	}
}

func TestOAuthRejectsUnsafeUID(t *testing.T) {
	client := &fakeOAuthClient{start: oauth.State{State: "state", AuthURL: "https://login.example"}, polls: 1}
	h := NewHandler(Config{Pool: pool.New(""), Upstream: adminUpstream(), OAuth: client, AuthDir: t.TempDir(), Region: "cn"})
	client.polls = 1
	// Override the successful account by using a tiny wrapper.
	h.cfg.OAuth = oauthClientFunc(func(context.Context, string) (*auth.Auth, error) {
		return &auth.Auth{UID: "../escape", AccessToken: "access"}, nil
	})
	start := httptest.NewRecorder()
	h.ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/api/admin/oauth/start", nil))
	var body struct {
		FlowID string `json:"flow_id"`
	}
	_ = json.Unmarshal(start.Body.Bytes(), &body)
	poll := httptest.NewRecorder()
	h.ServeHTTP(poll, httptest.NewRequest(http.MethodPost, "/api/admin/oauth/poll", strings.NewReader(`{"flow_id":"`+body.FlowID+`"}`)))
	if poll.Code != http.StatusBadGateway {
		t.Fatalf("code=%d body=%s", poll.Code, poll.Body)
	}
}

type oauthClientFunc func(context.Context, string) (*auth.Auth, error)

func (f oauthClientFunc) Start(context.Context) (oauth.State, error) {
	return oauth.State{State: "state", AuthURL: "https://login.example"}, nil
}

func (f oauthClientFunc) Poll(ctx context.Context, state string) (*auth.Auth, error) {
	return f(ctx, state)
}
