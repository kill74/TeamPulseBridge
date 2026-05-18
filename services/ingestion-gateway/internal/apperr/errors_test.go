package apperr

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestError_Error(t *testing.T) {
	inner := errors.New("inner error")
	
	tests := []struct {
		name     string
		err      *Error
		expected string
	}{
		{
			name:     "full error",
			err:      New("op", CodeInternalServerError, "msg", inner),
			expected: "op: msg: inner error",
		},
		{
			name:     "no op",
			err:      &Error{Code: CodeInternalServerError, Message: "msg", Err: inner},
			expected: "msg: inner error",
		},
		{
			name:     "no inner",
			err:      &Error{Code: CodeInternalServerError, Message: "msg", Op: "op"},
			expected: "op: msg",
		},
		{
			name:     "only msg",
			err:      &Error{Code: CodeInternalServerError, Message: "msg"},
			expected: "msg",
		},
		{
			name:     "nil",
			err:      nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestCodeOf(t *testing.T) {
	inner := New("op", CodeUnauthorized, "unauthorized", nil)
	wrapped := New("op2", CodeInternalServerError, "wrapped", inner)
	stdErr := errors.New("std error")

	assert.Equal(t, CodeInternalServerError, CodeOf(wrapped))
	assert.Equal(t, CodeUnauthorized, CodeOf(inner))
	assert.Equal(t, Code(""), CodeOf(stdErr))
	assert.Equal(t, Code(""), CodeOf(nil))
}
