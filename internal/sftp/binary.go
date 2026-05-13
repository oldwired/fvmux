// Package sftp owns fvmux's remote-file browser, hex preview, and the
// thin SFTP client wrapper that ties them to a host alias.
package sftp

import (
	"bytes"
	"path/filepath"
	"strings"
)

// MediaKind classifies a file's contents for preview-pane dispatch.
type MediaKind uint8

const (
	KindText MediaKind = iota
	KindMarkdown
	KindImage
	KindBinary
)

// Sniff classifies the leading bytes of a file. Heuristics, in order:
//
//  1. Extension whitelist for text-y types → KindText.
//  2. Extension .md → KindMarkdown.
//  3. Extension among known image types → KindImage.
//  4. Null byte present in first 8 KiB → KindBinary.
//  5. ELF / Mach-O / ZIP / PDF magic in first bytes → KindBinary.
//  6. Printable-ASCII ratio < 0.85 across the first 4 KiB → KindBinary.
//  7. Otherwise → KindText.
func Sniff(name string, buf []byte) MediaKind {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".md", ".markdown":
		return KindMarkdown
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return KindImage
	case ".txt", ".log", ".conf", ".cfg", ".ini", ".yaml", ".yml", ".json", ".xml",
		".go", ".py", ".rs", ".c", ".cpp", ".h", ".hpp", ".ts", ".js", ".jsx", ".tsx",
		".sh", ".bash", ".zsh", ".lua", ".rb", ".pl", ".sql", ".csv":
		return KindText
	}

	sniff := buf
	if len(sniff) > 8192 {
		sniff = sniff[:8192]
	}
	if bytes.IndexByte(sniff, 0) >= 0 {
		return KindBinary
	}
	if len(sniff) >= 4 {
		// Common binary magic numbers.
		switch {
		case bytes.HasPrefix(sniff, []byte{0x7f, 'E', 'L', 'F'}): // ELF
			return KindBinary
		case bytes.HasPrefix(sniff, []byte{0xCF, 0xFA, 0xED, 0xFE}): // Mach-O LE
			return KindBinary
		case bytes.HasPrefix(sniff, []byte{0xFE, 0xED, 0xFA, 0xCE}): // Mach-O BE
			return KindBinary
		case bytes.HasPrefix(sniff, []byte("PK")):
			return KindBinary
		case bytes.HasPrefix(sniff, []byte("%PDF")):
			return KindBinary
		}
	}

	check := sniff
	if len(check) > 4096 {
		check = check[:4096]
	}
	if len(check) == 0 {
		return KindText
	}
	printable := 0
	for _, b := range check {
		if (b >= 0x20 && b < 0x7f) || b == '\n' || b == '\r' || b == '\t' {
			printable++
		}
	}
	ratio := float64(printable) / float64(len(check))
	if ratio < 0.85 {
		return KindBinary
	}
	return KindText
}
