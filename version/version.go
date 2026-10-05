// Package version defines Horizon build version information.
package version

import (
	"fmt"

	"go.rtnl.ai/x/semver"
)

// Version component constants for the current build.
const (
	Major         = 1
	Minor         = 0
	Patch         = 0
	ReleaseLevel  = "beta"
	ReleaseNumber = 1
)

// Provides the Horizon user-agent value used for outbound requests.
func UserAgent() string {
	return "Horizon/" + String(true)
}

// Returns the semantic version for the current build.
func Version() semver.Version {
	return semver.Version{
		Major:      Major,
		Minor:      Minor,
		Patch:      Patch,
		PreRelease: PreRelease(),
	}
}

// Returns the human-readable version string for the current build.
func String(short bool) string {
	vers := Version()

	if short {
		return vers.Short()
	}
	return vers.String()
}

// Returns the prerelease component for the current build.
func PreRelease() string {
	if ReleaseLevel != "" && ReleaseLevel != "final" {
		if ReleaseNumber > 0 {
			return fmt.Sprintf("%s.%d", ReleaseLevel, ReleaseNumber)
		}
		return ReleaseLevel
	}
	return ""
}
