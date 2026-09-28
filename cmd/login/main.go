package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const clientID = "app_EMoamEEZ73f0CkXaXp7hrann"
const authURL = "https://auth.openai.com"

type deviceCode struct {
	ID       string
	UserCode string
	Interval time.Duration
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	if err := login(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func login(ctx context.Context) error {
	device, err := startDeviceCode(ctx)
	if err != nil {
		return err
	}
	fmt.Println("Open https://auth.openai.com/codex/device and enter code:", device.UserCode)
	code, verifier, err := waitForCode(ctx, device)
	if err != nil {
		return err
	}
	credentials, err := exchangeCode(ctx, code, verifier)
	if err != nil {
		return err
	}
	path := os.Getenv("CODEX_AUTH_FILE")
	if path == "" {
		path = ".state/codex-auth.json"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create login directory: %w", err)
	}
	data, err := json.Marshal(credentials)
	if err != nil {
		return fmt.Errorf("encode login: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("save login: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("protect login: %w", err)
	}
	fmt.Println("Saved ChatGPT login to", path)
	return nil
}

func startDeviceCode(ctx context.Context) (deviceCode, error) {
	body, err := json.Marshal(map[string]string{"client_id": clientID})
	if err != nil {
		return deviceCode{}, err
	}
	response, err := postJSON(ctx, authURL+"/api/accounts/deviceauth/usercode", body)
	if err != nil {
		return deviceCode{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return deviceCode{}, fmt.Errorf("start device login: %s", response.Status)
	}
	var result struct {
		ID       string          `json:"device_auth_id"`
		UserCode string          `json:"user_code"`
		Interval json.RawMessage `json:"interval"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return deviceCode{}, fmt.Errorf("decode device login: %w", err)
	}
	if result.ID == "" || result.UserCode == "" {
		return deviceCode{}, errors.New("device login response is incomplete")
	}
	var seconds int
	if err := json.Unmarshal(result.Interval, &seconds); err != nil {
		var raw string
		if err := json.Unmarshal(result.Interval, &raw); err != nil {
			return deviceCode{}, errors.New("device login interval is invalid")
		}
		seconds, err = strconv.Atoi(raw)
		if err != nil {
			return deviceCode{}, fmt.Errorf("parse device login interval: %w", err)
		}
	}
	if seconds < 1 {
		seconds = 5
	}
	return deviceCode{ID: result.ID, UserCode: result.UserCode, Interval: time.Duration(seconds) * time.Second}, nil
}

func waitForCode(ctx context.Context, device deviceCode) (string, string, error) {
	for {
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-time.After(device.Interval):
		}
		body, err := json.Marshal(map[string]string{"device_auth_id": device.ID, "user_code": device.UserCode})
		if err != nil {
			return "", "", err
		}
		response, err := postJSON(ctx, authURL+"/api/accounts/deviceauth/token", body)
		if err != nil {
			return "", "", err
		}
		if response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusNotFound {
			response.Body.Close()
			continue
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return "", "", fmt.Errorf("wait for device login: %s", response.Status)
		}
		var result struct {
			Code     string `json:"authorization_code"`
			Verifier string `json:"code_verifier"`
		}
		err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result)
		response.Body.Close()
		if err != nil {
			return "", "", fmt.Errorf("decode device authorization: %w", err)
		}
		if result.Code == "" || result.Verifier == "" {
			return "", "", errors.New("device authorization is incomplete")
		}
		return result.Code, result.Verifier, nil
	}
}

func exchangeCode(ctx context.Context, code, verifier string) (any, error) {
	values := "grant_type=authorization_code&client_id=" + clientID + "&code=" + url.QueryEscape(code) + "&code_verifier=" + url.QueryEscape(verifier) + "&redirect_uri=" + url.QueryEscape(authURL+"/deviceauth/callback")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL+"/oauth/token", strings.NewReader(values))
	if err != nil {
		return nil, fmt.Errorf("prepare token exchange: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("exchange login: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exchange login: %s", response.Status)
	}
	var tokens struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		ID      string `json:"id_token"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&tokens); err != nil {
		return nil, fmt.Errorf("decode login tokens: %w", err)
	}
	if tokens.Access == "" || tokens.Refresh == "" {
		return nil, errors.New("login tokens are incomplete")
	}
	accountID, err := accountID(tokens.Access)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"auth_mode":    "chatgpt",
		"last_refresh": time.Now().UTC(),
		"tokens": map[string]string{
			"access_token":  tokens.Access,
			"refresh_token": tokens.Refresh,
			"id_token":      tokens.ID,
			"account_id":    accountID,
		},
	}, nil
}

func accountID(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("login access token is invalid")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode login access token: %w", err)
	}
	var claims struct {
		Auth struct {
			AccountID string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("decode login account: %w", err)
	}
	if claims.Auth.AccountID == "" {
		return "", errors.New("login account ID is missing")
	}
	return claims.Auth.AccountID, nil
}

func postJSON(ctx context.Context, endpoint string, body []byte) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("prepare device login: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request device login: %w", err)
	}
	return response, nil
}
