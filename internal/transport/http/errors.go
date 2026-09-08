package http

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/api"
	"github.com/talense-tasks/backend-trainee-assignment-autumn-2026-flow-2-damakalshchikov-3b6ddaba/internal/domain"
)

// statusFor отображает категорию доменной ошибки в HTTP-статус
func statusFor(kind domain.Kind) int {
	switch kind {
	case domain.KindInvalid:
		return http.StatusBadRequest
	case domain.KindUnauthorized:
		return http.StatusUnauthorized
	case domain.KindNotFound:
		return http.StatusNotFound
	case domain.KindConflict:
		return http.StatusConflict
	case domain.KindNotImplemented:
		return http.StatusNotImplemented
	default:
		return http.StatusInternalServerError
	}
}

// errorBody переводит доменную ошибку в схему Error из OpenAPI
func errorBody(err *domain.Error) api.Error {
	body := api.Error{Code: err.Code, Message: err.Message}
	if len(err.Details) == 0 {
		return body
	}

	details := make([]api.ErrorDetail, 0, len(err.Details))
	for _, d := range err.Details {
		detail := api.ErrorDetail{Code: d.Code, Message: d.Message}
		if d.Field != "" {
			detail.Field = ptr(d.Field)
		}
		if d.MenuItemID != uuid.Nil {
			detail.MenuItemId = ptr(d.MenuItemID)
		}
		if d.RequestedQty > 0 {
			detail.RequestedQty = ptr(d.RequestedQty)
		}
		if d.AvailableQty >= 0 && d.RequestedQty > 0 {
			detail.AvailableQty = ptr(d.AvailableQty)
		}
		details = append(details, detail)
	}
	body.Details = &details
	return body
}

// internalError тело ответа для непредвиденных ошибок
var internalError = api.Error{
	Code:    "internal_error",
	Message: "Внутренняя ошибка сервиса",
}

// writeError отправляет ошибку клиенту в едином формате Error
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	if domainErr, ok := domain.AsError(err); ok {
		writeJSON(w, statusFor(domainErr.Kind), errorBody(domainErr))
		return
	}

	slog.ErrorContext(r.Context(), "unhandled error",
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.String("error", err.Error()),
	)
	writeJSON(w, http.StatusInternalServerError, internalError)
}

func ptr[T any](v T) *T { return &v }
