# fvmux Keyboard Reference

Global commands use the configured prefix. Contextual keys apply only in the named mode or focused view.

## Global prefix commands

### File

- `C-g c` — **N**ew Window
- **Q**uit fvmux
- `C-g C` — New Window from Profile
- `C-g S` — **S**ave Session
- Ne**w** Session
- `C-g s` — **O**pen Session…
- Save Session **A**s…
- `C-g $` — Rena**m**e Session…
- De**l**ete Session…
- `C-g D` — **D**etach from tmux
- `C-g :` — **R**un Command…
- Edit **c**onfig.toml
- Edit pro**f**iles.toml
- Edit **k**eybindings.toml

### Edit

- `C-g [` — Enter **C**opy Mode
- `C-g ]` — **P**aste
- `C-g /` — **F**ind in Scrollback
- `C-g ~` — Toggle **S**ync-Input (broadcast)
- Send **I**nterrupt (Ctrl-C)
- Send **Q**uit (Ctrl-\)
- Send **E**OF (Ctrl-D)
- Send SIG**T**ERM

### View

- `C-g z` — **Z**oom Focused Pane
- `C-g t` — Toggle **C**lock
- Toggle **S**tatus Bar
- Toggle **M**enu Bar
- `C-g m` — Activate Menu Bar
- `C-g r` — **R**edraw
- `C-g q` — Flash Window **N**umbers
- Layout: Even **H**orizontal
- Layout: Even **V**ertical
- Layout: Main H**o**rizontal
- Layout: Main Ve**r**tical
- Layout: **T**iled
- `C-g T` — **T**heme…
- **E**dit Themes…
- `C-g Space` — Cycle **L**ayout

### Pane

- `C-g %` — Split Left/Ri**g**ht
- `C-g "` — Split **T**op/Bottom
- `C-g x` — **K**ill Pane
- `C-g h` — Focus **L**eft
- `C-g j` — Focus **D**own
- `C-g k` — Focus **U**p
- `C-g l` — Focus **R**ight
- `C-g }` — Swap with **N**ext
- `C-g {` — S**w**ap with Prev
- `C-g !` — **B**reak Out to Window
- Rename **P**ane…
- `C-g @` — **J**oin from Window…
- `C-g o` — Focus N**e**xt Pane
- `C-g ;` — Focus Previous Pane
- Re**s**pawn Dead Pane
- `C-g R` — Enter **R**esize Mode

### Window

- `C-g n` — N**e**xt Window
- `C-g p` — **P**revious Window
- `C-g X` — **K**ill Window
- `C-g ,` — **R**ename Window…
- `C-g Tab` — **L**ast Window (MRU)
- `C-g w` — Window L**i**st…
- `C-g f` — **F**ind Window…
- `C-g 1` — Focus Window 1
- `C-g 2` — Focus Window 2
- `C-g 3` — Focus Window 3
- `C-g 4` — Focus Window 4
- `C-g 5` — Focus Window 5
- `C-g 6` — Focus Window 6
- `C-g 7` — Focus Window 7
- `C-g 8` — Focus Window 8
- `C-g 9` — Focus Window 9
- `C-g g` — **T**ile (grid)
- `C-g G` — Tile **H**orizontal
- `C-g v` — Tile **V**ertical
- `C-g K` — **C**ascade
- Cascade (keep si**z**es)

### Connections

- `C-g H` — **C**onnect to Host…
- `C-g B` — **E**dit hosts.toml
- SSH Connection **D**iagnostics…
- **V**alidate hosts.toml

### Transfer

- `C-g F` — **F**iles Window…
- Open Files **H**ere
- Open Another Files **W**indow Here
- Follow Terminal **D**irectory
- C**o**py to Other Side (F5)
- **M**ove / Rename (F6)
- **A**ctive Transfers…
- **C**lear Completed Transfers

### Help

- `C-g ?` — **C**heatsheet
- `C-g P` — Command Palette
- `C-g W` — **R**eset First-Run Wizard…
- Re**l**oad Config
- `C-g L` — Lo**g** Viewer
- **A**bout fvmux…

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

## Cheatsheet window

- **/** — Fuzzy-search every reference entry.
- **Arrows / PgUp / PgDn / Home / End** — Scroll the reference.
- **Esc** — Clear a text selection or close the Cheatsheet window.

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
