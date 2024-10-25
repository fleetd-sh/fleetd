package version

import "os"

var (
	// Set during the build process using ldflags
	Version   = "development"
	CommitSHA = "unknown"
	BuildTime = "unknown"
)

func init() {
	if v := os.Getenv("VERSION"); v != "" {
		Version = v
	}
	if c := os.Getenv("COMMIT_SHA"); c != "" {
		CommitSHA = c
	}
	if b := os.Getenv("BUILD_TIME"); b != "" {
		BuildTime = b
	}
}

// GetVersion returns the full version string
func GetVersion() string {
	return Version + " (" + CommitSHA + ") built at " + BuildTime
}
