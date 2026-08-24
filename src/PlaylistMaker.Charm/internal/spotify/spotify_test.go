package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"playlistmaker/charm/internal/tracking"
)

func TestLoginRejectsMismatchedPKCEState(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	auth := &Auth{ClientID: "client", RedirectURI: "http://" + address + "/callback", TokenPath: filepath.Join(t.TempDir(), "auth.json")}
	auth.OpenBrowser = func(target string) error {
		parsed, _ := url.Parse(target)
		if parsed.Query().Get("code_challenge_method") != "S256" || parsed.Query().Get("code_challenge") == "" {
			t.Errorf("authorization URL did not contain PKCE: %s", target)
		}
		go func() {
			_, _ = http.Get(auth.RedirectURI + "?code=code&state=wrong")
		}()
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := auth.Login(ctx); err == nil || !strings.Contains(err.Error(), "state") {
		t.Fatalf("state validation error = %v", err)
	}
}

func TestClientRefreshesOnceOnExpiredAccessAndPersistsToken(t *testing.T) {
	var mu sync.Mutex
	apiCalls, tokenCalls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if request.URL.Path == "/api/token" {
			tokenCalls++
			_ = json.NewEncoder(writer).Encode(map[string]any{"access_token": "fresh", "token_type": "Bearer", "expires_in": 3600})
			return
		}
		apiCalls++
		if request.Header.Get("Authorization") == "Bearer old" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"devices": []any{}})
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "spotify-auth.json")
	auth := &Auth{ClientID: "client", RedirectURI: "http://127.0.0.1/callback", TokenPath: path, HTTP: server.Client(), AccountsBase: server.URL, Now: func() time.Time { return time.Unix(1000, 0) }}
	if err := auth.write(Token{AccessToken: "old", RefreshToken: "refresh", ExpiresAtUTC: time.Unix(5000, 0)}); err != nil {
		t.Fatal(err)
	}
	client := &Client{Auth: auth, HTTP: server.Client(), APIBase: server.URL}
	if _, err := client.Devices(context.Background()); err != nil {
		t.Fatal(err)
	}
	if apiCalls != 2 || tokenCalls != 1 {
		t.Fatalf("API calls = %d, token calls = %d", apiCalls, tokenCalls)
	}
	contents, _ := os.ReadFile(path)
	if !strings.Contains(string(contents), "fresh") {
		t.Fatalf("refreshed token was not persisted: %s", contents)
	}
	if !privateFile(path) {
		t.Fatal("token file permissions are not private")
	}
}

func TestPlayerRequiresUniqueDeviceAndRestoresVolume(t *testing.T) {
	requests := []string{}
	duplicate := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.Method+" "+request.URL.String())
		if request.URL.Path == "/me/player/devices" {
			devices := []map[string]any{{"id": "one", "name": "Living Room", "is_restricted": false, "supports_volume": true, "volume_percent": 37}}
			if duplicate {
				devices = append(devices, map[string]any{"id": "two", "name": "living room", "is_restricted": false, "supports_volume": true, "volume_percent": 20})
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"devices": devices})
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	auth := validAuth(t, server)
	client := &Client{Auth: auth, HTTP: server.Client(), APIBase: server.URL}
	state := filepath.Join(t.TempDir(), "active.json")
	player := &Player{Client: client, StatePath: state, SessionID: "session", HelperPID: 123}
	if err := player.Preflight(context.Background(), "living room"); err != nil {
		t.Fatal(err)
	}
	if err := player.Start(context.Background(), tracking.Track{SpotifyURI: "spotify:track:abc"}); err != nil {
		t.Fatal(err)
	}
	if err := player.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(requests, "\n")
	if !strings.Contains(joined, "volume_percent=0") || !strings.Contains(joined, "volume_percent=37") || !strings.Contains(joined, "/play") || !strings.Contains(joined, "/pause") {
		t.Fatalf("Spotify session requests:\n%s", joined)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("active state remained after restoration")
	}
	duplicate = true
	if err := (&Player{Client: client, StatePath: state}).Preflight(context.Background(), "Living Room"); err == nil || !strings.Contains(err.Error(), "matched 2") {
		t.Fatalf("duplicate device error = %v", err)
	}
}

func TestPlaybackRateLimitReturnsImmediatelyWithoutRetry(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.Header().Set("Retry-After", "30")
		writer.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}
	started := time.Now()
	err := client.Play(context.Background(), "device", "spotify:track:one")
	var rateLimit *RateLimitError
	if !errors.As(err, &rateLimit) || rateLimit.RetryAfter != 30*time.Second {
		t.Fatalf("play error = %v", err)
	}
	if requests != 1 || time.Since(started) > 500*time.Millisecond {
		t.Fatalf("play requests = %d, duration = %s", requests, time.Since(started))
	}
}

func TestRecoverLeavesActiveStateAndRestoresStaleState(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	statePath := filepath.Join(t.TempDir(), "active.json")
	state, _ := json.Marshal(ActiveState{SessionID: "session", HelperPID: 42, DeviceID: "device", OriginalVolume: 37})
	if err := os.WriteFile(statePath, state, 0o600); err != nil {
		t.Fatal(err)
	}
	client := &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}
	if err := Recover(context.Background(), client, statePath, func(pid int) bool { return pid == 42 }); err != nil {
		t.Fatal(err)
	}
	if requests != 0 {
		t.Fatal("active Spotify state was recovered")
	}
	if err := Recover(context.Background(), client, statePath, func(int) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("stale recovery requests = %d", requests)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatal("stale state remained")
	}
}

func TestRecoverHandlesMissingAndPreservesMalformedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "active.json")
	if err := Recover(context.Background(), nil, path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Recover(context.Background(), nil, path); err == nil {
		t.Fatal("malformed state was accepted")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("malformed state was removed")
	}
}

func TestPlayerRejectsDeviceWithoutVolumeSupport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{"devices": []map[string]any{{"id": "one", "name": "Room", "supports_volume": false, "volume_percent": 20}}})
	}))
	defer server.Close()
	client := &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}
	if err := (&Player{Client: client, StatePath: filepath.Join(t.TempDir(), "state")}).Preflight(context.Background(), "room"); err == nil {
		t.Fatal("unsupported volume device was accepted")
	}
}

func validAuth(t *testing.T, server *httptest.Server) *Auth {
	t.Helper()
	auth := &Auth{ClientID: "client", TokenPath: filepath.Join(t.TempDir(), "auth.json"), HTTP: server.Client(), AccountsBase: server.URL}
	if err := auth.write(Token{AccessToken: "token", RefreshToken: "refresh", ExpiresAtUTC: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	return auth
}

func Example_trackID() {
	id, _ := trackID("https://open.spotify.com/track/abc?si=one")
	fmt.Println(id)
	// Output: abc
}
