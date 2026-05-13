// Package clipboard wraps github.com/atotto/clipboard with fvmux's
// preferred name. atotto handles each platform's native clipboard
// (X11 selection, macOS pasteboard, Win32 clipboard) and falls back
// to OSC 52 on Linux without xclip/wl-copy, which works fine inside
// an outer tmux.
//
// fv-go's own clipboard surface is OSC-52-set-only; fvmux uses this
// package whenever it needs to read or reliably set the OS clipboard.
package clipboard

import "github.com/atotto/clipboard"

// Get returns the current OS clipboard contents. Returns "" + nil
// if the platform doesn't support clipboard reads in this environment.
func Get() (string, error) { return clipboard.ReadAll() }

// Set replaces the OS clipboard with text.
func Set(text string) error { return clipboard.WriteAll(text) }

// Supported reports whether atotto can talk to the OS clipboard at all
// in this environment.
func Supported() bool { return !clipboard.Unsupported }
