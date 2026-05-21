// Package server implements the local HTTP API consumed by the embedded dashboard.
package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Deps is the set of dependencies the server needs.
type Deps struct {
	Store    contracts.Store
	Pricing  contracts.PricingTable
	Profiles ProfileLister
	WebRoot  http.FileSystem
}

// ProfileLister exposes the subset of the profile manager the server needs.
type ProfileLister interface {
	List(ctx context.Context) ([]contracts.Profile, error)
}

// Server is the local HTTP server.
type Server struct {
	deps    Deps
	version string
	mux     *chi.Mux
}

// New constructs a Server.
func New(deps Deps, version string) *Server {
	s := &Server{deps: deps, version: version, mux: chi.NewRouter()}
	s.routes()
	return s
}

// Handler returns the underlying http.Handler.
func (s *Server) Handler() http.Handler { return s.mux }

// Serve listens on 127.0.0.1 within [startPort, endPort].
func (s *Server) Serve(ctx context.Context, startPort, endPort int) (boundPort int, run func() error, err error) {
	var (
		ln   net.Listener
		port int
	)
	for port = startPort; port <= endPort; port++ {
		ln, err = net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			break
		}
	}
	if ln == nil {
		return 0, nil, errors.New("no free port in range")
	}

	srv := &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	return port, func() error {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}, nil
}

func (s *Server) routes() {
	s.mux.Use(middleware.RealIP)
	s.mux.Use(middleware.Recoverer)
	s.mux.Use(securityHeaders)
	s.mux.Get("/api/health", s.handleHealth)
	s.mux.Get("/api/profiles", s.handleProfiles)
	s.mux.Get("/api/usage", s.handleUsage)
	s.mux.Get("/api/usage/live", s.handleUsageLive)
	if s.deps.WebRoot != nil {
		s.mux.Handle("/*", http.FileServer(s.deps.WebRoot))
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'")
		next.ServeHTTP(w, r)
	})
}
