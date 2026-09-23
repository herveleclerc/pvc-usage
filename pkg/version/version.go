package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the current semantic version of the plugin.
	Version = "v0.1.0"
	// GitCommit is the git sha1 of the commit.
	GitCommit = "dev"
	// BuildDate is the RFC3339 formatted build timestamp.
	BuildDate = "unknown"
)

// Info holds version details.
type Info struct {
	Version   string `json:"version"`
	GitCommit string `json:"gitCommit"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
	Compiler  string `json:"compiler"`
	Platform  string `json:"platform"`
}

// Get returns the version info struct.
func Get() Info {
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		Compiler:  runtime.Compiler,
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns a formatted version string.
func (i Info) String() string {
	return fmt.Sprintf("kubectl-pvc-usage %s (commit: %s, built: %s, %s, %s)",
		i.Version, i.GitCommit, i.BuildDate, i.GoVersion, i.Platform)
}
