// Package vcruntime checks the Microsoft Visual C++ runtime that Windows PHP builds load,
// and runs Microsoft's own installer when it is missing or too old.
package vcruntime

import (
	"debug/pe"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"phlo/internal/api"
)

// InstallerURL is Microsoft's permanent link to the latest supported x64 redistributable.
const InstallerURL = "https://aka.ms/vc14/vc_redist.x64.exe"

// Version is a PE linker version such as 14.29 (Visual Studio 2019) or 14.44 (Visual Studio 2022).
type Version struct{ Major, Minor int }

func (v Version) String() string { return fmt.Sprintf("%d.%d", v.Major, v.Minor) }

func (v Version) less(o Version) bool {
	return v.Major < o.Major || (v.Major == o.Major && v.Minor < o.Minor)
}

// LinkerVersion reads the linker version from the header of an .exe or .dll.
func LinkerVersion(path string) (Version, error) {
	file, err := pe.Open(path)
	if err != nil {
		return Version{}, err
	}
	defer file.Close()
	switch header := file.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		return Version{int(header.MajorLinkerVersion), int(header.MinorLinkerVersion)}, nil
	case *pe.OptionalHeader32:
		return Version{int(header.MajorLinkerVersion), int(header.MinorLinkerVersion)}, nil
	}
	return Version{}, errors.New(path + " has no optional header")
}

// Build is one installed PHP folder.
type Build struct {
	Version string
	Dir     string
}

// SystemDir is %SystemRoot%\System32.
func SystemDir() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32")
}

// Check reports the PHP builds that cannot load their vcruntime140.dll.
// PHP refuses to start unless that DLL has PHP's major linker version and at least its minor one.
// A copy next to php-cgi.exe is loaded before the one in System32.
func Check(systemDir string, builds []Build) api.VCRuntime {
	out := api.VCRuntime{OK: true}
	systemDLL := filepath.Join(systemDir, "vcruntime140.dll")
	if have, err := LinkerVersion(systemDLL); err == nil {
		out.Have = have.String()
	}
	var need Version
	for _, build := range builds {
		want, err := phpLinker(build.Dir)
		if err != nil {
			continue
		}
		dll := filepath.Join(build.Dir, "vcruntime140.dll")
		if _, err := os.Stat(dll); err != nil {
			dll = systemDLL
		}
		have, err := LinkerVersion(dll)
		if err == nil && have.Major == want.Major && !have.less(want) {
			continue
		}
		out.OK = false
		out.Blocked = append(out.Blocked, build.Version)
		if need.less(want) {
			need = want
		}
	}
	if !out.OK {
		out.Need = need.String()
	}
	return out
}

// phpLinker is the linker version of php8.dll, php8ts.dll, or php7ts.dll, else php-cgi.exe.
func phpLinker(dir string) (Version, error) {
	candidates, _ := filepath.Glob(filepath.Join(dir, "php[0-9]*.dll"))
	candidates = append(candidates, filepath.Join(dir, "php-cgi.exe"))
	for _, candidate := range candidates {
		if version, err := LinkerVersion(candidate); err == nil {
			return version, nil
		}
	}
	return Version{}, errors.New("no PHP binary in " + dir)
}
