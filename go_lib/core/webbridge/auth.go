package webbridge

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"go_lib/core/repository"
	"golang.org/x/crypto/bcrypt"
)

const (
	webAuthSettingKey = "webui_auth_credentials"
	webAuthUsername   = "sysadmin"
	webAuthTokenTTL   = 12 * time.Hour
)

type webAuthCredentials struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	UpdatedAt    string `json:"updated_at"`
}

type webAuthBootstrap struct {
	GeneratedInitialPassword bool
	Username                 string
	InitialPassword          string
	Token                    string
}

type webAuthSession struct {
	Username  string
	ExpiresAt time.Time
}

type WebAuthManager struct {
	mu           sync.Mutex
	credentialMu sync.Mutex
	sessions     map[string]webAuthSession
}

func NewWebAuthManager() *WebAuthManager {
	return &WebAuthManager{sessions: make(map[string]webAuthSession)}
}

func (m *WebAuthManager) EnsureInitialized() (webAuthBootstrap, error) {
	m.credentialMu.Lock()
	defer m.credentialMu.Unlock()

	var result webAuthBootstrap
	creds, found, err := m.loadCredentials()
	if err != nil {
		return result, err
	}
	if found {
		result.Username = creds.Username
		return result, nil
	}

	password, err := generateInitialPassword()
	if err != nil {
		return result, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return result, fmt.Errorf("failed to hash initial password: %w", err)
	}

	creds = webAuthCredentials{
		Username:     webAuthUsername,
		PasswordHash: string(hash),
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := m.saveCredentials(creds); err != nil {
		return result, err
	}
	token, err := m.createSession(creds.Username)
	if err != nil {
		return result, err
	}

	return webAuthBootstrap{
		GeneratedInitialPassword: true,
		Username:                 creds.Username,
		InitialPassword:          password,
		Token:                    token,
	}, nil
}

func (m *WebAuthManager) Login(username, password string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return "", fmt.Errorf("username and password are required")
	}

	m.credentialMu.Lock()
	defer m.credentialMu.Unlock()

	creds, found, err := m.loadCredentials()
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("web auth is not initialized")
	}
	if subtle.ConstantTimeCompare([]byte(username), []byte(creds.Username)) != 1 {
		return "", fmt.Errorf("invalid username or password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(creds.PasswordHash), []byte(password)); err != nil {
		return "", fmt.Errorf("invalid username or password")
	}
	return m.createSession(creds.Username)
}

func (m *WebAuthManager) ChangePassword(currentPassword, newPassword string) error {
	if currentPassword == "" {
		return fmt.Errorf("current password is required")
	}
	if len([]rune(newPassword)) < 6 {
		return fmt.Errorf("new password must be at least 6 characters")
	}

	m.credentialMu.Lock()
	defer m.credentialMu.Unlock()

	creds, found, err := m.loadCredentials()
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("web auth is not initialized")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(creds.PasswordHash), []byte(currentPassword)); err != nil {
		return fmt.Errorf("current password is incorrect")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}
	creds.PasswordHash = string(hash)
	creds.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := m.saveCredentials(creds); err != nil {
		return err
	}
	m.clearSessions()
	return nil
}

func (m *WebAuthManager) AuthenticateRequest(r *http.Request) bool {
	token := bearerTokenFromRequest(r)
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("access_token"))
	}
	if token == "" {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(session.ExpiresAt) {
		delete(m.sessions, token)
		return false
	}
	session.ExpiresAt = time.Now().Add(webAuthTokenTTL)
	m.sessions[token] = session
	return true
}

func (m *WebAuthManager) Logout(r *http.Request) {
	token := bearerTokenFromRequest(r)
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("access_token"))
	}
	if token == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
}

func (m *WebAuthManager) createSession(username string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate auth token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[token] = webAuthSession{
		Username:  username,
		ExpiresAt: time.Now().Add(webAuthTokenTTL),
	}
	return token, nil
}

func (m *WebAuthManager) clearSessions() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions = make(map[string]webAuthSession)
}

func (m *WebAuthManager) loadCredentials() (webAuthCredentials, bool, error) {
	var creds webAuthCredentials
	repo := repository.NewAppSettingsRepository(nil)
	raw, err := repo.GetSetting(webAuthSettingKey)
	if err != nil {
		return creds, false, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return creds, false, nil
	}
	if err := json.Unmarshal([]byte(raw), &creds); err != nil {
		return creds, false, fmt.Errorf("invalid web auth credentials: %w", err)
	}
	if creds.Username == "" || creds.PasswordHash == "" {
		return creds, false, fmt.Errorf("invalid web auth credentials")
	}
	return creds, true, nil
}

func (m *WebAuthManager) saveCredentials(creds webAuthCredentials) error {
	payload, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to encode web auth credentials: %w", err)
	}
	repo := repository.NewAppSettingsRepository(nil)
	if err := repo.SaveSetting(webAuthSettingKey, string(payload)); err != nil {
		return fmt.Errorf("failed to save web auth credentials: %w", err)
	}
	return nil
}

func bearerTokenFromRequest(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" {
		return ""
	}
	parts := strings.Fields(auth)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func generateInitialPassword() (string, error) {
	english := []rune("23456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz")
	password := make([]rune, 0, 6)
	for len(password) < 6 {
		r, err := randomRune(english)
		if err != nil {
			return "", fmt.Errorf("failed to generate initial password: %w", err)
		}
		password = append(password, r)
	}
	for i := range password {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(len(password))))
		if err != nil {
			return "", fmt.Errorf("failed to shuffle initial password: %w", err)
		}
		password[i], password[j.Int64()] = password[j.Int64()], password[i]
	}
	return string(password), nil
}

func randomRune(runes []rune) (rune, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(runes))))
	if err != nil {
		return 0, err
	}
	return runes[n.Int64()], nil
}
