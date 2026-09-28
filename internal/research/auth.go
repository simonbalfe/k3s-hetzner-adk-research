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
