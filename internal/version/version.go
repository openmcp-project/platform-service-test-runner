package version

import (
	"os"
	"strings"
)

// These variables are set via ldflags during build time by the task build system.
// See hack/common/tasks_build_bin.yaml for the ldflags configuration.
var (
	buildVersion = "unknown" // set via -X flag
)

// GetVersion returns the build version.
// If the version was not set via ldflags (e.g., running with go run or go test),
// it attempts to read from the VERSION file as a fallback.
func GetVersion() string {
	if buildVersion != "unknown" {
		return strings.TrimSpace(buildVersion)
	}

	// Fallback: try to read VERSION file for development/testing
	if data, err := os.ReadFile("VERSION"); err == nil {
		return strings.TrimSpace(string(data))
	}

	return "v0.0.0-dev"
}
