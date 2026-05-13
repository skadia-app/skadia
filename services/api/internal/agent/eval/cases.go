package eval

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"

	_ "embed"
)

//go:embed testdata/cases.json
var defaultCasesJSON []byte

func DefaultCases() []Case {
	cases, err := decodeCases(defaultCasesJSON)
	if err != nil {
		panic(err)
	}
	return cases
}

func DefaultSuiteHash() string {
	return hashSuiteBytes(defaultCasesJSON)
}

func hashSuiteBytes(raw []byte) string {
	sum := sha256.Sum256(bytes.TrimSpace(raw))
	return hex.EncodeToString(sum[:])
}
