package auth_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

func TestCallbackServer_ReceivesCode(t *testing.T) {
	srv, err := auth.StartCallbackServer()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()

	if srv.Port == 0 {
		t.Fatal("port should be non-zero")
	}

	// Simulate browser redirect.
	go func() {
		url := fmt.Sprintf("http://127.0.0.1:%d/callback?code=test-auth-code&state=test-state", srv.Port)
		resp, err := http.Get(url) //nolint:errcheck
		if err != nil {
			return
		}
		resp.Body.Close() //nolint:errcheck
	}()

	select {
	case result := <-srv.Result:
		if result.Code != "test-auth-code" {
			t.Errorf("Code = %q, want %q", result.Code, "test-auth-code")
		}
		if result.State != "test-state" {
			t.Errorf("State = %q, want %q", result.State, "test-state")
		}
		if result.Error != "" {
			t.Errorf("unexpected error: %s", result.Error)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for callback")
	}
}

func TestCallbackServer_ReceivesError(t *testing.T) {
	srv, err := auth.StartCallbackServer()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()

	go func() {
		url := fmt.Sprintf("http://127.0.0.1:%d/callback?error=access_denied&error_description=User+denied+access", srv.Port)
		resp, err := http.Get(url) //nolint:errcheck
		if err != nil {
			return
		}
		resp.Body.Close() //nolint:errcheck
	}()

	select {
	case result := <-srv.Result:
		if result.Error != "access_denied" {
			t.Errorf("Error = %q, want %q", result.Error, "access_denied")
		}
		if result.ErrorDescription != "User denied access" {
			t.Errorf("ErrorDescription = %q", result.ErrorDescription)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for callback")
	}
}

func TestCallbackServer_RedirectURI(t *testing.T) {
	srv, err := auth.StartCallbackServer()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()

	uri := srv.RedirectURI()
	want := fmt.Sprintf("http://127.0.0.1:%d/callback", srv.Port)
	if uri != want {
		t.Errorf("RedirectURI() = %q, want %q", uri, want)
	}
}
