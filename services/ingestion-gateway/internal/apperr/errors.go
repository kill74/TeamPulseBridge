package apperr

import "fmt"

type Code string

const (
	CodeInvalidRequestBody  Code = "ERR_INVALID_REQUEST_BODY"
	CodePayloadTooLarge     Code = "ERR_PAYLOAD_TOO_LARGE"
	CodeMethodNotAllowed    Code = "ERR_METHOD_NOT_ALLOWED"
	CodeUnauthorized        Code = "ERR_UNAUTHORIZED"
	CodeForbidden           Code = "ERR_FORBIDDEN"
	CodeMissingBearerToken  Code = "ERR_MISSING_BEARER_TOKEN"
	CodeInvalidToken        Code = "ERR_INVALID_TOKEN"
	CodeRateLimitExceeded   Code = "ERR_RATE_LIMIT_EXCEEDED"
	CodeInternalServerError Code = "ERR_INTERNAL_SERVER_ERROR"
	CodeQueueFull           Code = "ERR_QUEUE_FULL"
	CodePublishFailed       Code = "ERR_PUBLISH_FAILED"
	CodeDuplicateEvent      Code = "ERR_DUPLICATE_EVENT"
	CodeFailedEventStore    Code = "ERR_FAILED_EVENT_STORE"
	CodeReplayInputInvalid  Code = "ERR_REPLAY_INPUT_INVALID"
	CodeReplayEventNotFound Code = "ERR_REPLAY_EVENT_NOT_FOUND"
	CodeReplayConfigInvalid Code = "ERR_REPLAY_CONFIG_INVALID"
	CodeReplayPublishFailed Code = "ERR_REPLAY_PUBLISH_FAILED"
	CodeReplayReadFailed    Code = "ERR_REPLAY_READ_FAILED"
)

type Error struct {
	Code    Code
	Message string
	Op      string
	Err     error
}

func (e *Error) Error() string {
	switch {
	case e == nil:
		return ""
	case e.Op != "" && e.Err != nil:
		return fmt.Sprintf("%s: %s: %v", e.Op, e.Message, e.Err)
	case e.Op != "":
		return fmt.Sprintf("%s: %s", e.Op, e.Message)
	case e.Err != nil:
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	default:
		return e.Message
	}
}

func (e *Error) Unwrap() error { return e.Err }

func New(op string, code Code, message string, err error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Op:      op,
		Err:     err,
	}
}

func CodeOf(err error) Code {
	for err != nil {
		if ae, ok := err.(*Error); ok {
			return ae.Code
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return ""
}
