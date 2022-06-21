//go:build mage

// Copyright 2021-2022
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/magefile/mage/mg" // mg contains helpful utility functions, like Deps
	"github.com/magefile/mage/sh"
)

const (
	binaryName  = "pvapi"
	packageName = "."
	module      = "github.com/penny-vault/pv-api"
)

// allow user to override go executable by running as GOEXE=xxx make ... on unix-like systems
var goexe = "go"

func init() {
	if exe := os.Getenv("GOEXE"); exe != "" {
		goexe = exe
	}
}

// Default target to run when none is specified
// If not set, running mage will list available targets
// var Default = Build

func Build() error {
	fmt.Printf("Building version: %s\n", version().String())
	return runWith(flagEnv(), goexe, "build", "-o", binaryName, "-ldflags", ldFlags(), buildFlags(), "-tags", buildTags(), "-v", packageName)
}

// Manage your deps, or running package managers.
func InstallDeps() error {
	fmt.Println("Installing Deps...")
	return runWith(flagEnv(), goexe, "get", ".")
}

func Install() error {
	return runWith(flagEnv(), goexe, "install", "-ldflags", ldFlags(), buildFlags(), "-tags", buildTags(), packageName)
}

func Uninstall() error {
	return sh.Run(goexe, "clean", "-i", packageName)
}

// Clean up
func Clean() {
	fmt.Println("Cleaning...")
	os.RemoveAll("pvapi")
}

// Run tests and linters
func Check() {
	mg.Deps(Fmt, Vet)

	// don't run two tests in parallel, they saturate the CPUs anyway, and running two
	// causes memory issues in CI.
	mg.Deps(TestRace)
}

func testGoFlags() string {
	return ""
}

// Run tests
func Test() error {
	fmt.Println("Go Test")
	env := map[string]string{"GOFLAGS": testGoFlags()}
	return runCmd(env, goexe, "test", "./...", buildFlags(), "-tags", buildTags())
}

// Run tests with race detector
func TestRace() error {
	fmt.Println("Go Test Race")
	env := map[string]string{"GOFLAGS": testGoFlags()}
	return runCmd(env, goexe, "test", "-race", "./...", buildFlags(), "-tags", buildTags())
}

// Run gofmt linter
func Fmt() error {
	fmt.Println("Go Format")

	pkgs, err := packages()

	if err != nil {
		return err
	}
	failed := false
	first := true
	for _, pkg := range pkgs {
		files, err := filepath.Glob(filepath.Join(pkg, "*.go"))
		if err != nil {
			return nil
		}
		for _, f := range files {
			fmt.Printf("Format: %s\n", f)
			// gofmt doesn't exit with non-zero when it finds unformatted code
			// so we have to explicitly look for output, and if we find any, we
			// should fail this target.
			s, err := sh.Output("gofmt", "-l", f)
			if err != nil {
				fmt.Printf("ERROR: running gofmt on %q: %v\n", f, err)
				failed = true
			}
			if s != "" {
				if first {
					fmt.Println("The following files are not gofmt'ed:")
					first = false
				}
				failed = true
				fmt.Println(s)
			}
		}
	}
	if failed {
		return errors.New("improperly formatted go files")
	}
	return nil
}

// Run golint linter
func Lint() error {
	fmt.Println("Go Lint")

	pkgs, err := packages()
	if err != nil {
		return err
	}
	failed := false
	for _, pkg := range pkgs {
		// We don't actually want to fail this target if we find golint errors,
		// so we don't pass -set_exit_status, but we still print out any failures.
		if _, err := sh.Exec(nil, os.Stderr, nil, "golint", pkg); err != nil {
			fmt.Printf("ERROR: running go lint on %q: %v\n", pkg, err)
			failed = true
		}
	}
	if failed {
		return errors.New("errors running golint")
	}
	return nil
}

//  Run go vet linter
func Vet() error {
	fmt.Println("Go Vet")

	if err := sh.Run(goexe, "vet", "./..."); err != nil {
		return fmt.Errorf("error running go vet: %v", err)
	}
	return nil
}

// Generate test coverage report
func TestCoverHTML() error {
	fmt.Println("Generate Test Coverage HTML")

	const (
		coverAll = "coverage-all.out"
		cover    = "coverage.out"
	)
	f, err := os.Create(coverAll)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write([]byte("mode: count")); err != nil {
		return err
	}

	pkgs, err := packages()
	if err != nil {
		return err
	}
	for _, pkg := range pkgs {
		if err := sh.Run(goexe, "test", "-coverprofile="+cover, "-covermode=count", pkg); err != nil {
			return err
		}
		b, err := ioutil.ReadFile(cover)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		idx := bytes.Index(b, []byte{'\n'})
		b = b[idx+1:]
		if _, err := f.Write(b); err != nil {
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return sh.Run(goexe, "tool", "cover", "-html="+coverAll)
}

// Helpers

func buildFlags() []string {
	if runtime.GOOS == "windows" {
		return []string{"-buildmode", "exe"}
	}
	return nil
}

func buildTags() string {
	return "jwx_goccy"
}

func gitHash() string {
	hash, err := sh.Output("git", "rev-parse", "--short", "HEAD")
	if err != nil {
		fmt.Printf("error determining current git hash (maybe no commits on repo?): %s\n", err.Error())
		return ""
	}
	return hash
}

func version() *semver.Version {
	// check if the current commit is tagged
	currentCommit, err := sh.Output("git", "show", "-s", `--format="%h %D"`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting commit info: %s", err.Error())
		os.Exit(1)
	}

	regex, _ := regexp.Compile(`(?P<hash>[a-z0-9]+) (grafted, )?HEAD.*?(?P<tag>tag: v(?P<version>\d{1,3}.\d{1,3}.\d{1,3}))?`)

	params := make(map[string]string)
	res := regex.FindStringSubmatch(currentCommit)
	if len(res) != 0 {
		for ii, name := range regex.SubexpNames() {
			params[name] = res[ii]
		}
	} else {
		fmt.Fprintf(os.Stderr, "version regex did not match '%s'\n", currentCommit)
		os.Exit(1)
	}

	for k, v := range params {
		if v == "" {
			delete(params, k)
		}
	}

	// if this is a tagged version just return it
	if taggedVersion, ok := params["version"]; ok {
		ver, err := semver.NewVersion(taggedVersion)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			fmt.Fprintln(os.Stderr, currentCommit)
			fmt.Fprintf(os.Stderr, "could not create version from tag: '%s'\n", taggedVersion)
			os.Exit(1)
		}
		return ver
	}

	// git a list of all version tags
	versionTags, err := sh.Output("git", "tag", "-l", "v*.*.*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting list of versions: %s\n", err.Error())
		os.Exit(1)
	}

	if versionTags == "" {
		initialVersion := fmt.Sprintf("0.0.0-dev+%s", params["hash"])
		if ver, err := semver.NewVersion(initialVersion); err != nil {
			fmt.Fprintf(os.Stderr, "error parsing initial version: '%s'\n", initialVersion)
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		} else {
			return ver
		}
	}

	// parse and sort
	versions := strings.Split(versionTags, "\n")
	vs := make([]*semver.Version, len(versions))
	for i, r := range versions {
		v, err := semver.NewVersion(r)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing version: '%s'\n", r)
			fmt.Fprintf(os.Stderr, err.Error())
		}

		vs[i] = v
	}

	// check if a version has previously been released; if so rev it and
	// apply the -dev pre-release fields
	var newVer string
	if len(vs) != 0 {
		sort.Sort(semver.Collection(vs))
		latestVersion := vs[len(vs)-1]
		major := latestVersion.Major()
		minor := latestVersion.Minor()
		patch := latestVersion.Patch()

		// plus up minor and annotate with meta-data
		minor++
		patch = 0
		newVer = fmt.Sprintf("%d.%d.%d-dev+%s", major, minor, patch, params["hash"])
	} else {
		// could not find a version, this must be a new development, use 0.0.0
		newVer = fmt.Sprintf("0.0.0-dev+%s", params["hash"])
	}

	ver, err := semver.NewVersion(newVer)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		fmt.Fprintf(os.Stderr, "could not create version %s\n", newVer)
		os.Exit(1)
	}
	return ver
}

func flagEnv() map[string]string {
	return map[string]string{}
}

func buildTimeVariables() map[string]string {
	return map[string]string{
		"pkginfo.BuildDate":   time.Now().Format("2006-01-02T15:04:05Z0700"),
		"pkginfo.CommitHash":  gitHash(),
		"pkginfo.ProgramName": binaryName,
		"pkginfo.Version":     version().String(),
	}
}

func ldFlags() string {
	embeddedVars := buildTimeVariables()
	var ldflags = make([]string, 0, len(embeddedVars)*2)

	for k, v := range embeddedVars {
		ldflags = append(ldflags, "-X")
		ldflags = append(ldflags, fmt.Sprintf("'%s/%s=%s'", module, k, v))
	}
	return strings.Join(ldflags, " ")
}

func runCmd(env map[string]string, cmd string, args ...interface{}) error {
	if mg.Verbose() {
		return runWith(env, cmd, args...)
	}
	output, err := sh.OutputWith(env, cmd, argsToStrings(args...)...)
	if err != nil {
		fmt.Fprint(os.Stderr, output)
	}

	return err
}

func runWith(env map[string]string, cmd string, inArgs ...interface{}) error {
	s := argsToStrings(inArgs...)
	fmt.Printf("%s %s\n", cmd, strings.Join(s, " "))
	return sh.RunWith(env, cmd, s...)
}

var (
	pkgPrefixLen = len(module)
	pkgs         []string
	pkgsInit     sync.Once
)

func packages() ([]string, error) {
	var err error
	pkgsInit.Do(func() {
		var s string
		s, err = sh.Output(goexe, "list", "./...")
		if err != nil {
			return
		}
		pkgs = strings.Split(s, "\n")
		for i := range pkgs {
			pkgs[i] = "." + pkgs[i][pkgPrefixLen:]
		}
	})
	return pkgs, err
}

func argsToStrings(v ...interface{}) []string {
	var args []string
	for _, arg := range v {
		switch v := arg.(type) {
		case string:
			if v != "" {
				args = append(args, v)
			}
		case []string:
			if v != nil {
				args = append(args, v...)
			}
		default:
			panic("invalid type")
		}
	}

	return args
}
