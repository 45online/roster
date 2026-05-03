// Package version exposes the binary's release version. It exists as its
// own tiny package so that ldflags can target a single, stable symbol
// path — both bootstrap and tui need this string and we don't want one
// to depend on the other just to share a constant.
//
// Override at link-time:
//
//	go build -ldflags "-X github.com/45online/roster/internal/version.Version=v0.1.2" ./cmd/roster
package version

// Version is the canonical version string for the binary. The default
// "dev" is overwritten by goreleaser / the Docker build for tagged
// releases.
var Version = "dev"
