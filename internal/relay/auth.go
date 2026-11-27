package relay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

const oauthCallback = "127.0.0.1:42813"

func readCredential(key string, out any) error {
	value, err := keyring.Get(credentialService, key)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(value), out)
}

func writeCredential(key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return keyring.Set(credentialService, key, string(data))
}

func randomHex(length int) string {
	data := make([]byte, length)
	_, _ = rand.Read(data)
	return hex.EncodeToString(data)
}

func requestID() string {
	data := make([]byte, 16)
	_, _ = rand.Read(data)
	data[6] = data[6]&0x0f | 0x40
	data[8] = data[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", data[:4], data[4:6], data[6:8], data[8:10], data[10:])
}

func openBrowser(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		command = exec.Command("open", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	if err := command.Start(); err != nil {
		return err
	}
	go command.Wait()
	return nil
}

func authorize(ctx context.Context, cfg *oauth2.Config, provider, key string) error {
	listener, err := net.Listen("tcp4", oauthCallback)
	if err != nil {
		return fmt.Errorf("open the local sign-in port: %w", err)
	}
	defer listener.Close()
	cfg.RedirectURL = "http://" + oauthCallback + "/callback"
	state, verifier := randomHex(32), oauth2.GenerateVerifier()
	code := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("state") != state {
			http.Error(w, "The sign-in request does not match.", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = fmt.Fprintln(w, "Sign-in response received. Return to Minecraft Proxy.")
		select {
		case code <- query.Get("code"):
		default:
		}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	defer server.Close()
	go server.Serve(listener)

	options := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier)}
	if provider == "google" {
		options = append(options, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	}
	if err := openBrowser(cfg.AuthCodeURL(state, options...)); err != nil {
		return fmt.Errorf("open your browser: %w", err)
	}
	select {
	case result := <-code:
		if result == "" {
			return errors.New("authorization was not completed")
		}
		token, err := cfg.Exchange(ctx, result, oauth2.VerifierOption(verifier))
		if err != nil {
			return errors.New("Could not exchange the code. Check the OAuth client settings.")
		}
		if token.RefreshToken == "" {
			return errors.New("No refresh token was returned. Enable refresh tokens in the OAuth client settings.")
		}
		return writeCredential(key, token)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// A refresh token can rotate, so profiles using the same client serialize refreshes.
var refreshLocks sync.Map

type savedTokens struct {
	ctx context.Context
	cfg *oauth2.Config
	key string
}

func (s *savedTokens) Token() (*oauth2.Token, error) {
	lock, _ := refreshLocks.LoadOrStore(s.key, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	var saved oauth2.Token
	if err := readCredential(s.key, &saved); err != nil {
		return nil, err
	}
	token, err := s.cfg.TokenSource(s.ctx, &saved).Token()
	if err != nil {
		return nil, errors.New("Sign-in expired. Sign in again in Settings.")
	}
	if token.AccessToken != saved.AccessToken || token.RefreshToken != saved.RefreshToken || !token.Expiry.Equal(saved.Expiry) {
		if err := writeCredential(s.key, token); err != nil {
			return nil, err
		}
	}
	return token, nil
}

func authenticatedClient(ctx context.Context, config Config, provider string) (*http.Client, error) {
	cfg, key, err := config.oauth(provider)
	if err != nil {
		return nil, err
	}
	var token oauth2.Token
	if err := readCredential(key, &token); err != nil {
		return nil, fmt.Errorf("Sign in to %s in Settings.", provider)
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: 15 * time.Second})
	client := oauth2.NewClient(ctx, &savedTokens{ctx: ctx, cfg: cfg, key: key})
	client.Timeout = 15 * time.Second
	return client, nil
}
