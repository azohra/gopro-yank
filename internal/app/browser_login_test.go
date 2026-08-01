package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestGoProAccessTokenStaysWithinGoPro(t *testing.T) {
	token := goProAccessToken([]browserCookie{
		{Name: "gp_access_token", Value: "wrong", Domain: "notgopro.com"},
		{Name: "gp_access_token", Value: "token", Domain: ".gopro.com"},
	})
	if token != "token" {
		t.Fatalf("unexpected GoPro token: %q", token)
	}
}

func TestCaptureBrowserCookiesWaitsForLogin(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer connection.Close()
		for call := 1; call <= 2; call++ {
			var request struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
			}
			if err := connection.ReadJSON(&request); err != nil {
				t.Error(err)
				return
			}
			if request.Method != "Storage.getCookies" {
				t.Errorf("unexpected method %q", request.Method)
			}
			cookies := []browserCookie{}
			if call == 2 {
				cookies = append(cookies, browserCookie{Name: "gp_access_token", Value: "token", Domain: ".gopro.com"})
			}
			if err := connection.WriteJSON(map[string]any{"id": request.ID, "result": map[string]any{"cookies": cookies}}); err != nil {
				t.Error(err)
				return
			}
		}
	}))
	defer server.Close()

	address := "ws" + strings.TrimPrefix(server.URL, "http")
	token, err := captureBrowserToken(context.Background(), address)
	if err != nil {
		t.Fatal(err)
	}
	if token != "token" {
		t.Fatalf("unexpected login: token=%q", token)
	}
}

func TestParseCredential(t *testing.T) {
	for name, test := range map[string]struct {
		value string
		want  string
	}{
		"token":   {value: "abc.def", want: "abc.def"},
		"cookie":  {value: "other=x; gp_access_token=abc.def; another=y", want: "abc.def"},
		"missing": {value: "other=x", want: ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := parseCredential(test.value, "gp_access_token"); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestProfileUserID(t *testing.T) {
	profile := map[string]any{"user": map[string]any{"id": "user"}}
	if got := profileUserID(profile); got != "user" {
		t.Fatalf("got %q", got)
	}
}
