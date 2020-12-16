package provisionserver

import (
	"fmt"

	"github.com/blang/semver"
)

var (
	// Raw is the string representation of the version. This will be replaced
	// with the calculated version at build time.
	Raw = "v0.0.1"

	// Version is semver representation of the version.
	Version = semver.MustParse("0.0.1")

	// String is the human-friendly representation of the version.
	String = fmt.Sprintf("ProvisionServerIpDiscoveryAgent %s", Raw)
)
