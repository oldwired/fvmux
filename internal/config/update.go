package config

import (
	"os"
	"strings"

	"github.com/oldwired/fvmux/internal/atomicfile"
)

// KV names one `key = "value"` assignment to persist into config.toml.
// Only string values are supported — which covers everything fvmux
// writes programmatically (theme, prefix_key, shell).
type KV struct {
	Section string // TOML table, e.g. "appearance"
	Key     string // bare key inside the table, e.g. "theme"
	Value   string // written as a TOML basic string
}

// UpdateKeys surgically edits key assignments in the TOML file at path,
// preserving comments, blank lines, ordering, and unknown keys. This is
// what every programmatic persist must use instead of re-marshaling the
// Config struct, which would flatten the deliberately-commented seeded
// template (and drop any keys the user added) on the very first theme
// or prefix change.
//
// A key already present in its section is rewritten in place (an inline
// trailing comment survives); a missing key is appended at the end of
// its section; a missing section is appended at the end of the file. A
// missing file is created. The write is atomic.
func UpdateKeys(path string, updates ...KV) error {
	if len(updates) == 0 {
		return nil
	}
	perm := os.FileMode(0o644)
	var lines []string
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if fi, serr := os.Stat(path); serr == nil {
			perm = fi.Mode().Perm()
		}
		lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		if len(lines) == 1 && lines[0] == "" {
			lines = nil
		}
	case os.IsNotExist(err):
		// Fall through with no lines: sections are synthesized below.
	default:
		return err
	}

	for _, kv := range updates {
		lines = applyKV(lines, kv)
	}
	return atomicfile.Write(path, []byte(strings.Join(lines, "\n")+"\n"), perm)
}

// applyKV returns lines with kv applied, per the UpdateKeys contract.
func applyKV(lines []string, kv KV) []string {
	secStart, secEnd := sectionRange(lines, kv.Section)
	if secStart < 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "["+kv.Section+"]", kv.Key+" = "+quoteTOML(kv.Value))
		return lines
	}
	for i := secStart; i < secEnd; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, kv.Key) {
			continue
		}
		rest := strings.TrimSpace(trimmed[len(kv.Key):])
		if !strings.HasPrefix(rest, "=") {
			continue
		}
		indent := lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))]
		comment := trailingComment(rest[1:])
		lines[i] = indent + kv.Key + " = " + quoteTOML(kv.Value) + comment
		return lines
	}
	// Key absent: append after the last non-blank line of the section so
	// it lands before the blank gap separating the next table.
	insert := secStart
	for i := secStart; i < secEnd; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			insert = i
		}
	}
	out := append(lines[:insert+1:insert+1], kv.Key+" = "+quoteTOML(kv.Value))
	return append(out, lines[insert+1:]...)
}

// sectionRange returns the half-open line range of the section's body
// (first line after the [section] header, up to the next header or
// EOF). Start is -1 when the section header doesn't exist.
func sectionRange(lines []string, section string) (start, end int) {
	start = -1
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if !strings.HasPrefix(t, "[") || strings.HasPrefix(t, "[[") {
			continue
		}
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(t, "["), "]"))
		if start >= 0 {
			return start, i
		}
		if name == section {
			start = i + 1
		}
	}
	if start >= 0 {
		return start, len(lines)
	}
	return -1, -1
}

// trailingComment extracts the "  # …" tail following a TOML value so a
// rewrite keeps the user's inline comment. rest is everything after the
// '=' of an assignment. Only basic strings and unquoted scalars appear
// in config.toml, so scanning for '#' outside quotes suffices.
func trailingComment(rest string) string {
	inString := false
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '\\':
			if inString {
				i++ // skip escaped char inside a basic string
			}
		case '"':
			inString = !inString
		case '#':
			if !inString {
				return " " + strings.TrimRight(rest[i:], " \t")
			}
		}
	}
	return ""
}

// quoteTOML renders v as a TOML basic string.
func quoteTOML(v string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range v {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
