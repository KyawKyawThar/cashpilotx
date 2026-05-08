// Package server wires together the HTTP router and middleware for CashPilotX.
package server

import (
	"cashpilotx/internal/config"
	"cashpilotx/internal/dto"
	mw "cashpilotx/internal/middleware"
	"context"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	httpSwagger "github.com/swaggo/http-swagger"
)

func (s *Server) setupRoutes() {
	r := chi.NewRouter()

	s.configureLogger()
	s.setupMiddleware(r)
	s.setupSwagger(r)
	s.setupPublicRoutes(r)
	s.setupProtectedRoutes(r)

	s.router = r
}

func (s *Server) configureLogger() {
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	if s.config.Environment == config.EnvironmentDevelopment {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()
	}
	s.logger = &logger
}

func (s *Server) setupMiddleware(r chi.Router) {
	r.Use(mw.RealIP)
	r.Use(mw.RequestID)
	r.Use(mw.Recoverer(s.logger))

	healthPaths := []string{"/api/v1/ready", "/api/v1/health"}
	r.Use(mw.SkipPaths(s.logger, healthPaths...))

	if s.config.Environment == config.EnvironmentDevelopment {
		r.Use(mw.SimpleCORS)
	} else {
		r.Use(mw.CORS(mw.ProductionCORSConfig(s.config.AllowedOrigins)))
	}

	r.Use(mw.SecurityHeaders)
}

func (s *Server) setupSwagger(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(mw.SwaggerCSP)
		r.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/docs/", http.StatusMovedPermanently)
		})
		r.Get("/docs/*", httpSwagger.Handler(
			httpSwagger.URL("/docs/doc.json"),
			httpSwagger.DeepLinking(true),
			httpSwagger.DocExpansion("list"),
		))
	})
}

// requestTimeout returns a middleware deadline that fires before WriteTimeout
// so context cancellation reaches handlers before the HTTP server hard-kills the connection.
func (s *Server) requestTimeout() time.Duration {
	const margin = 5 * time.Second
	if s.config.ServerWriteTimeout > margin {
		return s.config.ServerWriteTimeout - margin
	}
	return 10 * time.Second
}

func (s *Server) setupPublicRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(mw.Timeout(s.requestTimeout()))
		r.Use(mw.MaxBodySize(1 << 20)) // 1 MB

		r.Get("/api/v1/health", s.handleHealth)
		r.Get("/api/v1/ready", s.handleReady)
		r.Get("/api/v1/version", s.handleVersion)

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			dto.WriteJSON(w, http.StatusOK, map[string]string{
				"service": "CashPilotX API",
				"version": "1.0.0",
				"status":  "running",
				"docs":    "/docs",
			})
		})

		// All /api/v1/auth routes live in one block — chi panics if the same
		// path is mounted twice via r.Route() on the same mux.
		r.Route("/api/v1/auth", func(r chi.Router) {
			// Public — no JWT required
			r.Post("/register", s.handler.Auth.Register)
			r.Post("/login", s.handler.Auth.Login)
			r.Post("/refresh", s.handler.Auth.RefreshToken)
			r.Post("/forgot-pin", s.handler.Auth.ForgotPIN)
			r.Post("/reset-pin", s.handler.Auth.ResetPIN)

			// Protected — JWT required
			r.Group(func(r chi.Router) {
				r.Use(mw.JWTAuthWithService(s.services.Auth))
				r.Post("/logout", s.handler.Auth.Logout)
				r.Get("/sessions", s.handler.Auth.GetSessions)
				r.Delete("/sessions/{session_id}", s.handler.Auth.RevokeSession)
			})
		})
	})
}

func (s *Server) setupProtectedRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(mw.Timeout(s.requestTimeout()))
		r.Use(mw.MaxBodySize(1 << 20)) // 1 MB
		r.Use(mw.JWTAuthWithService(s.services.Auth))

		r.Route("/api/v1/customers", func(r chi.Router) {
			r.Post("/", s.handler.Customer.CreateCustomer)
			r.Get("/", s.handler.Customer.ListCustomers)
			r.Get("/search", s.handler.Customer.SearchCustomers)
			r.Get("/{customer_id}", s.handler.Customer.GetCustomer)
			r.Patch("/{customer_id}", s.handler.Customer.UpdateCustomer)
			r.Delete("/{customer_id}", s.handler.Customer.ArchiveCustomer)
		})

		r.Route("/api/v1/channel-configs", func(r chi.Router) {
			r.Post("/", s.handler.Customer.UpsertChannelConfig)
			r.Get("/", s.handler.Customer.ListChannelConfigs)
			r.Get("/{channel}", s.handler.Customer.GetChannelConfig)
			r.Delete("/{channel}", s.handler.Customer.DeleteChannelConfig)
			r.Patch("/{channel}/active", s.handler.Customer.SetChannelActive)
		})
	})
}

// r.Route("/api/v1/invoices", func(r chi.Router) {
//     r.With(mw.Idempotency(s.store)).Post("/", s.handler.Invoice.Create)
// })

// =============================================================================
// Probe handlers
// =============================================================================

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	dto.WriteJSON(w, http.StatusOK, map[string]string{
		"status": config.StatusHealthy,
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]string)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.store.Ping(ctx); err != nil {
		checks["database"] = config.StatusUnhealthy + ": " + err.Error()
		dto.WriteReady(w, false, checks)
		return
	}
	checks["database"] = config.StatusHealthy

	if s.cache != nil {
		if err := s.cache.Client.Ping(ctx).Err(); err != nil {
			checks["cache"] = config.StatusUnhealthy + ": " + err.Error()
			dto.WriteReady(w, false, checks)
			return
		}
		checks["cache"] = config.StatusHealthy
	}

	dto.WriteReady(w, true, checks)
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	dto.WriteJSON(w, http.StatusOK, map[string]string{
		"version":     "1.0.0",
		"api_version": "v1",
		"go_version":  "1.26.1",
	})
}
