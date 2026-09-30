package horizon

import "go.rtnl.ai/horizon/version"

// Version component constants for the current build.
const (
	VersionMajor         = version.Major
	VersionMinor         = version.Minor
	VersionPatch         = version.Patch
	VersionReleaseLevel  = version.ReleaseLevel
	VersionReleaseNumber = version.ReleaseNumber
)

// Returns the semantic version for the current build.
func Version(short bool) string {
	return version.String(short)
}

// Returns the prerelease component for the current build.
func PreRelease() string {
	return version.PreRelease()
}
