// Package sshmgr owns fvmux's SSH host directory and connection
// helpers. Hosts come from two sources merged in order:
//
//  1. ~/.ssh/config         — parsed via kevinburke/ssh_config.
//  2. ~/.config/fvmux/hosts.toml — fvmux-managed extras and overrides.
//
// On alias collision, hosts.toml wins (Source = "hosts.toml" for the
// badge column in the picker).
package sshmgr

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/kevinburke/ssh_config"
)

// Host is one merged entry — what the picker, the pool, and the SFTP
// browser all consume.
type Host struct {
	Alias    string
	User     string
	Hostname string
	Port     string
	Tags     []string
	Notes    string
	Source   string // "ssh_config" | "hosts.toml"
}

// Load returns the merged host list. A missing ~/.ssh/config is not an
// error (empty list from that source); a missing hosts.toml is not an
// error either. Other read/parse errors are returned along with as much
// data as was successfully gathered.
func Load(hostsTOMLPath string) ([]*Host, error) {
	var errs []string

	sshCfgPath := defaultSSHConfigPath()
	fromCfg, err := loadSSHConfig(sshCfgPath)
	if err != nil {
		errs = append(errs, "ssh_config: "+err.Error())
	}
	fromTOML, err := loadHostsTOML(hostsTOMLPath)
	if err != nil {
		errs = append(errs, "hosts.toml: "+err.Error())
	}

	byAlias := map[string]*Host{}
	for _, h := range fromCfg {
		byAlias[h.Alias] = h
	}
	for _, h := range fromTOML {
		byAlias[h.Alias] = h // hosts.toml wins on collision
	}
	out := make([]*Host, 0, len(byAlias))
	for _, h := range byAlias {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })

	if len(errs) > 0 {
		return out, errors.New(strings.Join(errs, "; "))
	}
	return out, nil
}

func defaultSSHConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "config")
}

func loadSSHConfig(path string) ([]*Host, error) {
	if path == "" {
		return nil, nil
	}
	// Gather the main config plus any Include'd files, then decode the
	// concatenation. kevinburke/ssh_config does not expand Include
	// directives itself, so users who split their config (Include
	// ~/.ssh/config.d/*) would otherwise have those hosts vanish from
	// the picker. Inlining the contents surfaces them; aliases are
	// deduped by the caller, so any double-counting is harmless.
	data, err := gatherSSHConfig(path, filepath.Dir(path), map[string]bool{}, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cfg, err := ssh_config.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var out []*Host
	for _, h := range cfg.Hosts {
		for _, pat := range h.Patterns {
			alias := pat.String()
			if strings.ContainsAny(alias, "*?") {
				continue // pattern-only entries (Host *) are not selectable.
			}
			user, _ := cfg.Get(alias, "User")
			host, _ := cfg.Get(alias, "Hostname")
			port, _ := cfg.Get(alias, "Port")
			if host == "" {
				host = alias
			}
			if port == "" {
				port = "22"
			}
			if _, err := strconv.Atoi(port); err != nil {
				port = "22"
			}
			out = append(out, &Host{
				Alias:    alias,
				User:     user,
				Hostname: host,
				Port:     port,
				Source:   "ssh_config",
			})
		}
	}
	return out, nil
}

const maxIncludeDepth = 16

// gatherSSHConfig reads path and recursively inlines any Include'd files,
// returning the concatenated bytes. sshBaseDir is the directory relative
// Include patterns resolve against (~/.ssh for a user config, matching
// ssh's own rule). visited guards against include loops; depth caps
// pathological nesting. A missing included file is skipped (as ssh does);
// only a missing top-level file (depth 0) surfaces ErrNotExist.
func gatherSSHConfig(path, sshBaseDir string, visited map[string]bool, depth int) ([]byte, error) {
	if depth > maxIncludeDepth {
		return nil, nil
	}
	abs, _ := filepath.Abs(path)
	if visited[abs] {
		return nil, nil
	}
	visited[abs] = true

	data, err := os.ReadFile(path)
	if err != nil {
		if depth == 0 {
			return nil, err
		}
		return nil, nil
	}

	var buf bytes.Buffer
	buf.Write(data)
	buf.WriteByte('\n')
	for _, pat := range parseIncludes(data) {
		for _, f := range expandIncludePattern(pat, sshBaseDir) {
			if more, _ := gatherSSHConfig(f, sshBaseDir, visited, depth+1); len(more) > 0 {
				buf.Write(more)
				buf.WriteByte('\n')
			}
		}
	}
	return buf.Bytes(), nil
}

// parseIncludes returns every whitespace-separated token following an
// Include keyword across the config text. Surrounding quotes are trimmed.
func parseIncludes(data []byte) []string {
	var out []string
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "Include") {
			continue
		}
		for _, tok := range fields[1:] {
			out = append(out, strings.Trim(tok, `"'`))
		}
	}
	return out
}

// expandIncludePattern resolves ~ and relative paths (against sshBaseDir)
// then glob-expands the pattern into concrete file paths.
func expandIncludePattern(pat, sshBaseDir string) []string {
	if strings.HasPrefix(pat, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			pat = filepath.Join(home, pat[2:])
		}
	}
	if !filepath.IsAbs(pat) {
		pat = filepath.Join(sshBaseDir, pat)
	}
	matches, _ := filepath.Glob(pat)
	return matches
}

// HostsFile is the on-disk schema for hosts.toml.
type HostsFile struct {
	Hosts []hostTOML `toml:"host"`
}

type hostTOML struct {
	Alias string   `toml:"alias"`
	User  string   `toml:"user"`
	Host  string   `toml:"host"`
	Port  int      `toml:"port"`
	Tags  []string `toml:"tags"`
	Notes string   `toml:"notes"`
}

func loadHostsTOML(path string) ([]*Host, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var hf HostsFile
	if err := toml.Unmarshal(data, &hf); err != nil {
		return nil, err
	}
	out := make([]*Host, 0, len(hf.Hosts))
	seen := map[string]bool{}
	for _, h := range hf.Hosts {
		alias := strings.TrimSpace(h.Alias)
		if alias == "" {
			continue // an entry with no alias is unusable; skip it.
		}
		if seen[alias] {
			continue // intra-file duplicate: first definition wins.
		}
		seen[alias] = true
		// Default to 22; reject out-of-range ports rather than emitting a
		// bogus "-5"/"99999" the ssh subprocess would choke on.
		port := "22"
		if h.Port > 0 && h.Port <= 65535 {
			port = strconv.Itoa(h.Port)
		}
		out = append(out, &Host{
			Alias:    alias,
			User:     h.User,
			Hostname: h.Host,
			Port:     port,
			Tags:     h.Tags,
			Notes:    h.Notes,
			Source:   "hosts.toml",
		})
	}
	return out, nil
}

// DisplayRow returns a single-line label suitable for the fuzzy picker.
func (h *Host) DisplayRow() string {
	var b strings.Builder
	b.WriteString(h.Alias)
	b.WriteString("  ")
	if h.User != "" {
		b.WriteString(h.User)
		b.WriteByte('@')
	}
	b.WriteString(h.Hostname)
	if h.Port != "" && h.Port != "22" {
		b.WriteByte(':')
		b.WriteString(h.Port)
	}
	if len(h.Tags) > 0 {
		b.WriteString("  [")
		b.WriteString(strings.Join(h.Tags, ","))
		b.WriteByte(']')
	}
	if h.Source != "" {
		b.WriteString("  (")
		b.WriteString(h.Source)
		b.WriteByte(')')
	}
	return b.String()
}
