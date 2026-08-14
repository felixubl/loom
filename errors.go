package loom

import "fmt"

// Code names a semantic error. The spec (§34) fixes the first seven names and
// calls them a minimum, so UnknownValue is added here for a ValueID that does
// not belong to the store it was handed to.
//
// Syntax errors are not in this set: they belong to the parser, and Parse
// reports them as *SyntaxError.
type Code string

const (
	UnboundVariable       Code = "unbound_variable"
	NonPersistableValue   Code = "non_persistable_value"
	UnknownClaim          Code = "unknown_claim"
	AlreadyRetractedClaim Code = "already_retracted_claim"
	InvalidPattern        Code = "invalid_pattern"
	ResourceLimit         Code = "resource_limit"
	PrimitiveError        Code = "primitive_error"
	UnknownValue          Code = "unknown_value"
	UnresolvedName        Code = "unresolved_name"
	CyclicDefinition      Code = "cyclic_definition"
	DuplicateDefinition   Code = "duplicate_definition"
)

// Error is a semantic error carrying one of the codes above.
type Error struct {
	Code   Code
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return string(e.Code)
	}
	return string(e.Code) + ": " + e.Detail
}

// Is matches on the code alone, so errors.Is(err, ErrUnboundVariable) succeeds
// whatever detail the raised error carries.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Detail == "" && t.Code == e.Code
}

// Sentinels for errors.Is. They carry no detail, which is what makes them match
// any error of the same code.
var (
	ErrUnboundVariable       = &Error{Code: UnboundVariable}
	ErrNonPersistableValue   = &Error{Code: NonPersistableValue}
	ErrUnknownClaim          = &Error{Code: UnknownClaim}
	ErrAlreadyRetractedClaim = &Error{Code: AlreadyRetractedClaim}
	ErrInvalidPattern        = &Error{Code: InvalidPattern}
	ErrResourceLimit         = &Error{Code: ResourceLimit}
	ErrPrimitiveError        = &Error{Code: PrimitiveError}
	ErrUnknownValue          = &Error{Code: UnknownValue}
	ErrUnresolvedName        = &Error{Code: UnresolvedName}
	ErrCyclicDefinition      = &Error{Code: CyclicDefinition}
	ErrDuplicateDefinition   = &Error{Code: DuplicateDefinition}
)

func errorf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Detail: fmt.Sprintf(format, args...)}
}

// SyntaxError reports malformed source. Pos is a byte offset into the input.
type SyntaxError struct {
	Pos int
	Msg string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("syntax error at %d: %s", e.Pos, e.Msg)
}
