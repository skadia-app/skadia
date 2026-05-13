package agent

import (
	"fmt"
	"strings"
	"unicode"
)

type SQLAnalysis struct {
	Tables          []string
	CTENames        []string
	FirstToken      string
	MultiStatement  bool
	UnsafeKeywords  []string
	UnsafeFunctions []string
}

func AnalyzeSQL(sql string) (SQLAnalysis, error) {
	tokens, multi, err := tokenizeSQL(sql)
	if err != nil {
		return SQLAnalysis{}, err
	}
	unsafe := unsafeSQLKeywords(tokens)
	unsafeFunctions := unsafeSessionConfigFunctions(tokens)

	ctes := extractCTENames(tokens)
	cteSet := make(map[string]bool, len(ctes))
	for _, cte := range ctes {
		cteSet[strings.ToLower(cte)] = true
	}

	tables := extractTableNames(tokens, cteSet)

	firstToken := ""
	if len(tokens) > 0 {
		firstToken = tokens[0]
	}

	return SQLAnalysis{Tables: tables, CTENames: ctes, FirstToken: firstToken, MultiStatement: multi, UnsafeKeywords: unsafe, UnsafeFunctions: unsafeFunctions}, nil
}

func extractTableNames(tokens []string, cteSet map[string]bool) []string {
	var tables []string
	seen := map[string]bool{}
	inFromClause := map[int]bool{}
	depth := 0
	addTableAt := func(index int) {
		name := relationNameAt(tokens, index)
		if name == "" || cteSet[strings.ToLower(name)] {
			return
		}
		key := strings.ToLower(name)
		if seen[key] {
			return
		}
		seen[key] = true
		tables = append(tables, name)
	}

	for i, token := range tokens {
		switch token {
		case "(":
			depth++
			continue
		case ")":
			delete(inFromClause, depth)
			if depth > 0 {
				depth--
			}
			continue
		}
		if endsFromClause(token) {
			inFromClause[depth] = false
		}
		if isKeyword(token, "from") {
			inFromClause[depth] = true
			addTableAt(i + 1)
			continue
		}
		if isKeyword(token, "join") {
			addTableAt(i + 1)
			continue
		}
		if token == "," && inFromClause[depth] {
			addTableAt(i + 1)
		}
	}
	return tables
}

func relationNameAt(tokens []string, index int) string {
	for index < len(tokens) && (isKeyword(tokens[index], "lateral") || isKeyword(tokens[index], "only")) {
		index++
	}
	if index >= len(tokens) || tokens[index] == "(" {
		return ""
	}
	name := normalizeIdentifier(tokens[index])
	if name == "" || endsFromClause(name) {
		return ""
	}
	return name
}

func endsFromClause(token string) bool {
	switch strings.ToLower(token) {
	case "where", "group", "having", "order", "limit", "offset", "fetch", "union", "intersect", "except", "window", "returning", "for":
		return true
	default:
		return false
	}
}

func unsafeSQLKeywords(tokens []string) []string {
	unsafeSet := map[string]bool{
		"alter":      true,
		"analyze":    true,
		"begin":      true,
		"call":       true,
		"cluster":    true,
		"commit":     true,
		"copy":       true,
		"create":     true,
		"deallocate": true,
		"delete":     true,
		"discard":    true,
		"do":         true,
		"drop":       true,
		"execute":    true,
		"grant":      true,
		"insert":     true,
		"listen":     true,
		"load":       true,
		"lock":       true,
		"merge":      true,
		"notify":     true,
		"prepare":    true,
		"refresh":    true,
		"release":    true,
		"reset":      true,
		"revoke":     true,
		"rollback":   true,
		"savepoint":  true,
		"set":        true,
		"show":       true,
		"truncate":   true,
		"unlisten":   true,
		"update":     true,
		"vacuum":     true,
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

func unsafeSessionConfigFunctions(tokens []string) []string {
	unsafeSet := map[string]bool{
		"current_catalog":                  true,
		"current_database":                 true,
		"current_query":                    true,
		"current_schema":                   true,
		"current_schemas":                  true,
		"current_setting":                  true,
		"pg_backend_pid":                   true,
		"pg_conf_load_time":                true,
		"pg_current_logfile":               true,
		"pg_notification_queue_usage":      true,
		"set_config":                       true,
		"currval":                          true,
		"lastval":                          true,
		"nextval":                          true,
		"setval":                           true,
		"lo_close":                         true,
		"lo_creat":                         true,
		"lo_create":                        true,
		"lo_export":                        true,
		"lo_from_bytea":                    true,
		"lo_get":                           true,
		"lo_import":                        true,
		"lo_lseek":                         true,
		"lo_lseek64":                       true,
		"lo_open":                          true,
		"lo_put":                           true,
		"lo_tell":                          true,
		"lo_tell64":                        true,
		"lo_truncate":                      true,
		"lo_truncate64":                    true,
		"lo_unlink":                        true,
		"loread":                           true,
		"lowrite":                          true,
		"pg_advisory_lock":                 true,
		"pg_advisory_lock_shared":          true,
		"pg_advisory_unlock":               true,
		"pg_advisory_unlock_all":           true,
		"pg_advisory_unlock_shared":        true,
		"pg_advisory_xact_lock":            true,
		"pg_advisory_xact_lock_shared":     true,
		"pg_try_advisory_lock":             true,
		"pg_try_advisory_lock_shared":      true,
		"pg_try_advisory_xact_lock":        true,
		"pg_try_advisory_xact_lock_shared": true,
	}
	seen := map[string]bool{}
	var out []string
	for i, token := range tokens {
		key := lastIdentifierPart(normalizeIdentifier(token))
		if key == "" || !unsafeSet[key] || seen[key] {
			continue
		}
		if i+1 < len(tokens) && tokens[i+1] != "(" {
			continue
		}
		seen[key] = true
		out = append(out, key)
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

func lastIdentifierPart(identifier string) string {
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	if identifier == "" {
		return ""
	}
	parts := strings.Split(identifier, ".")
	return parts[len(parts)-1]
}
