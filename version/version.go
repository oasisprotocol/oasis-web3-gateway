package version

import (
	"fmt"
)

const (
	VersionMajor = 0
	VersionMinor = 1
	VersionPatch = 0
	VersionTag   = "alpha"
)

func Version() string {
	return fmt.Sprintf("%d.%d.%d-%s", VersionMajor, VersionMinor, VersionPatch, VersionTag)
}
