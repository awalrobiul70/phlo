package app

import (
	"fmt"
	"runtime"

	"phlo/internal/errs"
	"phlo/internal/paths"
)

// rootProblem explains why Phlo cannot run from root, or returns "".
// The Windows builds of Nginx, PHP, and PostgreSQL read paths in the ANSI code page,
// so with any non-ASCII character in the path Nginx and PostgreSQL do not start
// and PHP loads none of its extensions. MariaDB, MongoDB, and the Go programs are fine.
func rootProblem(root, goos string) string {
	if goos != "windows" {
		return ""
	}
	part := paths.NonASCIIPart(root)
	if part == "" {
		return ""
	}
	return fmt.Sprintf("Phlo cannot run from this folder: %s\n"+
		"The folder name \"%s\" has non-English characters. On Windows, Nginx, PHP, and PostgreSQL cannot open a path like that, so your sites would not load.\n"+
		"To fix it:\n"+
		"1. Close the Phlo window.\n"+
		"2. Move the whole Phlo folder to a path with only English letters, numbers, spaces, - or _, for example D:\\Phlo\n"+
		"3. Start Phlo again from the new place. Your sites, settings, and databases move with the folder.",
		root, part)
}

// CheckRoot refuses to run PHP or the stack from a root that Windows tools cannot open.
func (a *App) CheckRoot() error {
	if msg := rootProblem(a.Layout.Root, runtime.GOOS); msg != "" {
		return errs.New(msg)
	}
	return nil
}
