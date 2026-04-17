// Package version exposes the build version of the lakshmi binary.
//
// The Version variable is overridden at build time via -ldflags:
//
//	go build -ldflags "-X github.com/aayush1607/lakshmi/internal/version.Version=$(git describe --tags --always --dirty)"
package version

// Version is the build version. Defaults to "dev" in non-release builds.
var Version = "dev"
