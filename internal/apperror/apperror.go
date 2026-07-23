// Package apperror provides typed pipeline errors with classification,
// cause chaining, and metadata. Every error in the cep-seed pipeline that
// represents a business, schema, or operation boundary should use these types
// instead of raw fmt.Errorf.
package apperror

import (
	"errors"
	"fmt"
)

// Kind classifies an error for caller handling and exit-code mapping.
type Kind int

const (
	KindSchema    Kind = iota + 1 // Schema contract violation
	KindDownload                  // Download failure (network, size limit)
	KindArchive                   // Archive extraction failure (zip-slip, size, missing entry)
	KindParse                     // File parsing failure
	KindCollision                 // CEP collision
	KindSkip                      // Not an error — signals skip
	KindInternal                  // Unexpected internal error
)

func (k Kind) String() string {
	switch k {
	case KindSchema:
		return "schema"
	case KindDownload:
		return "download"
	case KindArchive:
		return "archive"
	case KindParse:
		return "parse"
	case KindCollision:
		return "collision"
	case KindSkip:
		return "skip"
	case KindInternal:
		return "internal"
	default:
		return fmt.Sprintf("kind(%d)", k)
	}
}

// Error is a typed pipeline error with classification, cause, and metadata.
type Error struct {
	Kind     Kind
	Cause    error
	Message  string
	Metadata map[string]string
}

// New creates a new apperror with the given kind and message.
func New(kind Kind, msg string) *Error {
	return &Error{Kind: kind, Message: msg}
}

// Newf is like New but uses fmt.Sprintf for the message.
func Newf(kind Kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// Wrap wraps an existing error with the given kind and additional message context.
// If err is nil, Wrap returns nil.
func Wrap(kind Kind, err error, msg string) *Error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Cause: err, Message: msg}
}

// Wrapf is like Wrap but uses fmt.Sprintf for the message.
func Wrapf(kind Kind, err error, format string, args ...any) *Error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Cause: err, Message: fmt.Sprintf(format, args...)}
}

func (e *Error) Error() string {
	if e.Cause != nil {
		if e.Message != "" {
			return fmt.Sprintf("%s: %v", e.Message, e.Cause)
		}
		return e.Cause.Error()
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Kind.String()
}

// Unwrap returns the wrapped cause error.
func (e *Error) Unwrap() error { return e.Cause }

// metadata accessors

const (
	mdKeyFile    = "file"
	mdKeyLine    = "line"
	mdKeyRelease = "release"
	mdKeyCEP     = "cep"
	mdKeyLimit   = "limit"
	mdKeySize    = "size"
)

// WithFile returns a copy of e with the file metadata set.
func (e *Error) WithFile(file string) *Error { return e.withMeta(mdKeyFile, file) }

// WithLine returns a copy of e with the line metadata set.
func (e *Error) WithLine(line int) *Error {
	return e.withMeta(mdKeyLine, fmt.Sprintf("%d", line))
}

// WithRelease returns a copy of e with the release metadata set.
func (e *Error) WithRelease(release string) *Error {
	return e.withMeta(mdKeyRelease, release)
}

// WithCEP returns a copy of e with the CEP metadata set.
func (e *Error) WithCEP(cep string) *Error { return e.withMeta(mdKeyCEP, cep) }

func (e *Error) withMeta(key, value string) *Error {
	if e == nil {
		return nil
	}
	cp := &Error{Kind: e.Kind, Cause: e.Cause, Message: e.Message}
	if e.Metadata != nil {
		cp.Metadata = make(map[string]string, len(e.Metadata)+1)
		for k, v := range e.Metadata {
			cp.Metadata[k] = v
		}
	} else {
		cp.Metadata = make(map[string]string, 1)
	}
	cp.Metadata[key] = value
	return cp
}

// File returns the file metadata value if set.
func (e *Error) File() string {
	if e == nil {
		return ""
	}
	return e.Metadata[mdKeyFile]
}

// Line returns the line metadata value if set.
func (e *Error) Line() int {
	if e == nil {
		return 0
	}
	v := e.Metadata[mdKeyLine]
	if v == "" {
		return 0
	}
	var n int
	fmt.Sscanf(v, "%d", &n)
	return n
}

// KindFrom extracts the Kind from an error chain. Returns KindInternal if
// no typed error is found in the chain.
func KindFrom(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindInternal
}

// IsKind reports whether err has the given kind anywhere in its chain.
func IsKind(err error, kind Kind) bool {
	if err == nil {
		return false
	}
	e := &Error{Kind: kind}
	return errors.Is(err, e)
}

// Is implements errors.Is matching by Kind.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Kind == t.Kind
}
