package apperror_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/whoisclebs/cep-seed/internal/apperror"
)

func TestNew(test *testing.T) {
	err := apperror.New(apperror.KindSchema, "schema mismatch")
	if err.Kind != apperror.KindSchema {
		test.Errorf("Kind = %v, want KindSchema", err.Kind)
	}
	if err.Error() != "schema mismatch" {
		test.Errorf("Error() = %q, want %q", err.Error(), "schema mismatch")
	}
}

func TestNewf(test *testing.T) {
	err := apperror.Newf(apperror.KindDownload, "download %s failed", "file.zip")
	if err.Kind != apperror.KindDownload {
		test.Errorf("Kind = %v, want KindDownload", err.Kind)
	}
	if err.Error() != "download file.zip failed" {
		test.Errorf("Error() = %q", err.Error())
	}
}

func TestWrap(test *testing.T) {
	cause := fmt.Errorf("connection refused")
	err := apperror.Wrap(apperror.KindDownload, cause, "fetch archive")
	if err.Cause != cause {
		test.Error("Cause should be the wrapped error")
	}
	if err.Kind != apperror.KindDownload {
		test.Errorf("Kind = %v", err.Kind)
	}
	if !errors.Is(err, cause) {
		test.Error("errors.Is should find cause")
	}
}

func TestWrap_NilCause(test *testing.T) {
	err := apperror.Wrap(apperror.KindInternal, nil, "should be nil")
	if err != nil {
		test.Error("Wrap with nil cause should return nil")
	}
}

func TestWrapf(test *testing.T) {
	cause := fmt.Errorf("disk full")
	err := apperror.Wrapf(apperror.KindArchive, cause, "extract %s", "data.zip")
	if err.Kind != apperror.KindArchive {
		test.Errorf("Kind = %v, want KindArchive", err.Kind)
	}
	if err.Error() != "extract data.zip: disk full" {
		test.Errorf("Error() = %q, want %q", err.Error(), "extract data.zip: disk full")
	}
}

func TestError_Chain(test *testing.T) {
	inner := fmt.Errorf("original cause")
	middle := apperror.Wrap(apperror.KindParse, inner, "parse failed")
	outer := apperror.Wrap(apperror.KindInternal, middle, "pipeline error")

	if !errors.Is(outer, inner) {
		test.Error("errors.Is should find innermost cause")
	}

	var target *apperror.Error
	if !errors.As(outer, &target) {
		test.Fatal("errors.As should find *apperror.Error")
	}
	if target.Kind != apperror.KindInternal {
		test.Errorf("outer Kind = %v, want KindInternal", target.Kind)
	}
}

func TestKindFrom(test *testing.T) {
	tests := []struct {
		name string
		err  error
		want apperror.Kind
	}{
		{"schema error", apperror.New(apperror.KindSchema, "bad"), apperror.KindSchema},
		{"download error", apperror.New(apperror.KindDownload, "timeout"), apperror.KindDownload},
		{"archive error", apperror.New(apperror.KindArchive, "zip-slip"), apperror.KindArchive},
		{"parse error", apperror.New(apperror.KindParse, "bad field"), apperror.KindParse},
		{"collision error", apperror.New(apperror.KindCollision, "dup"), apperror.KindCollision},
		{"skip", apperror.New(apperror.KindSkip, "already applied"), apperror.KindSkip},
		{"internal", apperror.New(apperror.KindInternal, "unexpected"), apperror.KindInternal},
		{"wrapped", apperror.Wrap(apperror.KindSchema, fmt.Errorf("cause"), "ctx"), apperror.KindSchema},
		{"plain error", fmt.Errorf("plain"), apperror.KindInternal},
		{"nil error", nil, apperror.KindInternal},
	}
	for _, tt := range tests {
		test.Run(tt.name, func(subtest *testing.T) {
			if got := apperror.KindFrom(tt.err); got != tt.want {
				subtest.Errorf("KindFrom(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsKind(test *testing.T) {
	schemaErr := apperror.New(apperror.KindSchema, "bad schema")

	if !apperror.IsKind(schemaErr, apperror.KindSchema) {
		test.Error("IsKind should match KindSchema")
	}
	if apperror.IsKind(schemaErr, apperror.KindDownload) {
		test.Error("IsKind should not match KindDownload")
	}
	if apperror.IsKind(nil, apperror.KindSchema) {
		test.Error("IsKind(nil) should be false")
	}
}

func TestError_Metadata(test *testing.T) {
	err := apperror.New(apperror.KindParse, "bad file").
		WithFile("data.txt").
		WithLine(42)

	if err.File() != "data.txt" {
		test.Errorf("File() = %q", err.File())
	}
	if err.Line() != 42 {
		test.Errorf("Line() = %d", err.Line())
	}
}

func TestMetadata_OnNil(test *testing.T) {
	var err *apperror.Error = nil
	if f := err.File(); f != "" {
		test.Errorf("File() on nil = %q", f)
	}
	if l := err.Line(); l != 0 {
		test.Errorf("Line() on nil = %d", l)
	}
}

func TestError_Unwrap(test *testing.T) {
	cause := fmt.Errorf("root cause")
	err := apperror.Wrap(apperror.KindInternal, cause, "wrapper")
	if u := errors.Unwrap(err); u != cause {
		test.Errorf("Unwrap = %v, want %v", u, cause)
	}
}

func TestKind_String(test *testing.T) {
	tests := []struct {
		kind apperror.Kind
		want string
	}{
		{apperror.KindSchema, "schema"},
		{apperror.KindDownload, "download"},
		{apperror.KindArchive, "archive"},
		{apperror.KindParse, "parse"},
		{apperror.KindCollision, "collision"},
		{apperror.KindSkip, "skip"},
		{apperror.KindInternal, "internal"},
		{apperror.Kind(99), "kind(99)"},
	}
	for _, tt := range tests {
		test.Run(tt.want, func(subtest *testing.T) {
			if got := tt.kind.String(); got != tt.want {
				subtest.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWrapf_NilCause(test *testing.T) {
	err := apperror.Wrapf(apperror.KindInternal, nil, "should be nil")
	if err != nil {
		test.Error("Wrapf with nil cause should return nil")
	}
}

func TestIsKind_WrappedChain(test *testing.T) {
	inner := fmt.Errorf("network error")
	middle := apperror.Wrap(apperror.KindDownload, inner, "download failed")
	outer := fmt.Errorf("pipeline: %w", middle)

	if !apperror.IsKind(outer, apperror.KindDownload) {
		test.Error("IsKind should find KindDownload in wrapped chain")
	}
}
