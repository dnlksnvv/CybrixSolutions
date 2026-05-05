package errors

// ErrorType — верхнеуровневый тип ошибки по контракту API.
type ErrorType string

const (
	ErrorTypeValidation ErrorType = "validation_error"
	ErrorTypeBusiness   ErrorType = "business_error"
	ErrorTypeNotFound   ErrorType = "not_found"
	ErrorTypeInternal   ErrorType = "internal_error"
)

// APIError — единый формат ошибки для transport слоя.
// Важно: field и limit могут быть nil, если неприменимы.
type APIError struct {
	Type    ErrorType `json:"type"`
	Field   *string   `json:"field"`
	Code    string    `json:"code"`
	Message string    `json:"message"`
	Limit   *int      `json:"limit"`
}

func (e APIError) Error() string {
	return string(e.Type) + ":" + e.Code + ":" + e.Message
}

// NewValidation создаёт ошибку валидации с указанием поля.
func NewValidation(field, code, message string, limit *int) APIError {
	f := field
	return APIError{Type: ErrorTypeValidation, Field: &f, Code: code, Message: message, Limit: limit}
}

// NewBusiness создаёт бизнес-ошибку (ограничение состояния).
func NewBusiness(code, message string) APIError {
	return APIError{Type: ErrorTypeBusiness, Field: nil, Code: code, Message: message, Limit: nil}
}

// NewNotFound создаёт ошибку отсутствия ресурса.
func NewNotFound(code, message string) APIError {
	return APIError{Type: ErrorTypeNotFound, Field: nil, Code: code, Message: message, Limit: nil}
}

// NewInternal создаёт внутреннюю ошибку (не раскрывает детали наружу).
func NewInternal(code, message string) APIError {
	return APIError{Type: ErrorTypeInternal, Field: nil, Code: code, Message: message, Limit: nil}
}

