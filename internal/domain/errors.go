package domain

import "errors"

func NewValidationError(message string) error {
	return errors.New(message)
}
