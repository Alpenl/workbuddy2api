package oauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStartAndPoll(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v2/plugin/auth/state", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("platform") != "CLI" {
			t.Fatalf("platform=%q", r.URL.Query().Get("platform"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"state":"state / one","authUrl":"https://login.example/authorize"}}`))
	})
	mux.HandleFunc("GET /v2/plugin/auth/token", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != "state / one" {
			t.Fatalf("state=%q", r.URL.Query().Get("state"))
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"accessToken":"access","refreshToken":"refresh","expiresIn":3600,"domain":""}}`))
	})
	mux.HandleFunc("GET /v2/plugin/login/account", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"uid":"user-1","enterpriseId":"enterprise-1","nickname":"Alpen"}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient()
	client.BaseURL = server.URL
	state, err := client.Start(context.Background())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if state.State != "state / one" || state.AuthURL != "https://login.example/authorize" {
		t.Fatalf("state=%+v", state)
	}
	account, err := client.Poll(context.Background(), state.State)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if account.UID != "user-1" || account.Nickname != "Alpen" || account.AccessToken != "access" {
		t.Fatalf("account=%+v", account)
	}
	if account.ExpiresAt <= 0 {
		t.Fatal("ExpiresAt was not populated")
	}
}

func TestPollPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":10001,"msg":"login ing","data":null}`))
	}))
	defer server.Close()
	client := NewClient()
	client.BaseURL = server.URL
	_, err := client.Poll(context.Background(), "pending-state")
	if !errors.Is(err, ErrPending) {
		t.Fatalf("err=%v want ErrPending", err)
	}
}
