package handlers

import (
	derr "github.com/cybrix-solutions/agents-service/internal/domain/errors"
)

// badJSON превращает ошибки биндинга Gin в единый формат ошибок API.
func badJSON(_ error) error {
	return derr.APIError{
		Type:    derr.ErrorTypeValidation,
		Field:   nil,
		Code:    "validation_error",
		Message: "invalid json body",
		Limit:   nil,
	}
}

