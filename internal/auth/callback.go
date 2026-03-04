package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
)

// CallbackResult holds the data received from the OAuth redirect.
type CallbackResult struct {
	Code             string
	State            string
	Error            string
	ErrorDescription string
}

// CallbackServer is a temporary local HTTP server for receiving OAuth redirects.
type CallbackServer struct {
	Port   int
	Result chan CallbackResult
	server *http.Server
}

// StartCallbackServer starts an HTTP server on a random available port.
// It listens for a single OAuth callback, sends the result on the Result channel,
// then the caller should Shutdown().
func StartCallbackServer() (*CallbackServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("starting callback listener: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	result := make(chan CallbackResult, 1)

	mux := http.NewServeMux()
	var once sync.Once
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		once.Do(func() {
			if e := q.Get("error"); e != "" {
				result <- CallbackResult{
					Error:            e,
					ErrorDescription: q.Get("error_description"),
				}
			} else {
				result <- CallbackResult{
					Code:  q.Get("code"),
					State: q.Get("state"),
				}
			}
		})

		w.Header().Set("Content-Type", "text/html")
		if q.Get("error") != "" {
			_, _ = fmt.Fprint(w, "<html><body>Authorization failed. You can close this tab.</body></html>")
		} else {
			_, _ = fmt.Fprint(w, "<html><body>Authorization successful! You can close this tab.<script>window.close()</script></body></html>")
		}
	})

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener) //nolint:errcheck

	return &CallbackServer{
		Port:   port,
		Result: result,
		server: srv,
	}, nil
}

// RedirectURI returns the redirect URI for this server.
func (s *CallbackServer) RedirectURI() string {
	return fmt.Sprintf("http://127.0.0.1:%d/callback", s.Port)
}

// Shutdown gracefully stops the callback server.
func (s *CallbackServer) Shutdown() {
	s.server.Shutdown(context.Background()) //nolint:errcheck
}
