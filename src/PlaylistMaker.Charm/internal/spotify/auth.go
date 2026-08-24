package spotify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const scopes = "user-read-playback-state user-modify-playback-state"

type Token struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	TokenType    string    `json:"tokenType"`
	Scope        string    `json:"scope"`
	ExpiresAtUTC time.Time `json:"expiresAtUtc"`
}

type Auth struct {
	ClientID     string
	RedirectURI  string
	TokenPath    string
	HTTP         *http.Client
	AccountsBase string
	OpenBrowser  func(string) error
	Now          func() time.Time
}

func (a *Auth) AccessToken(ctx context.Context, interactive bool) (string, error) {
	token, err := a.read()
	if err == nil && token.AccessToken != "" && a.now().Before(token.ExpiresAtUTC.Add(-30*time.Second)) {
		return token.AccessToken, nil
	}
	if err == nil && token.RefreshToken != "" {
		refreshed, refreshErr := a.refresh(ctx, token.RefreshToken)
		if refreshErr == nil {
			return refreshed.AccessToken, nil
		}
		if !interactive {
			return "", refreshErr
		}
	}
	if !interactive {
		return "", fmt.Errorf("Spotify authentication is missing; open Spotify update mode with U")
	}
	loggedIn, err := a.Login(ctx)
	if err != nil {
		return "", err
	}
	return loggedIn.AccessToken, nil
}

func (a *Auth) Refresh(ctx context.Context) (string, error) {
	token, err := a.read()
	if err != nil || token.RefreshToken == "" {
		return "", fmt.Errorf("Spotify refresh token is unavailable")
	}
	refreshed, err := a.refresh(ctx, token.RefreshToken)
	if err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}

func (a *Auth) Login(ctx context.Context) (Token, error) {
	redirect, err := url.Parse(a.RedirectURI)
	if err != nil || redirect.Scheme != "http" || redirect.Hostname() != "127.0.0.1" && redirect.Hostname() != "localhost" {
		return Token{}, fmt.Errorf("spotifyRedirectUri must be an HTTP loopback address")
	}
	state, err := randomHex(24)
	if err != nil {
		return Token{}, err
	}
	verifierBytes := make([]byte, 64)
	if _, err := rand.Read(verifierBytes); err != nil {
		return Token{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	challengeBytes := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])
	listener, err := net.Listen("tcp", redirect.Host)
	if err != nil {
		return Token{}, fmt.Errorf("listen for Spotify callback: %w", err)
	}
	defer listener.Close()
	type result struct {
		code string
		err  error
	}
	resultChannel := make(chan result, 1)
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second}
	server.Handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != redirect.Path {
			http.NotFound(writer, request)
			return
		}
		if request.URL.Query().Get("state") != state {
			resultChannel <- result{err: fmt.Errorf("Spotify callback state did not match")}
			http.Error(writer, "Spotify login state did not match", http.StatusBadRequest)
			return
		}
		if message := request.URL.Query().Get("error"); message != "" {
			resultChannel <- result{err: fmt.Errorf("Spotify authorization failed: %s", message)}
			http.Error(writer, "Spotify authorization failed", http.StatusBadRequest)
			return
		}
		code := request.URL.Query().Get("code")
		if code == "" {
			resultChannel <- result{err: fmt.Errorf("Spotify callback did not contain a code")}
			http.Error(writer, "Spotify authorization code missing", http.StatusBadRequest)
			return
		}
		_, _ = writer.Write([]byte("Spotify login complete. You can close this window."))
		resultChannel <- result{code: code}
	})
	go func() { _ = server.Serve(listener) }()
	authorize, _ := url.Parse(a.accountsBase() + "/authorize")
	query := authorize.Query()
	query.Set("response_type", "code")
	query.Set("client_id", a.ClientID)
	query.Set("scope", scopes)
	query.Set("redirect_uri", a.RedirectURI)
	query.Set("state", state)
	query.Set("code_challenge_method", "S256")
	query.Set("code_challenge", challenge)
	authorize.RawQuery = query.Encode()
	opener := a.OpenBrowser
	if opener == nil {
		opener = openBrowser
	}
	if err := opener(authorize.String()); err != nil {
		_ = server.Shutdown(context.Background())
		return Token{}, err
	}
	var callback result
	select {
	case <-ctx.Done():
		callback.err = ctx.Err()
	case callback = <-resultChannel:
	}
	_ = server.Shutdown(context.Background())
	if callback.err != nil {
		return Token{}, callback.err
	}
	return a.exchange(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {callback.code}, "redirect_uri": {a.RedirectURI}, "client_id": {a.ClientID}, "code_verifier": {verifier}}, "")
}

func (a *Auth) refresh(ctx context.Context, refreshToken string) (Token, error) {
	return a.exchange(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}, "client_id": {a.ClientID}}, refreshToken)
}

func (a *Auth) exchange(ctx context.Context, values url.Values, existingRefresh string) (Token, error) {
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, a.accountsBase()+"/api/token", strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := a.client().Do(request)
	if err != nil {
		return Token{}, err
	}
	defer response.Body.Close()
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		Description  string `json:"error_description"`
	}
	if json.NewDecoder(response.Body).Decode(&payload) != nil || response.StatusCode/100 != 2 || payload.AccessToken == "" {
		return Token{}, fmt.Errorf("Spotify token request failed: %s %s", payload.Error, payload.Description)
	}
	if payload.RefreshToken == "" {
		payload.RefreshToken = existingRefresh
	}
	token := Token{AccessToken: payload.AccessToken, RefreshToken: payload.RefreshToken, TokenType: payload.TokenType, Scope: payload.Scope, ExpiresAtUTC: a.now().Add(time.Duration(payload.ExpiresIn) * time.Second).UTC()}
	if err := a.write(token); err != nil {
		return Token{}, err
	}
	return token, nil
}

func (a *Auth) read() (Token, error) {
	contents, err := os.ReadFile(a.TokenPath)
	if err != nil {
		return Token{}, err
	}
	var token Token
	if err := json.Unmarshal(contents, &token); err != nil {
		return Token{}, err
	}
	return token, nil
}

func (a *Auth) write(token Token) error {
	contents, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(a.TokenPath), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(a.TokenPath), ".spotify-auth-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err = temporary.Write(append(contents, '\n')); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := secureFile(name); err != nil {
		return err
	}
	return os.Rename(name, a.TokenPath)
}

func (a *Auth) client() *http.Client {
	if a.HTTP != nil {
		return a.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}
func (a *Auth) accountsBase() string {
	if a.AccountsBase != "" {
		return strings.TrimRight(a.AccountsBase, "/")
	}
	return "https://accounts.spotify.com"
}
func (a *Auth) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}
func randomHex(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
