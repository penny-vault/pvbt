// Copyright 2021-2023
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/penny-vault/pv-api/pkginfo"
	"github.com/spf13/cobra"
)

var deps bool

func init() {
	rootCmd.AddCommand(versionCmd)

	versionCmd.Flags().BoolVar(&deps, "deps", false, "print dependencies")
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Long:  `Print the version number`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(BuildVersionString())
		if deps {
			fmt.Println()
			fmt.Println(DepString())
		}
	},
}

// BuildVersionString creates a version string. This is what you see when
// running "import-fred version".
func BuildVersionString() string {
	version := "v" + pkginfo.Version

	osArch := runtime.GOOS + "/" + runtime.GOARCH
	goVersion := runtime.Version()

	date := pkginfo.BuildDate
	if date == "" {
		date = "unknown"
	}

	versionString := fmt.Sprintf(`%s %s %s

Build Date: %s
Commit: %s
Built with: %s`,
		pkginfo.ProgramName, version, osArch, date, pkginfo.CommitHash, goVersion)

	return versionString
}

func DepString() string {
	return "Dependencies:\n\n" + strings.Join(GetDependencyList(), "\n")
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
