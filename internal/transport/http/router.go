// Package http содержит HTTP-транспорт сервиса kitchen: роутер, middleware и хендлеры
package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/api"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

// Pinger проверяет доступность зависимости (БД) для health-check
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps зависимости роутера
type Deps struct {
	Logger     *slog.Logger
	DB         Pinger
	Catalog    CatalogService
	Partner    PartnerService
	AdminToken string
}

// NewRouter собирает chi-роутер со всеми маршрутами сервиса
func NewRouter(deps Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestLogger(deps.Logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/health", healthHandler(deps.DB))

	server := NewServer(deps.Catalog, deps.Partner)
	strict := api.NewStrictHandlerWithOptions(server, nil, api.StrictHTTPServerOptions{
		// Ошибка разбора запроса
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, r, domain.Invalid("Некорректный запрос").Wrap(err))
		},
		// Ошибка хендлера
		ResponseErrorHandlerFunc: writeError,
	})

	api.HandlerWithOptions(strict, api.ChiServerOptions{
		BaseRouter:  r,
		Middlewares: []api.MiddlewareFunc{authenticate(deps.Partner, deps.AdminToken)},
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, r, domain.Invalid("Некорректные параметры запроса").Wrap(err))
		},
	})

	return r
}

func healthHandler(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "db": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
