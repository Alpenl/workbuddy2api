// login.go exposes the existing WorkBuddy CN OAuth flow as a small CLI.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"workbuddy2api/internal/oauth"
)

const stateFile = "/tmp/wb2api-login-state.json"

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "login: "+format+"\n", args...)
	os.Exit(1)
}

type loginState struct {
	State string `json:"state"`
}

func main() {
	if len(os.Args) < 2 {
		fatal("usage: login <url|poll>")
	}
	client := oauth.NewClient()
	ctx := context.Background()

	switch os.Args[1] {
	case "url":
		state, err := client.Start(ctx)
		if err != nil {
			fatal("auth state failed: %v", err)
		}
		raw, _ := json.Marshal(loginState{State: state.State})
		if err := os.WriteFile(stateFile, raw, 0o600); err != nil {
			fatal("write state: %v", err)
		}
		fmt.Println(state.AuthURL)

	case "poll":
		raw, err := os.ReadFile(stateFile)
		if err != nil {
			fatal("read state: %v (先跑 login url)", err)
		}
		var ls loginState
		if err := json.Unmarshal(raw, &ls); err != nil {
			fatal("parse state: %v", err)
		}
		acct, err := client.Poll(ctx, ls.State)
		if err != nil {
			fatal("登录未完成（waiting for login）。请确认已在浏览器完成登录再按 y: %v", err)
		}
		out := map[string]any{
			"access_token":  acct.AccessToken,
			"refresh_token": acct.RefreshToken,
			"expires_at":    acct.ExpiresAt,
			"domain":        acct.Domain,
			"uid":           acct.UID,
			"enterprise_id": acct.EnterpriseID,
			"nickname":      acct.Nickname,
		}
		oraw, _ := json.Marshal(out)
		fmt.Println(string(oraw))
		os.Remove(stateFile)

	default:
		fatal("unknown subcommand %q (want url|poll)", os.Args[1])
	}
}
