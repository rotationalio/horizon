package version

import (
	"fmt"

	"go.rtnl.ai/x/semver"
)

// Version component constants for the current build.
const (
	VersionMajor         = 1
	VersionMinor         = 0
	VersionPatch         = 0
	VersionReleaseLevel  = "beta"
	VersionReleaseNumber = 1
)

// Version returns the semantic version for the current build.
func Version(short bool) string {
	vers := semver.Version{
		Major:      VersionMajor,
		Minor:      VersionMinor,
		Patch:      VersionPatch,
		PreRelease: PreRelease(),
	}

	if short {
		return vers.Short()
	}
	return vers.String()
}

func PreRelease() string {
	if VersionReleaseLevel != "" && VersionReleaseLevel != "final" {
		if VersionReleaseNumber > 0 {
			return fmt.Sprintf("%s.%d", VersionReleaseLevel, VersionReleaseNumber)
		}
		return VersionReleaseLevel
	}
	return ""
}
