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
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	cfg, err := ssh_config.Decode(f)
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
	for _, h := range hf.Hosts {
		port := strconv.Itoa(h.Port)
		if h.Port == 0 {
			port = "22"
		}
		out = append(out, &Host{
			Alias:    h.Alias,
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
