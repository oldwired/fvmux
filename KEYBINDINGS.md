# fvmux Keyboard Reference

Global commands use the configured prefix. Contextual keys apply only in the named mode or focused view.

## Global prefix commands

### File

- `C-g c` — New Window
- Quit fvmux
- `C-g C` — New Window from Profile
- `C-g S` — Save Session
- New Session
- `C-g s` — Open Session…
- Save Session As…
- `C-g $` — Rename Session…
- Delete Session…
- `C-g D` — Detach from tmux
- `C-g :` — Run Command…
- Edit config.toml
- Edit profiles.toml
- Edit keybindings.toml

### Edit

- `C-g [` — Enter Copy Mode
- `C-g ]` — Paste
- `C-g /` — Find in Scrollback
- `C-g ~` — Toggle Sync-Input (broadcast)
- Send Interrupt (Ctrl-C)
- Send Quit (Ctrl-\)
- Send EOF (Ctrl-D)
- Send SIGTERM

### View

- `C-g z` — Zoom Focused Pane
- `C-g t` — Toggle Clock
- Toggle Status Bar
- Toggle Menu Bar
- `C-g m` — Activate Menu Bar
- `C-g r` — Redraw
- `C-g q` — Flash Window Numbers
- Layout: Even Horizontal
- Layout: Even Vertical
- Layout: Main Horizontal
- Layout: Main Vertical
- Layout: Tiled
- `C-g T` — Theme…
- Edit Themes…
- `C-g Space` — Cycle Layout

### Pane

- `C-g %` — Split Left/Right
- `C-g "` — Split Top/Bottom
- `C-g x` — Kill Pane
- `C-g h` — Focus Left
- `C-g j` — Focus Down
- `C-g k` — Focus Up
- `C-g l` — Focus Right
- `C-g }` — Swap with Next
- `C-g {` — Swap with Prev
- `C-g !` — Break Out to Window
- Rename Pane…
- `C-g @` — Join from Window…
- `C-g o` — Focus Next Pane
- `C-g ;` — Focus Previous Pane
- Respawn Dead Pane
- `C-g R` — Enter Resize Mode

### Window

- `C-g n` — Next Window
- `C-g p` — Previous Window
- `C-g &` — Kill Window
- `C-g ,` — Rename Window…
- `C-g Tab` — Last Window (MRU)
- `C-g w` — Window List…
- `C-g f` — Find Window…
- `C-g 1` — Focus Window 1
- `C-g 2` — Focus Window 2
- `C-g 3` — Focus Window 3
- `C-g 4` — Focus Window 4
- `C-g 5` — Focus Window 5
- `C-g 6` — Focus Window 6
- `C-g 7` — Focus Window 7
- `C-g 8` — Focus Window 8
- `C-g 9` — Focus Window 9
- `C-g g` — Tile (grid)
- `C-g G` — Tile Horizontal
- `C-g v` — Tile Vertical
- `C-g K` — Cascade
- Cascade (keep sizes)

### Connections

- `C-g H` — Connect to Host…
- `C-g B` — Edit hosts.toml
- SSH Connection Diagnostics…
- Validate hosts.toml

### Transfer

- `C-g F` — Files Window…
- Open Files Here
- Open Another Files Window Here
- Follow Terminal Directory
- Copy to Other Side (F5)
- Move / Rename (F6)
- Active Transfers…
- Clear Completed Transfers

### Help

- `C-g ?` — Cheatsheet
- `C-g P` — Command Palette
- `C-g W` — Reset First-Run Wizard…
- Reload Config
- `C-g L` — Log Viewer
- About fvmux…

## Menu activation and navigation

- **C-g m** — Activate the fvmux menu bar; works from terminal and non-terminal focus.
- **F10 / Alt mnemonic** — Activate a top-level menu; only when focus is not inside an embedded terminal.
- **Left / Right** — Move between top-level menus.
- **Up / Down** — Move within a menu.
- **Enter** — Open or select the highlighted item.
- **Esc** — Close the active menu.

## Embedded terminal and scrollback

- **F10 / Alt-F, Alt-E, Alt-V, Alt-P, Alt-W, Alt-C, Alt-T, Alt-H** — Forward to the child PTY; the menu bar deliberately passes these through while terminal focus is raw.
- **Shift-PgUp / Shift-PgDn** — Scroll back or forward by half a page.
- **Shift-Home / Shift-End** — Jump to the oldest scrollback or live bottom.
- **/** — Search scrollback; when already viewing scrollback.
- **n / N** — Move to next / previous search result.
- **Other keys** — Forward to the child PTY; includes fv-go v0.5.4 classic modified arrows, navigation keys, F1-F12, Alt-Unicode, Ctrl-Space, and DECCKM cursor keys.

## Copy mode

- **Arrows / PgUp / PgDn / Home / End** — Move the copy cursor.
- **Space** — Toggle the selection anchor.
- **Enter** — Copy the selection and exit.
- **/** — Exit copy mode and start scrollback search.
- **Esc** — Exit without copying.

## Resize mode

- **h / j / k / l** — Move the nearest divider by one cell.
- **H / J / K / L** — Move the nearest divider by five cells.
- **Esc** — Exit resize mode.

## Files / SFTP window

- **Tab** — Rotate focus through remote tree, remote listing, local tree, and local listing.
- **Enter** — Enter a folder or preview a file.
- **F5** — Copy the selected file or folder to the other side.
- **F6** — Move or rename the selected entry.
- **F7** — Create a directory on the focused side.
- **F8** — Delete the selected entry recursively.
- **Ctrl-R** — Refresh both panels.
- **Delete** — Cancel the newest active transfer.
- **Esc** — Close the Files window.

## Dialogs and pickers

- **Tab / Shift-Tab** — Move focus between controls.
- **Arrows** — Move within lists and popup choices.
- **Type** — Filter fuzzy pickers or edit the active input.
- **Enter** — Accept the current choice.
- **Esc** — Cancel or close.

## fvmuxa / tmux wrapper

- **F12 d** — Detach the private tmux session.
- **F12 F12** — Send one literal F12 to fvmux / its focused child.

## Platform, layout, and protocol notes

- **macOS F-keys** — Hold Fn/Globe when system keyboard settings reserve physical F-keys [macOS].
- **Logical characters** — Bindings follow the character produced by the keyboard layout, not the physical key position; the portable preset avoids Alt/Meta and punctuation-heavy US-layout positions.
- **Legacy xterm input** — Supported keyboard protocol boundary; do not force kitty keyboard protocol, CSI-u, or modifyOtherKeys for the fvmux process; enhanced protocols are not negotiated.
- **Ctrl-I / Ctrl-M / Ctrl-[** — Classic aliases remain Tab / Enter / Esc; fvmux canonicalizes these aliases so they cannot create unreachable duplicate bindings.
