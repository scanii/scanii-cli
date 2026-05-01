package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/uvasoftware/scanii-cli/assets"
	"github.com/uvasoftware/scanii-cli/internal/client"
	"github.com/uvasoftware/scanii-cli/internal/engine"
	"github.com/uvasoftware/scanii-cli/internal/identifiers"
)

type contextKey int

const (
	keyInContext contextKey = iota
)

// middleware chains multiple middleware functions around a handler.
func middleware(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

func Setup(mux *http.ServeMux, eng *engine.Engine, key, secret, data, baseURL string) {
	handlers := FakeHandler{
		engine:  eng,
		store:   store{path: data},
		baseurl: baseURL,
	}

	hostID := "hst_" + identifiers.GenerateShort()

	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, password, ok := r.BasicAuth()
			if ok {

				// we allow two forms of authentication, API keys and auth tokens
				if password == "" {
					token := &client.AuthToken{}
					err := handlers.store.load(username, token)
					if err != nil {
						slog.Error("failed to load token", "error", err)
					} else {
						ctx := context.WithValue(r.Context(), keyInContext, *token.ID)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}

				} else {
					usernameHash := sha256.Sum256([]byte(username))
					passwordHash := sha256.Sum256([]byte(password))
					expectedUsernameHash := sha256.Sum256([]byte(key))
					expectedPasswordHash := sha256.Sum256([]byte(secret))

					usernameMatch := subtle.ConstantTimeCompare(usernameHash[:], expectedUsernameHash[:]) == 1
					passwordMatch := subtle.ConstantTimeCompare(passwordHash[:], expectedPasswordHash[:]) == 1

					if usernameMatch && passwordMatch {
						ctx := context.WithValue(r.Context(), keyInContext, username)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
			}

			err := writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "Apologies but we could not authenticate this request.",
			}, http.Header{"WWW-Authenticate": {" Basic realm=Scanii API"}})
			if err != nil {
				panic(err)
			}
		})
	}

	headersMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(`X-Scanii-Request-Id`, "req_"+identifiers.GenerateShort())
			w.Header().Set(`X-Scanii-Host-Id`, hostID)
			next.ServeHTTP(w, r)
		})
	}

	// wrap wraps a handler func with the auth and headers middleware.
	wrap := func(h http.HandlerFunc) http.Handler {
		return middleware(h, authMiddleware, headersMiddleware)
	}

	// Static fixtures (unauthenticated) — used both by the CLI demo and
	// by integration tests that fetch via the server's own URL.
	mux.HandleFunc("GET /static/eicar.txt", serverEICAR)
	fileServer := http.FileServer(http.FS(assets.EmbeddedFiles))
	mux.Handle("GET /static/", http.StripPrefix("/static/", fileServer))

	// healthcheck route used by the docker container:
	mux.Handle("GET /healthcheck", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, err := io.WriteString(writer, "UP")
		if err != nil {
			slog.Error("failed to write status response", "error", err)
		}
	}))

	mux.Handle("GET /v2.2/account.json", wrap(handlers.Account))
	mux.Handle("GET /v2.2/ping", wrap(handlers.Ping))
	mux.Handle("POST /v2.2/files/async", wrap(handlers.ProcessFileAsync))
	mux.Handle("POST /v2.2/files/fetch", wrap(handlers.ProcessFileFetch))
	mux.Handle("POST /v2.2/files", wrap(handlers.ProcessFile))
	mux.Handle("GET /v2.2/files/{id}/trace", wrap(func(w http.ResponseWriter, r *http.Request) {
		handlers.RetrieveTrace(w, r, r.PathValue("id"))
	}))
	mux.Handle("GET /v2.2/files/{id}", wrap(func(w http.ResponseWriter, r *http.Request) {
		handlers.RetrieveFile(w, r, r.PathValue("id"))
	}))

	mux.Handle("POST /v2.2/auth/tokens", wrap(handlers.CreateToken))
	mux.Handle("GET /v2.2/auth/tokens/{id}", wrap(func(w http.ResponseWriter, r *http.Request) {
		handlers.RetrieveToken(w, r, r.PathValue("id"))
	}))
	mux.Handle("DELETE /v2.2/auth/tokens/{id}", wrap(func(w http.ResponseWriter, r *http.Request) {
		handlers.DeleteToken(w, r, r.PathValue("id"))
	}))
}

func generateID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")
}
