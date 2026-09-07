// Package domain содержит сущности предметной области, правила переходов статусов заказа и доменные ошибки
package domain

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Kind категория доменной ошибки
type Kind int

const (
	// KindInvalid: некорректные входные данные (400)
	KindInvalid Kind = iota
	// KindUnauthorized: нет или неверные учётные данные (401)
	KindUnauthorized
	// KindNotFound: ресурс не найден (404)
	KindNotFound
	// KindConflict: бизнес-конфликт: нет остатков, недопустимый переход и т. п. (409)
	KindConflict
	// KindNotImplemented: сценарий ещё не реализован (501)
	KindNotImplemented
)

// Коды ошибок в поле `code` ответа API
const (
	CodeValidation             = "validation_error"
	CodeUnauthorized           = "unauthorized"
	CodeNotFound               = "not_found"
	CodeRestaurantUnavailable  = "restaurant_unavailable"
	CodeSlugTaken              = "slug_taken"
	CodeCartRestaurantMismatch = "cart_restaurant_mismatch"
	CodeCartEmpty              = "cart_empty"
	CodeMinOrderAmount         = "min_order_amount"
	CodeItemsUnavailable       = "items_unavailable"
	CodeInvalidTransition      = "invalid_transition"
	CodeNotImplemented         = "not_implemented"
)

// Detail уточнение ошибки: проблемное поле или конкретная позиция меню
type Detail struct {
	Code         string
	Message      string
	Field        string
	MenuItemID   uuid.UUID
	RequestedQty int
	AvailableQty int
}

// Error доменная ошибка с кодом и опциональными деталями
type Error struct {
	Kind    Kind
	Code    string
	Message string
	Details []Detail
	err     error
}

func (e *Error) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap возвращает обёрнутую техническую ошибку, если она есть
func (e *Error) Unwrap() error { return e.err }

// WithDetails возвращает копию ошибки с добавленными деталями
func (e *Error) WithDetails(details ...Detail) *Error {
	cp := *e
	cp.Details = append(append([]Detail(nil), e.Details...), details...)
	return &cp
}

// Wrap возвращает копию ошибки с обёрнутой технической причиной
func (e *Error) Wrap(err error) *Error {
	cp := *e
	cp.err = err
	return &cp
}

// AsError извлекает доменную ошибку из цепочки errors
func AsError(err error) (*Error, bool) {
	var domainErr *Error
	if errors.As(err, &domainErr) {
		return domainErr, true
	}
	return nil, false
}

func newError(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// Invalid возвращает ошибку валидации входных данных
func Invalid(message string, details ...Detail) *Error {
	return &Error{Kind: KindInvalid, Code: CodeValidation, Message: message, Details: details}
}

// NotFound возвращает ошибку "ресурс не найден"
func NotFound(message string) *Error {
	return newError(KindNotFound, CodeNotFound, message)
}

// Conflict возвращает бизнес-конфликт с заданным кодом
func Conflict(code, message string) *Error {
	return newError(KindConflict, code, message)
}

// Общие ошибки для нескольких сценариев
var (
	// ErrUnauthorized: отсутствует или неверен заголовок аутентификации
	ErrUnauthorized = newError(KindUnauthorized, CodeUnauthorized, "Требуется аутентификация")
	// ErrRestaurantNotFound: заведение не найдено или недоступно клиенту
	ErrRestaurantNotFound = NotFound("Заведение не найдено")
	// ErrMenuItemNotFound: позиция меню не найдена у этого заведения
	ErrMenuItemNotFound = NotFound("Позиция меню не найдена")
	// ErrSlugTaken: заведение с таким slug уже существует
	ErrSlugTaken = Conflict(CodeSlugTaken, "Заведение с таким slug уже существует")
	// ErrCartRestaurantMismatch: в корзине позиции другого заведения
	ErrCartRestaurantMismatch = Conflict(CodeCartRestaurantMismatch,
		"В корзине уже есть позиции другого заведения")
	// ErrCartEmpty: попытка оформить заказ из пустой корзины
	ErrCartEmpty = Conflict(CodeCartEmpty, "Корзина пуста")
	// ErrOrderNotFound: заказ не найден или принадлежит другому пользователю
	ErrOrderNotFound = NotFound("Заказ не найден")
	// ErrRestaurantUnavailable: заведение не принимает заказы
	ErrRestaurantUnavailable = Conflict(CodeRestaurantUnavailable,
		"Заведение сейчас не принимает заказы")
	// ErrItemsUnavailable: часть позиций нельзя заказать в нужном количестве
	ErrItemsUnavailable = Conflict(CodeItemsUnavailable, "Некоторые позиции недоступны")
	// ErrMinOrderAmount: сумма заказа меньше минимальной для заведения
	ErrMinOrderAmount = Conflict(CodeMinOrderAmount, "Сумма заказа меньше минимальной")
	// ErrNotImplemented: эндпоинт объявлен в OpenAPI, но ещё не реализован
	ErrNotImplemented = newError(KindNotImplemented, CodeNotImplemented, "Сценарий ещё не реализован")
)
