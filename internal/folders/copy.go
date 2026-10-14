// Package folders copies a projects or database folder to a new place and leaves the original as it was.
package folders

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"phlo/internal/paths"
)

// Copied is one finished copy. Skipped lists links that could not be recreated.
type Copied struct {
	Src     string
	Dst     string
	Skipped []string
	made    bool
}

// Empty reports whether dir is missing or has no entries. A file at dir is an error.
func Empty(dir string) (bool, error) {
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("%s is a file, not a folder", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

// Copy copies everything inside src into dst, which must be missing or empty.
// Links are recreated, not followed. An absolute link target inside src points at the same place inside dst.
// If the copy fails, what it created in dst is removed. src is only read.
func Copy(src, dst string) (Copied, error) {
	src, dst = filepath.Clean(src), filepath.Clean(dst)
	out := Copied{Src: src, Dst: dst}
	info, err := os.Stat(src)
	if err != nil {
		return out, err
	}
	if !info.IsDir() {
		return out, fmt.Errorf("%s is not a folder", src)
	}
	if paths.Overlap(src, dst) {
		return out, fmt.Errorf("cannot copy %s into %s because one folder is inside the other", src, dst)
	}
	empty, err := Empty(dst)
	if err != nil {
		return out, err
	}
	if !empty {
		return out, fmt.Errorf("%s already has files", dst)
	}
	if _, statErr := os.Stat(dst); errors.Is(statErr, fs.ErrNotExist) {
		if err := os.MkdirAll(dst, info.Mode().Perm()|0o700); err != nil {
			return out, err
		}
		out.made = true
	} else if err := os.Chmod(dst, info.Mode().Perm()|0o700); err != nil {
		return out, err
	}
	err = filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == src {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		mode := entry.Type()
		switch {
		case mode.IsDir():
			info, err := entry.Info()
			if err != nil {
				return err
			}
			return os.Mkdir(target, info.Mode().Perm()|0o700)
		case mode.IsRegular():
			return copyFile(path, target)
		case mode&(fs.ModeSymlink|fs.ModeIrregular) != 0:
			if err := copyLink(path, target, src, dst); err != nil {
				out.Skipped = append(out.Skipped, path)
			}
			return nil
		default:
			out.Skipped = append(out.Skipped, path)
			return nil
		}
	})
	if err != nil {
		_ = out.Undo()
		return Copied{Src: src, Dst: dst}, fmt.Errorf("copy %s to %s: %w", src, dst, err)
	}
	return out, nil
}

// Undo removes what Copy put in dst. dst was missing or empty before Copy, so nothing else is removed.
func (c Copied) Undo() error {
	if c.Dst == "" {
		return nil
	}
	if c.made {
		return os.RemoveAll(c.Dst)
	}
	entries, err := os.ReadDir(c.Dst)
	if err != nil {
		return err
	}
	var first error
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(c.Dst, entry.Name())); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm()|0o200)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chmod(dst, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}

func copyLink(src, dst, srcRoot, dstRoot string) error {
	target, err := os.Readlink(src)
	if err != nil {
		return err
	}
	if filepath.IsAbs(target) && paths.Within(target, srcRoot) {
		if rel, relErr := filepath.Rel(srcRoot, target); relErr == nil {
			target = filepath.Join(dstRoot, rel)
		}
	}
	if err := os.Symlink(target, dst); err == nil {
		return nil
	} else if info, statErr := os.Stat(src); statErr != nil || !info.IsDir() {
		return err
	}
	abs := target
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(filepath.Dir(dst), abs)
	}
	return junction(dst, abs)
}
