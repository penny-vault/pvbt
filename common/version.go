package common

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
)

var (
	// commitHash contains the current Git revision.
	// Use mage to build to make sure this gets set.
	commitHash string

	// buildDate contains the date of the current build.
	buildDate string

	// vendorInfo contains vendor notes about the current build.
	vendorInfo string
)

// Version represents a SemVer 2.0.0 compatible build version
type Version struct {
	// Increment this for backwards incompatible changes
	Major int

	// Increment this for feature releases
	Minor int

	// Increment this for bug releases
	Patch int

	// VersionSuffix is the suffix used in the PV API version string.
	// It will be blank for release versions.
	Suffix string
}

// GetDependencyList returns a sorted dependency list on the format package="version".
func GetDependencyList() []string {
	var deps []string

	formatDep := func(path, version string) string {
		return fmt.Sprintf("%s=%q", path, version)
	}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return deps
	}

	for _, dep := range bi.Deps {
		deps = append(deps, formatDep(dep.Path, dep.Version))
	}

	sort.Strings(deps)

	return deps
}

func (v Version) String() string {
	metadata := ""
	preRelease := ""

	if v.Suffix != "" {
		preRelease = fmt.Sprintf("-%s", v.Suffix)
		if commitHash != "" {
			metadata = fmt.Sprintf("+%s", strings.ToLower(commitHash))
		}
	}

	return fmt.Sprintf("%d.%d.%d%s%s", v.Major, v.Minor, v.Patch, preRelease, metadata)
}

// BuildVersionString creates a version string. This is what you see when
// running "pvapi version".
func BuildVersionString() string {
	program := "pvapi"

	version := "v" + CurrentVersion.String()

	osArch := runtime.GOOS + "/" + runtime.GOARCH
	goVersion := runtime.Version()

	date := buildDate
	if date == "" {
		date = "unknown"
	}

	versionString := fmt.Sprintf(`%s %s %s

Build Date: %s
Commit: %s
Built with: %s`,
		program, version, osArch, date, commitHash, goVersion)

	if vendorInfo != "" {
		versionString += "\nVendor Info: " + vendorInfo
	}

	versionString += "\n\nDependencies:\n\n" + strings.Join(GetDependencyList(), "\n")

	return versionString
}
