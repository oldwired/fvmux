package main

import "flag"

// flags holds the parsed command-line flags. Only the fields actually
// consumed in step 1+2 (NoSplash, Log, ShowVersion) are wired; the
// others are accepted-but-ignored until later sub-steps consume them.
type flags struct {
	Config      string
	Session     string
	Profile     string
	NoSplash    bool
	Log         string
	CheckConfig bool
	ShowVersion bool
}

func parseFlags() flags {
	var f flags
	flag.StringVar(&f.Config, "config", "", "path to config directory (defaults to XDG)")
	flag.StringVar(&f.Session, "session", "", "session name to load on startup")
	flag.StringVar(&f.Profile, "profile", "", "profile name to use for the initial pane")
	flag.BoolVar(&f.NoSplash, "no-splash", false, "skip the splash screen and welcome dialog")
	flag.StringVar(&f.Log, "log", "", "path to log file (defaults to stderr ring buffer)")
	flag.BoolVar(&f.CheckConfig, "check-config", false, "validate configuration files and exit")
	flag.BoolVar(&f.ShowVersion, "version", false, "print version and exit")
	flag.Parse()
	return f
}
