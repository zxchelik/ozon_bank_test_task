package service

import (
	"crypto/rand"
	"fmt"
	"io"
)

const (
	CodeLength = 10
	Alphabet   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_"
)

type CodeGenerator interface {
	Generate() (string, error)
}

type RandomGenerator struct {
	reader io.Reader
}

func NewRandomGenerator() *RandomGenerator {
	return &RandomGenerator{reader: rand.Reader}
}

func NewRandomGeneratorWithReader(reader io.Reader) *RandomGenerator {
	return &RandomGenerator{reader: reader}
}

func (g *RandomGenerator) Generate() (string, error) {
	result := make([]byte, CodeLength)
	for i := 0; i < len(result); {
		var b [1]byte
		if _, err := io.ReadFull(g.reader, b[:]); err != nil {
			return "", fmt.Errorf("read random bytes: %w", err)
		}
		if b[0] >= 252 {
			continue
		}
		result[i] = Alphabet[int(b[0])%len(Alphabet)]
		i++
	}
	return string(result), nil
}
