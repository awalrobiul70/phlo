package runtime

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const maxUncompressed = 8 << 30

func extract(format, src, dest string, skip []string) error {
	switch format {
	case "zip":
		return extractZip(src, dest, skip)
	case "tar.gz":
		return extractTarGZ(src, dest, skip)
	case "file":
		return fmt.Errorf("file artifacts are copied, not extracted")
	default:
		return fmt.Errorf("unknown archive format %s", format)
	}
}

func extractZip(src, dest string, skip []string) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer reader.Close()
	var written int64
	for _, file := range reader.File {
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive contains a symlink")
		}
		target, err := safePath(dest, file.Name)
		if err != nil {
			return err
		}
		if skipped(file.Name, skip) {
			continue
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		n, err := copyZipFile(file, target, maxUncompressed-written)
		if err != nil {
			return err
		}
		written += n
		if written > maxUncompressed {
			return fmt.Errorf("archive is too large")
		}
	}
	return flatten(dest)
}

func copyZipFile(file *zip.File, target string, budget int64) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, err
	}
	src, err := file.Open()
	if err != nil {
		return 0, err
	}
	defer src.Close()
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode(file.Mode()))
	if err != nil {
		return 0, err
	}
	defer dst.Close()
	n, err := io.Copy(dst, io.LimitReader(src, budget+1))
	if err != nil {
		return n, err
	}
	if n > budget {
		return n, fmt.Errorf("archive is too large")
	}
	return n, nil
}

type pendingLink struct {
	path     string
	linkname string
	source   string
	hard     bool
}

// extractTarGZ writes regular files first and creates links afterwards, so no
// file is ever written through a link. A link must point inside dest.
func extractTarGZ(src, dest string, skip []string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var written int64
	var links []pendingLink
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeXGlobalHeader || header.Typeflag == tar.TypeXHeader {
			continue
		}
		target, err := safePath(dest, header.Name)
		if err != nil {
			return err
		}
		if skipped(header.Name, skip) {
			continue
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode(header.FileInfo().Mode()))
			if err != nil {
				return err
			}
			n, copyErr := io.Copy(dst, io.LimitReader(tr, maxUncompressed-written+1))
			closeErr := dst.Close()
			written += n
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			if written > maxUncompressed {
				return fmt.Errorf("archive is too large")
			}
		case tar.TypeSymlink:
			source, err := symlinkSource(dest, header.Name, header.Linkname)
			if err != nil {
				return err
			}
			links = append(links, pendingLink{path: target, linkname: header.Linkname, source: source})
		case tar.TypeLink:
			source, err := safePath(dest, header.Linkname)
			if err != nil {
				return err
			}
			links = append(links, pendingLink{path: target, source: source, hard: true})
		}
	}
	for _, link := range links {
		if err := makeLink(link); err != nil {
			return err
		}
	}
	return flatten(dest)
}

func symlinkSource(dest, name, linkname string) (string, error) {
	linkname = strings.ReplaceAll(linkname, "\\", "/")
	if linkname == "" || strings.HasPrefix(linkname, "/") || strings.Contains(linkname, ":") || strings.Contains(linkname, "\x00") {
		return "", fmt.Errorf("illegal link %q -> %q", name, linkname)
	}
	joined := path.Join(path.Dir(strings.ReplaceAll(name, "\\", "/")), linkname)
	source, err := safePath(dest, joined)
	if err != nil {
		return "", fmt.Errorf("illegal link %q -> %q", name, linkname)
	}
	return source, nil
}

// makeLink creates a symlink, or copies the file when the system cannot make
// one (Windows without developer mode) or when the archive used a hard link.
func makeLink(link pendingLink) error {
	if err := os.MkdirAll(filepath.Dir(link.path), 0o755); err != nil {
		return err
	}
	_ = os.Remove(link.path)
	if !link.hard {
		if err := os.Symlink(filepath.FromSlash(link.linkname), link.path); err == nil {
			return nil
		}
	}
	info, err := os.Stat(link.source)
	if err != nil || info.IsDir() {
		return nil
	}
	return copyFile(link.source, link.path, info.Mode().Perm())
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode(mode))
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// fileMode keeps the archive's exec bits and always leaves the file writable
// by its owner, so a later download can replace the folder.
func fileMode(mode os.FileMode) os.FileMode {
	perm := mode.Perm()
	if perm == 0 {
		return 0o644
	}
	return perm | 0o600
}

// skipped reports whether any path segment matches one of the skip patterns.
func skipped(name string, skip []string) bool {
	if len(skip) == 0 {
		return false
	}
	for _, segment := range strings.Split(path.Clean(strings.ReplaceAll(name, "\\", "/")), "/") {
		for _, pattern := range skip {
			if ok, _ := path.Match(pattern, segment); ok {
				return true
			}
		}
	}
	return false
}

func safePath(dest, name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\x00") || strings.Contains(name, ":") {
		return "", fmt.Errorf("illegal archive path %q", name)
	}
	cleaned := path.Clean(name)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("illegal archive path %q", name)
	}
	full := filepath.Join(dest, filepath.FromSlash(cleaned))
	rel, err := filepath.Rel(filepath.Clean(dest), full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("illegal archive path %q", name)
	}
	return full, nil
}

func flatten(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var dirs, files []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry)
		} else {
			files = append(files, entry)
		}
	}
	if len(files) != 0 || len(dirs) != 1 {
		return nil
	}
	inner := filepath.Join(dir, ".phlo-flatten")
	if err := os.Rename(filepath.Join(dir, dirs[0].Name()), inner); err != nil {
		return err
	}
	children, err := os.ReadDir(inner)
	if err != nil {
		return err
	}
	for _, child := range children {
		from := filepath.Join(inner, child.Name())
		to := filepath.Join(dir, child.Name())
		if err := os.Rename(from, to); err != nil {
			return err
		}
	}
	return os.Remove(inner)
}
