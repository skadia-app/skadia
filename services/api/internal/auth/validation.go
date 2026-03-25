package auth

import "regexp"

// usernameRe matches valid usernames: starts with alphanumeric, rest alphanumeric or underscore.
var usernameValidRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_]*$`)

const (
	UsernameMinLen = 3
	UsernameMaxLen = 30
)

// ValidateUsername checks that a username is 3–30 ASCII alphanumeric/underscore chars,
// starting with an alphanumeric character. Returns an error key or "".
func ValidateUsername(username string) string {
	if len(username) < UsernameMinLen || len(username) > UsernameMaxLen {
		return "username_invalid_length"
	}
	if !usernameValidRe.MatchString(username) {
		return "username_invalid_chars"
	}
	return ""
}
