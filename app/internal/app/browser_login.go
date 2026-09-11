package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const browserStartupTimeout = 15 * time.Second

type browserCookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
}

type devToolsReply struct {
	ID     int `json:"id"`
	Result struct {
		Cookies []browserCookie `json:"cookies"`
	} `json:"result"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type launchedBrowser struct {
	command *exec.Cmd
	done    chan struct{}
	profile string
	waitErr error
}

func browserCandidates() []string {
	if configured := strings.TrimSpace(os.Getenv("GOPRO_YANK_BROWSER")); configured != "" {
		return []string{configured}
	}
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	case "windows":
		roots := []string{os.Getenv("LOCALAPPDATA"), os.Getenv("PROGRAMFILES"), os.Getenv("PROGRAMFILES(X86)")}
		var candidates []string
		for _, root := range roots {
			if root == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(root, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
			)
		}
		return candidates
	default:
		return []string{"google-chrome", "microsoft-edge", "brave-browser", "chromium", "chromium-browser"}
	}
}

func findBrowser() (string, error) {
	for _, candidate := range browserCandidates() {
		if filepath.IsAbs(candidate) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
			continue
		}
		if found, err := exec.LookPath(candidate); err == nil {
			return found, nil
		}
	}
	return "", errors.New("Chrome, Edge, Brave, or Chromium was not found")
}

func launchLoginBrowser(ctx context.Context) (*launchedBrowser, error) {
	executable, err := findBrowser()
	if err != nil {
		return nil, err
	}
	profile, err := os.MkdirTemp("", "gopro-yank-login-")
	if err != nil {
		return nil, fmt.Errorf("create private browser profile: %w", err)
	}
	command := exec.CommandContext(ctx, executable,
		"--user-data-dir="+profile,
		"--remote-debugging-port=0",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-mode",
		"--disable-sync",
		"--app="+mediaLibraryURL,
	)
	if err := command.Start(); err != nil {
		_ = os.RemoveAll(profile)
		return nil, fmt.Errorf("open sign-in window: %w", err)
	}
	browser := &launchedBrowser{command: command, done: make(chan struct{}), profile: profile}
	go func() {
		browser.waitErr = command.Wait()
		close(browser.done)
	}()
	return browser, nil
}

func (browser *launchedBrowser) close() error {
	if browser.command.Process != nil {
		_ = browser.command.Process.Kill()
	}
	select {
	case <-browser.done:
	case <-time.After(3 * time.Second):
	}
	var err error
	for range 10 {
		if err = os.RemoveAll(browser.profile); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("remove private browser profile: %w", err)
}

func (browser *launchedBrowser) websocketURL(ctx context.Context) (string, error) {
	startup, cancel := context.WithTimeout(ctx, browserStartupTimeout)
	defer cancel()
	portFile := filepath.Join(browser.profile, "DevToolsActivePort")
	for {
		payload, err := os.ReadFile(portFile)
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(payload)), "\n")
			if len(lines) >= 2 {
				if _, err := strconv.Atoi(lines[0]); err == nil {
					return "ws://127.0.0.1:" + lines[0] + "/" + strings.TrimPrefix(lines[1], "/"), nil
				}
			}
		}
		select {
		case <-browser.done:
			if browser.waitErr == nil {
				return "", errors.New("sign-in window closed")
			}
			return "", fmt.Errorf("sign-in window closed: %w", browser.waitErr)
		case <-startup.Done():
			return "", errors.New("sign-in window did not become ready")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func isGoProDomain(domain string) bool {
	domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), ".")
	return domain == "gopro.com" || strings.HasSuffix(domain, ".gopro.com")
}

func goProAccessToken(cookies []browserCookie) string {
	for _, cookie := range cookies {
		if isGoProDomain(cookie.Domain) && cookie.Name == "gp_access_token" {
			return cookie.Value
		}
	}
	return ""
}

func captureBrowserToken(ctx context.Context, address string) (string, error) {
	dialer := *websocket.DefaultDialer
	dialer.Proxy = nil
	connection, response, err := dialer.DialContext(ctx, address, http.Header{})
	if err != nil {
		if response != nil {
			return "", fmt.Errorf("connect to sign-in window: HTTP %d", response.StatusCode)
		}
		return "", fmt.Errorf("connect to sign-in window: %w", err)
	}
	defer connection.Close()
	connection.SetReadLimit(1 << 20)
	requestID := 0
	for {
		requestID++
		if err := connection.WriteJSON(map[string]any{"id": requestID, "method": "Storage.getCookies"}); err != nil {
			return "", fmt.Errorf("read sign-in: %w", err)
		}
		for {
			var reply devToolsReply
			if err := connection.ReadJSON(&reply); err != nil {
				return "", fmt.Errorf("read sign-in: %w", err)
			}
			if reply.ID != requestID {
				continue
			}
			if reply.Error != nil {
				return "", errors.New(reply.Error.Message)
			}
			if token := goProAccessToken(reply.Result.Cookies); token != "" {
				return token, nil
			}
			break
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func loginInBrowser(ctx context.Context) (token string, err error) {
	browser, err := launchLoginBrowser(ctx)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := browser.close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	address, err := browser.websocketURL(ctx)
	if err != nil {
		return "", err
	}
	return captureBrowserToken(ctx, address)
}

func profileUserID(profile map[string]any) string {
	for _, key := range []string{"id", "user_id", "uuid"} {
		if value := stringValue(profile[key]); value != "" {
			return value
		}
	}
	if user, ok := profile["user"].(map[string]any); ok {
		return profileUserID(user)
	}
	return ""
}

func parseCredential(value, name string) string {
	value = strings.TrimSpace(value)
	for _, fragment := range strings.FieldsFunc(value, func(r rune) bool { return r == ';' || r == '\n' || r == '\r' }) {
		fragment = strings.Trim(strings.TrimSpace(fragment), "'\"")
		if key, found := strings.CutPrefix(fragment, name+"="); found {
			return strings.Trim(strings.TrimSpace(key), "'\"")
		}
	}
	if !strings.ContainsAny(value, " ;\r\n") && !strings.Contains(value, "=") {
		return strings.Trim(value, "'\"")
	}
	return ""
}
