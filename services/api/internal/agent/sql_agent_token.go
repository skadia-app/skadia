package agent

import (
	"fmt"
	"strings"
	"unicode"
)

type SQLAnalysis struct {
	Tables         []string
	CTENames       []string
	MultiStatement bool
	UnsafeKeywords []string
}

func AnalyzeSQL(sql string) (SQLAnalysis, error) {
	tokens, multi, err := tokenizeSQL(sql)
	if err != nil {
		return SQLAnalysis{}, err
	}
	unsafe := unsafeSQLKeywords(tokens)

	ctes := extractCTENames(tokens)
	cteSet := make(map[string]bool, len(ctes))
	for _, cte := range ctes {
		cteSet[strings.ToLower(cte)] = true
	}

	var tables []string
	seen := map[string]bool{}
	for i := 0; i < len(tokens)-1; i++ {
		if !isKeyword(tokens[i], "from") && !isKeyword(tokens[i], "join") {
			continue
		}
		j := i + 1
		if j < len(tokens) && isKeyword(tokens[j], "lateral") {
			j++
		}
		if j >= len(tokens) || tokens[j] == "(" {
			continue
		}
		name := normalizeIdentifier(tokens[j])
		if name == "" || cteSet[strings.ToLower(name)] {
			continue
		}
		key := strings.ToLower(name)
		if !seen[key] {
			seen[key] = true
			tables = append(tables, name)
		}
	}

	return SQLAnalysis{Tables: tables, CTENames: ctes, MultiStatement: multi, UnsafeKeywords: unsafe}, nil
}

func unsafeSQLKeywords(tokens []string) []string {
	unsafeSet := map[string]bool{
		"insert": true,
		"update": true,
		"delete": true,
		"merge":  true,
		"alter":  true,
		"drop":   true,
		"create": true,
		"copy":   true,
		"do":     true,
	}
	seen := map[string]bool{}
	var out []string
	for _, token := range tokens {
		key := strings.ToLower(token)
		if unsafeSet[key] && !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	return out
}

func tokenizeSQL(sql string) ([]string, bool, error) {
	var tokens []string
	var b strings.Builder
	multi := false
	parenDepth := 0

	flush := func() {
		if b.Len() == 0 {
			return
		}
		tokens = append(tokens, b.String())
		b.Reset()
	}

	for i := 0; i < len(sql); {
		ch := sql[i]
		if ch == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			flush()
			i += 2
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			continue
		}
		if ch == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			flush()
			i += 2
			closed := false
			for i+1 < len(sql) {
				if sql[i] == '*' && sql[i+1] == '/' {
					i += 2
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, false, fmt.Errorf("unterminated block comment")
			}
			continue
		}
		if ch == '\'' {
			flush()
			i++
			for i < len(sql) {
				if sql[i] == '\'' {
					if i+1 < len(sql) && sql[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			if i > len(sql) {
				return nil, false, fmt.Errorf("unterminated string literal")
			}
			continue
		}
		if ch == '$' && i+1 < len(sql) && sql[i+1] == '$' {
			flush()
			i += 2
			closed := false
			for i+1 < len(sql) {
				if sql[i] == '$' && sql[i+1] == '$' {
					i += 2
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, false, fmt.Errorf("unterminated dollar quote")
			}
			continue
		}
		if ch == ';' {
			flush()
			multi = true
			i++
			continue
		}
		if ch == '(' || ch == ')' || ch == ',' {
			flush()
			if ch == '(' {
				parenDepth++
			}
			if ch == ')' && parenDepth > 0 {
				parenDepth--
			}
			tokens = append(tokens, string(ch))
			i++
			continue
		}
		r := rune(ch)
		if unicode.IsSpace(r) {
			flush()
			i++
			continue
		}
		b.WriteByte(ch)
		i++
	}
	flush()
	return tokens, multi, nil
}

func extractCTENames(tokens []string) []string {
	if len(tokens) < 3 || !isKeyword(tokens[0], "with") {
		return nil
	}
	var ctes []string
	for i := 1; i < len(tokens)-2; i++ {
		if tokens[i] == "," {
			continue
		}
		name := normalizeIdentifier(tokens[i])
		if name == "" {
			break
		}
		if !isKeyword(tokens[i+1], "as") || tokens[i+2] != "(" {
			break
		}
		ctes = append(ctes, name)
		depth := 0
		for ; i < len(tokens); i++ {
			if tokens[i] == "(" {
				depth++
			}
			if tokens[i] == ")" {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		if i+1 >= len(tokens) || tokens[i+1] != "," {
			break
		}
	}
	return ctes
}

func isKeyword(token, keyword string) bool {
	return strings.EqualFold(token, keyword)
}

func normalizeIdentifier(token string) string {
	token = strings.TrimSpace(token)
	if token == "" || token == "(" || token == ")" || token == "," {
		return ""
	}
	if strings.HasPrefix(token, `"`) && strings.HasSuffix(token, `"`) && len(token) >= 2 {
		return strings.Trim(token, `"`)
	}
	if idx := strings.IndexByte(token, '.'); idx >= 0 {
		parts := strings.Split(token, ".")
		for i, part := range parts {
			parts[i] = strings.Trim(part, `"`)
		}
		return strings.Join(parts, ".")
	}
	return strings.Trim(token, `"`)
}
