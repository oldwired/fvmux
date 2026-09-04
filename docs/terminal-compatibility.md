# Terminal compatibility release matrix

Last updated: 2026-09-04

fvmux targets legacy xterm-compatible keyboard input. It does not negotiate
kitty keyboard protocol, CSI-u, or comprehensive `modifyOtherKeys`; do not
force those protocols for the fvmux process. This boundary does not prevent
inner applications from receiving fv-go v0.5.4's improved classic modified
arrow, navigation, F-key, Alt-Unicode, Ctrl-Space, and DECCKM encodings.

## Recorded verification

| Date | OS / path | Emulator or host | Version | Layout / Meta mode | Result |
|---|---|---|---|---|---|
| 2026-09-04 | macOS 26.6.2 arm64, local integration suite | headless fv-go backend + Go tests | fv-go v0.5.4 | normalized synthetic US/German-relevant identities | Pass: dispatch, menu raw focus, prefix rebinding, literal/wizard separation, Files/help parity |
| next CI run | Linux native | GitHub Actions `ubuntu-latest` | recorded by Actions | n/a | Required: build, vet, test, race |
| next CI run | macOS native | GitHub Actions `macos-latest` | recorded by Actions | n/a | Required: build, vet, test |
| next CI run | Windows native | GitHub Actions `windows-latest` | recorded by Actions | n/a | Required: build, vet, test |

The automated rows verify product integration and Windows runtime tests. They
do not substitute for an emulator session. Fill the manual table for a release
candidate; never infer a pass from source review or cross-compilation.

## Manual release matrix

| Date | Platform / nesting path | Emulator and version | Keyboard layout | Option / Meta mode | Result / notes |
|---|---|---|---|---|---|
| pending | Linux native | GNOME Terminal / VTE | US + German | default | release candidate check |
| pending | Linux native | Konsole | US + German | default | release candidate check |
| pending | Linux native | xterm | US | default | release candidate check |
| pending | Linux native | kitty | US + German | legacy mode | do not force kitty protocol |
| pending | Linux native | WezTerm | US + German | legacy mode | release candidate check |
| pending | Linux → tmux | selected emulator → tmux → fvmux | US + German | recorded per emulator | nesting check |
| pending | Linux → SSH | selected emulator → SSH → fvmux | US + German | recorded per emulator | latency / ESC framing check |
| pending | macOS native | Terminal.app | US + German | Option ordinary + Meta | Fn/Globe may be needed for F-keys |
| pending | macOS native | iTerm2 | US + German | Esc+ and ordinary | release candidate check |
| pending | macOS native | kitty | US + German | legacy mode | do not force kitty protocol |
| pending | macOS native | WezTerm | US + German | legacy mode | release candidate check |
| pending | Windows native | Windows Terminal | US + German | default | manual Windows host required |
| pending | Windows native | Console Host | US | default | manual Windows host required |
| pending | Windows native | VS Code integrated terminal | US + German | default | manual Windows host required |
| pending | Windows compatibility | mintty / Git Bash → ConPTY | US + German | default | classify ConPTY; record older winpty separately |
| pending | WSL | Windows Terminal → WSL → Linux fvmux | US + German | default | nested compatibility check |
| pending | wrapper | emulator → private `fvmuxa` tmux config | US + German | recorded per emulator | verify F12 d and F12 F12 |

## Scenarios for every manual path

- Exercise every default prefix chord, especially punctuation-heavy
  split/swap/break/join bindings.
- Repeat with Ctrl-A, Ctrl-B, and Ctrl-G prefixes.
- Confirm double-prefix immediately sends exactly one literal prefix byte and
  that prefix+W opens the wizard without sending child input.
- Confirm F10 and Alt-F/E/V/P/W/C/T/H reach a focused child, while prefix+m
  opens the fvmux menu from every focus.
- Exercise modified arrows, navigation keys, F1-F12, Ctrl-Space, Alt-Unicode,
  and application-cursor behavior in an inner shell/editor/TUI.
- Exercise copy mode, resize mode, and Files F5-F8, Ctrl-R, Delete, Enter,
  Tab, and Esc.
- On German layouts, exercise AltGr-produced symbols and the portable preset.
- On macOS, record physical Fn/Globe behavior and both ordinary Option input
  and Option-as-Meta/Esc+.

Record failures with the complete path, emulator version, layout, Meta mode,
and whether an enhanced keyboard protocol was forced.
