package http

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

// Заголовки аутентификации трёх групп API
const (
	headerUserID     = "X-User-Id"
	headerAPIKey     = "X-Api-Key" //nolint:gosec
	headerAdminToken = "X-Admin-Token"
)

// Префиксы путей, определяющие способ аутентификации запроса
const (
	prefixClient  = "/api/v1"
	prefixPartner = "/partner/v1"
	prefixAdmin   = "/admin/v1"
)

type contextKey int

const (
	contextKeyUserID contextKey = iota
	contextKeyRestaurant
)

// Authenticator проверяет API-ключ заведения
type Authenticator interface {
	Authenticate(ctx context.Context, apiKey string) (domain.Restaurant, error)
}

// UserIDFrom возвращает идентификатор пользователя, положенный middleware
func UserIDFrom(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(contextKeyUserID).(uuid.UUID)
	return id, ok
}

// RestaurantFrom возвращает заведение, аутентифицированное по API-ключу
func RestaurantFrom(ctx context.Context) (domain.Restaurant, bool) {
	restaurant, ok := ctx.Value(contextKeyRestaurant).(domain.Restaurant)
	return restaurant, ok
}

// mustRestaurant достаёт заведение из контекста
func mustRestaurant(ctx context.Context) (domain.Restaurant, error) {
	restaurant, ok := RestaurantFrom(ctx)
	if !ok {
		return domain.Restaurant{}, domain.ErrUnauthorized
	}
	return restaurant, nil
}

// authenticate выбирает способ аутентификации по префиксу пути
func authenticate(partners Authenticator, adminToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			switch {
			case strings.HasPrefix(r.URL.Path, prefixClient):
				raw := r.Header.Get(headerUserID)
				userID, err := uuid.Parse(raw)
				if err != nil {
					writeError(w, r, domain.ErrUnauthorized.WithDetails(domain.Detail{
						Code:    domain.CodeUnauthorized,
						Message: "Заголовок X-User-Id должен содержать UUID пользователя",
						Field:   headerUserID,
					}))
					return
				}
				ctx = context.WithValue(ctx, contextKeyUserID, userID)

			case strings.HasPrefix(r.URL.Path, prefixPartner):
				restaurant, err := partners.Authenticate(ctx, r.Header.Get(headerAPIKey))
				if err != nil {
					writeError(w, r, err)
					return
				}
				ctx = context.WithValue(ctx, contextKeyRestaurant, restaurant)

			case strings.HasPrefix(r.URL.Path, prefixAdmin):
				provided := r.Header.Get(headerAdminToken)
				if subtle.ConstantTimeCompare([]byte(provided), []byte(adminToken)) != 1 {
					writeError(w, r, domain.ErrUnauthorized)
					return
				}
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
