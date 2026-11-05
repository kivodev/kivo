package fault

import (
	"errors"
)

// Kind is what sort of failure an error is.
type Kind int

const (
	// Internal is a failure of the server itself.
	Internal Kind = iota
	// Invalid is a request that is wrong as asked.
	Invalid
	// Unauthenticated is a request without valid credentials.
	Unauthenticated
	// Forbidden is a request the caller may not make.
	Forbidden
	// NotFound is a request for something that doesn't exist, or that the
	// caller may not see.
	NotFound
	// Conflict is a request that the current state refuses.
	Conflict
	// Gone is a request for something that has expired or been used.
	Gone
	// Throttled is a request refused until the caller waits.
	Throttled
	// Unavailable is a request that something the server depends on can't
	// serve right now.
	Unavailable
)

// code is each kind's own problem code.
var code = map[Kind]string{
	Internal:        "internal",
	Invalid:         "invalidRequest",
	Unauthenticated: "unauthenticated",
	Forbidden:       "forbidden",
	NotFound:        "notFound",
	Conflict:        "conflict",
	Gone:            "expired",
	Throttled:       "rateLimited",
	Unavailable:     "providerUnavailable",
}

// Error is a sentinel error of a kind, with the problem code that clients
// use to recognize it.
type Error struct {
	kind Kind
	code string
	msg  string
}

// New returns an error of kind whose problem code is the kind's own. msg
// names the condition, such as "not found", and a problem shows it as its
// detail.
func New(kind Kind, msg string) *Error {
	return &Error{kind: kind, code: code[kind], msg: msg}
}

// Coded returns an error of kind with its own problem code. Use it for an
// error that clients must tell apart from others of its kind.
func Coded(kind Kind, code, msg string) *Error {
	return &Error{kind: kind, code: code, msg: msg}
}

func (e *Error) Error() string { return e.msg }

// Kind returns the error's kind.
func (e *Error) Kind() Kind { return e.kind }

// Code returns the error's problem code.
func (e *Error) Code() string { return e.code }

// Of returns the first fault in err's tree.
func Of(err error) (*Error, bool) {
	return errors.AsType[*Error](err)
}

// Field is the part of a request that an error is about. Name is a JSON
// pointer into the body, or a parameter's name. Code says what is wrong
// with it.
type Field struct {
	Name, Code string
}

// About returns err as being about a field of the request.
func About(err error, f Field) error {
	return &aboutError{err: err, field: f}
}

// FieldOf returns the field err is about, if it is about one.
func FieldOf(err error) (Field, bool) {
	a, ok := errors.AsType[*aboutError](err)
	if !ok {
		return Field{}, false
	}
	return a.field, true
}

type aboutError struct {
	err   error
	field Field
}

func (e *aboutError) Error() string { return e.err.Error() }

func (e *aboutError) Unwrap() error { return e.err }
