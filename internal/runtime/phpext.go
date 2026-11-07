package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"phlo/internal/errs"
	"phlo/internal/paths"
	"phlo/internal/php"
)

var phpExtNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// PHPExtensionLabels names the extensions Phlo can download. A manifest entry needs a label here.
var PHPExtensionLabels = map[string]string{
	"mongodb":   "MongoDB",
	"redis":     "Redis",
	"memcached": "Memcached",
}

func (m Manifest) validatePHPExtensions() error {
	for name, items := range m.PHPExtensions {
		if !phpExtNameRe.MatchString(name) {
			return fmt.Errorf("php extension name %s is not valid", name)
		}
		if _, ok := PHPExtensionLabels[name]; !ok {
			return fmt.Errorf("unknown php extension %s in manifest", name)
		}
		seen := map[string]struct{}{}
		for _, art := range items {
			if err := art.validate("php extension "+name, false); err != nil {
				return err
			}
			if !toolVersionRe.MatchString(art.Version) {
				return fmt.Errorf("php extension %s version %s must look like 2.5.4", name, art.Version)
			}
			if !versionRe.MatchString(art.PHP) {
				return fmt.Errorf("php extension %s %s needs php like 8.3", name, art.Version)
			}
			key := PHPExtKey(art.PHP, art.TS) + "/" + art.OS + "/" + art.Arch
			if _, ok := seen[key]; ok {
				return fmt.Errorf("duplicate php extension %s for %s", name, key)
			}
			seen[key] = struct{}{}
		}
	}
	return nil
}

// LookupPHPExtension finds the build of extension name for PHP major.minor and its thread-safety.
func (m Manifest) LookupPHPExtension(name, phpMinor string, ts bool, goos, goarch string) (Artifact, error) {
	for _, art := range m.PHPExtensions[name] {
		if art.PHP == phpMinor && art.TS == ts && art.OS == goos && art.Arch == goarch {
			return art, nil
		}
	}
	label := PHPExtensionLabels[name]
	if label == "" {
		label = name
	}
	return Artifact{}, errs.New(fmt.Sprintf("the %s PHP extension is not in the Phlo manifest for PHP %s %s on %s/%s",
		label, phpMinor, threadName(ts), goos, goarch))
}

// PHPExtKey is the folder name for one PHP build flavor, such as 8.3-nts.
func PHPExtKey(phpMinor string, ts bool) string {
	return phpMinor + "-" + threadName(ts)
}

func threadName(ts bool) string {
	if ts {
		return "ts"
	}
	return "nts"
}

// PHPThreadSafe reports whether the PHP build in dir is thread safe (php8ts.dll instead of php8.dll).
func PHPThreadSafe(dir string) bool {
	matches, _ := filepath.Glob(filepath.Join(dir, "php*ts.dll"))
	return len(matches) > 0
}

// PHPExtensionFiles lists downloaded or dropped-in extension files for the PHP build in phpDir.
// They live in runtime/phpext/{name}/{major.minor}-{ts|nts}/.
// Companion libraries such as libmemcached.dll stay in that folder but are not listed here.
func PHPExtensionFiles(layout paths.Layout, version, phpDir, goos string) []string {
	dirs := PHPExtPathDirs(layout, version, phpDir)
	var files []string
	for _, dir := range dirs {
		pattern := "*.so"
		if goos == "windows" {
			pattern = "php_*.dll"
		}
		matches, _ := filepath.Glob(filepath.Join(dir, pattern))
		for _, match := range matches {
			base := strings.ToLower(filepath.Base(match))
			if goos != "windows" && strings.HasPrefix(base, "lib") {
				continue
			}
			if info, err := os.Stat(match); err == nil && !info.IsDir() {
				files = append(files, match)
			}
		}
	}
	sort.Strings(files)
	return files
}

// PHPExtPathDirs are runtime/phpext folders for this PHP build. PHP needs them on PATH
// so companion DLLs next to an extension (for example libmemcached.dll) can load.
func PHPExtPathDirs(layout paths.Layout, version, phpDir string) []string {
	minor, err := php.Normalize(version)
	if err != nil {
		return nil
	}
	dirs, _ := filepath.Glob(filepath.Join(layout.Runtime(), "phpext", "*", PHPExtKey(minor, PHPThreadSafe(phpDir))))
	sort.Strings(dirs)
	return dirs
}

// PHPINI is the generated php.ini for the PHP build in phpDir, with its extra extensions.
// A runtime/phpext file is skipped when the PHP ext folder already has that extension.
func PHPINI(layout paths.Layout, version, phpDir, goos string) string {
	extDir := filepath.Join(phpDir, "ext")
	extensions := php.DiscoverExtensions(extDir)
	bundled := append([]string(nil), extensions...)
	for _, file := range PHPExtensionFiles(layout, version, phpDir, goos) {
		if !slices.Contains(bundled, phpExtName(file)) {
			extensions = append(extensions, file)
		}
	}
	return php.INI(extDir, extensions)
}

func phpExtName(file string) string {
	return strings.ToLower(filepath.Base(filepath.Dir(filepath.Dir(file))))
}

// HasPHPExtension reports whether files include one from runtime/phpext/{name}.
func HasPHPExtension(files []string, name string) bool {
	for _, file := range files {
		if phpExtName(file) == name {
			return true
		}
	}
	return false
}
