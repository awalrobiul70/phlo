package laravel

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"phlo/internal/errs"
	"phlo/internal/php"
	"phlo/internal/site"
)

// WebRoot is the directory Nginx should serve. A Laravel project is served from public.
func WebRoot(dir string) string {
	public := filepath.Join(dir, "public")
	if isFile(filepath.Join(dir, "artisan")) && isFile(filepath.Join(public, "index.php")) {
		return public
	}
	return dir
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Env makes Composer scripts such as artisan migrate use the same php.ini as the CLI.
func Env(ini string) []string {
	return []string{"PHPRC=" + ini}
}

// Command runs Composer with the selected PHP CLI to install Laravel in the working directory.
// constraint is a Composer version such as 8.*. An empty constraint installs the current release.
func Command(phpExe, ini, composer, constraint string) []string {
	args := []string{
		phpExe,
		"-c", ini,
		composer,
		"create-project",
		"laravel/laravel",
		".",
	}
	if constraint != "" {
		args = append(args, constraint)
	}
	return append(args, "--no-interaction")
}

// Release is the laravel/laravel constraint for this PHP version.
// PHP 8.2 and newer get the current release. Older PHP gets the newest Laravel that runs on it.
func Release(version string) (string, error) {
	if err := php.Accept(version); err != nil {
		return "", errs.New("PHP version must look like 8.3")
	}
	bits := strings.Split(version, ".")
	major, _ := strconv.Atoi(bits[0])
	minor, _ := strconv.Atoi(bits[1])
	switch {
	case major > 8 || (major == 8 && minor >= 2):
		return "", nil
	case major == 8 && minor == 1:
		return "10.*", nil
	case major == 8 && minor == 0:
		return "9.*", nil
	case major == 7 && minor >= 3:
		return "8.*", nil
	default:
		return "", errs.New("Laravel needs PHP 7.3 or newer. This folder uses " + version + ".")
	}
}

// AcceptPHP reports whether version can install a Laravel release.
func AcceptPHP(version string) error {
	_, err := Release(version)
	return err
}

// Prepare makes dir safe for composer create-project.
// An empty folder is ready. The Phlo placeholder index.php is removed.
// Any other file stays in place and the install is refused.
func Prepare(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	if len(entries) == 1 && entries[0].Name() == "index.php" && !entries[0].IsDir() {
		path := filepath.Join(dir, "index.php")
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(body) == site.IndexPHP {
			return os.Remove(path)
		}
	}
	return errs.New("folder is not empty")
}
