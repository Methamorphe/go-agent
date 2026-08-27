package errs

import (
	"errors"
	"fmt"
)

type Code string

const (
	CodeInvalidArgument   Code = "invalid_argument"
	CodeNotFound          Code = "not_found"
	CodeConflict          Code = "conflict"
	CodePermissionDenied  Code = "permission_denied"
	CodeResourceExhausted Code = "resource_exhausted"
	CodeUnavailable       Code = "unavailable"
	CodeCorruption        Code = "corruption"
	CodeUnsupported       Code = "unsupported"
	CodeInternal          Code = "internal"
)

type Error struct {
	Code    Code
	Op      string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}

	if e.Op != "" && e.Message != "" {
		return fmt.Sprintf(
			"%s: %s: %s",
			e.Code,
			e.Op,
			e.Message,
		)
	}

	if e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Cause, e.Message)
	}

	if e.Op != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Op)
	}

	return string(e.Code)
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func New(
	code Code,
	op string,
	message string,
) error {
	return &Error{
		Code:    code,
		Op:      op,
		Message: message,
	}
}

func Wrap(
	code Code,
	op string,
	message string,
	cause error,
) error {
	if cause == nil {
		return New(code, op, message)
	}

	return &Error{
		Code:    code,
		Op:      op,
		Message: message,
		Cause:   cause,
	}
}

func CodeOf(err error) Code {
	if err == nil {
		return ""
	}

	var target *Error

	if errors.As(err, &target) {
		return target.Code
	}

	return CodeInternal
}

func IsCode(err error, code Code) bool {
	return CodeOf(err) == code
}
