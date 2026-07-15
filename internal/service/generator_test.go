package service

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestRandomGeneratorLengthAndAlphabet(t *testing.T) {
	generator := NewRandomGeneratorWithReader(strings.NewReader(string([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})))
	code, err := generator.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(code) != CodeLength {
		t.Fatalf("len(code) = %d, want %d", len(code), CodeLength)
	}
	if !ValidShortCode(code) {
		t.Fatalf("Generate() returned invalid code %q", code)
	}
}

func TestRandomGeneratorRejectsBiasedBytes(t *testing.T) {
	input := append([]byte{252, 253, 254, 255}, make([]byte, CodeLength)...)
	code, err := NewRandomGeneratorWithReader(strings.NewReader(string(input))).Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if code != strings.Repeat("a", CodeLength) {
		t.Fatalf("Generate() = %q, want all 'a'", code)
	}
}

func TestRandomGeneratorPropagatesReaderError(t *testing.T) {
	want := errors.New("entropy unavailable")
	_, err := NewRandomGeneratorWithReader(errorReader{err: want}).Generate()
	if !errors.Is(err, want) {
		t.Fatalf("Generate() error = %v, want wrapped %v", err, want)
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

var _ io.Reader = errorReader{}
