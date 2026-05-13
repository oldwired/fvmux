# fvmux v1 — SFTP Smoke Checklist

Bring up a local SSH server first:

```bash
docker compose -f test/smoke/docker-compose.yml up -d sshd
# Hostname: localhost  Port: 2222  User: fvmux  Password: fvmux
ssh-copy-id -p 2222 fvmux@localhost   # one-time
```

Add an entry to `~/.ssh/config`:

```
Host fvmux-smoke
  HostName localhost
  Port 2222
  User fvmux
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
