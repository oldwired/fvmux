# Next release notes

## Keyboard integration

- fvmux now requires `github.com/oldwired/fv-go v0.5.4+` for normalized
  logical key identity and improved classic terminal key forwarding.
- Focused terminals receive F10 and top-level Alt menu mnemonics; prefix+m is
  the universal fvmux menu path.
- The overlapping triple-prefix wizard gesture is removed. Prefix+W opens the
  wizard, while double-prefix remains immediate literal-prefix forwarding.
- Custom bindings share one parser/formatter/event model, support normalized
  arrows, navigation keys, F1-F12, Unicode, modifiers, Ctrl-Space, and
  representable control punctuation, and report structured reachability
  diagnostics.
- `fvmux --check-config` validates all user configuration without starting
  the TUI.
- `KEYBINDINGS.md` and the in-app cheatsheet now include maintained
  contextual shortcuts. A tested letter-oriented portable preset is shipped
  for punctuation- and AltGr-heavy layouts.
