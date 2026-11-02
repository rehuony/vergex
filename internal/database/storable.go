package database

import "strings"

// StorableText makes model output safe for a PostgreSQL text column, which rejects
// NUL bytes and invalid UTF-8.
func StorableText(text string) string {
	return strings.ToValidUTF8(strings.ReplaceAll(text, "\x00", ""), "\uFFFD")
}
