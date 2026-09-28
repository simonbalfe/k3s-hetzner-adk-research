package research

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const serviceAccountPath = "/var/run/secrets/kubernetes.io/serviceaccount"
const codexClientID = "app_EMoamEEZ73f0CkXaXp7hrann"

type codexCredentials struct {
	AuthMode    string    `json:"auth_mode"`
	LastRefresh time.Time `json:"last_refresh"`
	Tokens      struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		AccountID    string `json:"account_id"`
		IDToken      string `json:"id_token"`
	} `json:"tokens"`
}

func accessToken(ctx context.Context, forceRefresh bool) (string, string, error) {
	path := filepath.Join(os.Getenv("CODEX_HOME"), "auth.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read Codex login: %w", err)
	}
	var credentials codexCredentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		return "", "", fmt.Errorf("decode Codex login: %w", err)
	}
	if credentials.AuthMode != "chatgpt" || credentials.Tokens.RefreshToken == "" || credentials.Tokens.AccountID == "" {
		return "", "", errors.New("Codex login is not a refreshable ChatGPT session")
	}
	if !forceRefresh && tokenValid(credentials.Tokens.AccessToken) {
		return credentials.Tokens.AccessToken, credentials.Tokens.AccountID, nil
	}
	if err := refreshToken(ctx, &credentials); err != nil {
		return "", "", err
	}
	updated, err := json.Marshal(credentials)
	if err != nil {
		return "", "", fmt.Errorf("encode refreshed login: %w", err)
	}
	if err := os.WriteFile(path, updated, 0600); err != nil {
		return "", "", fmt.Errorf("save refreshed login: %w", err)
	}
	if err := persistAuth(ctx); err != nil {
		return "", "", err
	}
	return credentials.Tokens.AccessToken, credentials.Tokens.AccountID, nil
}

func tokenValid(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		Expires int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return false
	}
	return claims.Expires > time.Now().Add(time.Minute).Unix()
}

func refreshToken(ctx context.Context, credentials *codexCredentials) error {
	values := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {credentials.Tokens.RefreshToken},
		"client_id":     {codexClientID},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://auth.openai.com/oauth/token", strings.NewReader(values.Encode()))
	if err != nil {
		return fmt.Errorf("prepare login refresh: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("refresh Codex login: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh Codex login: authentication returned %s", response.Status)
	}
	var tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&tokens); err != nil {
		return fmt.Errorf("decode refreshed login: %w", err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		return errors.New("refreshed login is missing tokens")
	}
	credentials.Tokens.AccessToken = tokens.AccessToken
	credentials.Tokens.RefreshToken = tokens.RefreshToken
	if tokens.IDToken != "" {
		credentials.Tokens.IDToken = tokens.IDToken
	}
	credentials.LastRefresh = time.Now().UTC()
	return nil
}

func persistAuth(ctx context.Context) error {
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	if host == "" {
		return nil
	}
	port := os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS")
	if port == "" {
		port = "443"
	}
	auth, err := os.ReadFile(filepath.Join(os.Getenv("CODEX_HOME"), "auth.json"))
	if err != nil {
		return fmt.Errorf("read Codex login: %w", err)
	}
	var credentials struct {
		AuthMode string `json:"auth_mode"`
		Tokens   struct {
			RefreshToken string `json:"refresh_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(auth, &credentials); err != nil {
		return fmt.Errorf("decode Codex login: %w", err)
	}
	if credentials.AuthMode != "chatgpt" || credentials.Tokens.RefreshToken == "" {
		return errors.New("Codex login is not a refreshable ChatGPT session")
	}
	token, err := os.ReadFile(filepath.Join(serviceAccountPath, "token"))
	if err != nil {
		return fmt.Errorf("read service account token: %w", err)
	}
	namespace, err := os.ReadFile(filepath.Join(serviceAccountPath, "namespace"))
	if err != nil {
		return fmt.Errorf("read namespace: %w", err)
	}
	certificate, err := os.ReadFile(filepath.Join(serviceAccountPath, "ca.crt"))
	if err != nil {
		return fmt.Errorf("read Kubernetes CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certificate) {
		return errors.New("Kubernetes CA is invalid")
	}
	patch, err := json.Marshal(map[string]map[string]string{
		"data": {"auth.json": base64.StdEncoding.EncodeToString(auth)},
	})
	if err != nil {
		return fmt.Errorf("encode login update: %w", err)
	}
	endpoint := url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(host, port),
		Path:   "/api/v1/namespaces/" + url.PathEscape(strings.TrimSpace(string(namespace))) + "/secrets/codex-auth",
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(saveCtx, http.MethodPatch, endpoint.String(), bytes.NewReader(patch))
	if err != nil {
		return fmt.Errorf("prepare login update: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	request.Header.Set("Content-Type", "application/merge-patch+json")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: pool}
	client := &http.Client{Transport: transport}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("update Codex login: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("update Codex login: Kubernetes returned %s", response.Status)
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		return fmt.Errorf("finish login update: %w", err)
	}
	return nil
}
