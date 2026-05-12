package eval

import _ "embed"

//go:embed testdata/cases.json
var defaultCasesJSON []byte

func DefaultCases() []Case {
	cases, err := decodeCases(defaultCasesJSON)
	if err != nil {
		panic(err)
	}
	return cases
}
