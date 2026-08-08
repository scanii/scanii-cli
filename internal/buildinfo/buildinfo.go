// Package buildinfo reports the identity of the running binary: the release
// version, the build date and the User-Agent we present to the Scanii API.
package buildinfo

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// These variables are set in the build step via -ldflags -X.
var (
	version string
	date    string
)

const (
	devVersion  = "dev"
	unknownDate = "unknown"
)

// Version returns the version of the running binary. Release builds carry it in
// the version ldflag, `go install`ed binaries carry it in the module version and
// local builds fall back to the VCS revision recorded by the toolchain.
func Version() string {
	if version != "" {
		return version
	}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return devVersion
	}

	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}

	var revision string
	var modified bool
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				modified = true
			}
		}
	}

	if revision == "" {
		return devVersion
	}

	if len(revision) > 7 {
		revision = revision[:7]
	}

	if modified {
		return fmt.Sprintf("%s-dirty", revision)
	}

	return revision
}

// Date returns the build date of the running binary.
func Date() string {
	if date != "" {
		return date
	}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return unknownDate
	}

	for _, s := range bi.Settings {
		if s.Key == "vcs.time" && s.Value != "" {
			return s.Value
		}
	}

	return unknownDate
}

// UserAgent returns the User-Agent header value sent with every API request,
// for example: scanii-cli/1.4.2 (go1.26; darwin/arm64)
func UserAgent() string {
	return fmt.Sprintf("scanii-cli/%s (%s; %s/%s)", Version(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
