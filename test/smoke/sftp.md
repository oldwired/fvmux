# fvmux v1 — SFTP Smoke Checklist

Bring up a local SSH server first:

```bash
smoke_key_dir="$(mktemp -d)"
ssh-keygen -q -t ed25519 -N '' -f "$smoke_key_dir/id_ed25519"
export FVMUX_SMOKE_PUBLIC_KEY="$(cat "$smoke_key_dir/id_ed25519.pub")"
docker compose -f test/smoke/docker-compose.yml up -d sshd
# Hostname: localhost  Port: 2222  User: fvmux; password login is disabled.
ssh -i "$smoke_key_dir/id_ed25519" -p 2222 fvmux@localhost
```

Add an entry to `~/.ssh/config`:

```
Host fvmux-smoke
  HostName localhost
  Port 2222
  User fvmux
  IdentityFile /replace/with/the/value/of/$smoke_key_dir/id_ed25519
  IdentitiesOnly yes
```

Then in fvmux:

- [ ] `Ctrl-G H` lists `fvmux-smoke`; selecting it spawns an ssh pane.
- [ ] `Ctrl-G F` lists `fvmux-smoke`; selecting it opens an SFTP browser
      rooted at the remote home directory.
- [ ] Expanding a directory in the tree lazy-loads its contents.
- [ ] Navigating to `/bin/ls` on the remote opens a hex preview
      (sub-step 12 polish; step-10 build shows the tree only).
- [ ] Closing the browser tears down the SFTP session and the ssh
      subprocess cleanly (verify with `ps aux | grep ssh`).

When finished, remove the container and ephemeral key:

```bash
docker compose -f test/smoke/docker-compose.yml down
rm -rf "$smoke_key_dir"
unset FVMUX_SMOKE_PUBLIC_KEY smoke_key_dir
```
