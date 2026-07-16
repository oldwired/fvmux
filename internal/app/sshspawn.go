package app

import (
	"github.com/oldwired/fv-go/pkg/fv/geom"

	"github.com/oldwired/fvmux/internal/profile"
	"github.com/oldwired/fvmux/internal/session"
	"github.com/oldwired/fvmux/internal/sshmgr"
)

// sshProfile acquires an SSH pool ref for alias and synthesizes the
// ad-hoc interactive-ssh profile every spawn path shares (Ctrl-G H
// connect, SFTP auth-retry, session-restore fallback, respawn). h may
// be nil; a hosts.toml-sourced Host contributes its HostName/User/Port
// overrides so standalone hosts are actually connectable.
//
// The profile carries SSHAlias, so the pane spawned from it owns the
// pool ref: stopPane releases it when the pane's terminal stops, and
// instantiateProfile releases it if the spawn fails. This is the single
// place the recipe lives — hand-rolling it at a call site is how the
// pool's refcount invariant got broken before.
func (m *Mux) sshProfile(h *sshmgr.Host, alias string) *profile.Profile {
	sock := m.sshPool.Acquire(alias)
	args := append([]string{}, sshmgr.ControlOpts(sock)...)
	args = append(args, h.ConnectOpts()...)
	args = append(args, alias)
	return &profile.Profile{
		Name:     alias,
		Command:  "ssh",
		Args:     args,
		Title:    alias,
		SSHAlias: alias,
	}
}

// hostByAlias resolves alias against hosts.toml + ~/.ssh/config,
// returning nil when unknown. Used to thread a hosts.toml entry's
// connection fields into ssh/sftp spawns that only carry an alias.
func (m *Mux) hostByAlias(alias string) *sshmgr.Host {
	hosts, _ := sshmgr.Load(m.Opts.Paths.HostsFile())
	for _, h := range hosts {
		if h != nil && h.Alias == alias {
			return h
		}
	}
	return nil
}

// instantiateProfile wraps profile.Instantiate with the Mux's terminal
// defaults, releasing the profile's SSH pool ref on failure (on
// success the pane owns it — see stopPane). Every pane spawn in this
// package must come through here so the Acquire/Release pairing can't
// be forgotten at a call site.
func (m *Mux) instantiateProfile(prof *profile.Profile, bounds geom.Rect) (*session.Pane, error) {
	pane, err := profile.Instantiate(prof, bounds,
		m.Opts.Config.Terminal.ScrollbackLines, m.Opts.Config.Terminal.Shell)
	if err != nil && prof != nil && prof.SSHAlias != "" && m.sshPool != nil {
		m.sshPool.Release(prof.SSHAlias)
	}
	return pane, err
}
