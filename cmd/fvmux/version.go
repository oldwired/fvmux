package main

// Version is overridden at build time via:
//
//	go build -ldflags "-X main.Version=v0.1.0" ./cmd/fvmux
var Version = "dev"
