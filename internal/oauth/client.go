// Package oauth implements the existing WorkBuddy CN device authorization flow.
package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"workbuddy2api/internal/auth"
)

const (
	defaultBaseURL = "https://copilot.tencent.com"
	defaultOrigin  = "https://www.codebuddy.cn"
	clientUA       = "CLI/2.63.2 CodeBuddy/2.63.2"
)

// ErrPending means the user has not completed browser authorization yet.
var ErrPending = errors.New("oauth authorization pending")

// State is returned when a device authorization flow starts.
type State struct {
	State   string `json:"state"`
	AuthURL string `json:"auth_url"`
}

// Client calls the WorkBuddy OAuth endpoints. BaseURL and Origin are exported
// so tests can point the client at an httptest server.
type Client struct {
	HTTP    *http.Client
	BaseURL string
	Origin  string
}

// NewClient returns a production OAuth client.
func NewClient() *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		BaseURL: defaultBaseURL,
		Origin:  defaultOrigin,
	}
}

type apiEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type apiError struct {
	Status int
	Code   int
	Msg    string
}

func (e *apiError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("oauth upstream code=%d: %s", e.Code, e.Msg)
	}
	return fmt.Sprintf("oauth upstream http %d: %s", e.Status, e.Msg)
}

func (c *Client) endpoint(path string) string {
	return strings.TrimRight(c.BaseURL, "/") + path
}

func (c *Client) addHeaders(req *http.Request) {
	origin := strings.TrimRight(c.Origin, "/")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", origin+"/")
	req.Header.Set("User-Agent", clientUA)
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, headers func(*http.Request), body io.Reader) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	if headers == nil {
		c.addHeaders(req)
	} else {
		headers(req)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var env apiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("oauth response parse failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || env.Code != 0 {
		return nil, &apiError{Status: resp.StatusCode, Code: env.Code, Msg: env.Msg}
	}
	return env.Data, nil
}

// Start creates a CN device authorization state and returns its browser URL.
func (c *Client) Start(ctx context.Context) (State, error) {
	data, err := c.doJSON(ctx, http.MethodPost, c.endpoint("/v2/plugin/auth/state?platform=CLI"), nil, bytes.NewReader([]byte("{}")))
	if err != nil {
		return State{}, fmt.Errorf("start oauth: %w", err)
	}
	var response struct {
		State   string `json:"state"`
		AuthURL string `json:"authUrl"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return State{}, fmt.Errorf("parse oauth state: %w", err)
	}
	if strings.TrimSpace(response.State) == "" || strings.TrimSpace(response.AuthURL) == "" {
		return State{}, errors.New("oauth state response is missing state or authUrl")
	}
	return State{State: response.State, AuthURL: response.AuthURL}, nil
}

// Poll checks a device authorization state once and returns the account when complete.
func (c *Client) Poll(ctx context.Context, state string) (*auth.Auth, error) {
	state = strings.TrimSpace(state)
	if state == "" {
		return nil, errors.New("oauth state is empty")
	}
	tokenURL := c.endpoint("/v2/plugin/auth/token?state=" + url.QueryEscape(state))
	tokenData, err := c.doJSON(ctx, http.MethodGet, tokenURL, nil, nil)
	if err != nil {
		var upstreamErr *apiError
		if errors.As(err, &upstreamErr) && upstreamErr.Status < http.StatusInternalServerError {
			return nil, ErrPending
		}
		return nil, fmt.Errorf("poll oauth token: %w", err)
	}
	var token struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    int64  `json:"expiresIn"`
		Domain       string `json:"domain"`
	}
	if err := json.Unmarshal(tokenData, &token); err != nil {
		return nil, fmt.Errorf("parse oauth token: %w", err)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return nil, ErrPending
	}

	accountURL := c.endpoint("/v2/plugin/login/account?state=" + url.QueryEscape(state))
	headers := func(req *http.Request) {
		c.addHeaders(req)
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	}
	accountData, err := c.doJSON(ctx, http.MethodGet, accountURL, headers, nil)
	if err != nil {
		return nil, fmt.Errorf("load oauth account: %w", err)
	}
	var account struct {
		UID          string `json:"uid"`
		EnterpriseID string `json:"enterpriseId"`
		Nickname     string `json:"nickname"`
	}
	if err := json.Unmarshal(accountData, &account); err != nil {
		return nil, fmt.Errorf("parse oauth account: %w", err)
	}
	if strings.TrimSpace(account.UID) == "" {
		return nil, errors.New("oauth account response is missing uid")
	}

	expiresAt := int64(0)
	if token.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).Unix()
	}
	return &auth.Auth{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    expiresAt,
		Domain:       token.Domain,
		UID:          account.UID,
		EnterpriseID: account.EnterpriseID,
		Nickname:     account.Nickname,
	}, nil
}
