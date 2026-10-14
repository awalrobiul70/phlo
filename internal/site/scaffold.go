package site

import (
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"phlo/internal/errs"
)

// IndexPHP is the welcome page written into a new public_html or subdomain.
// Laravel install removes it only when the file is still exactly this text.
//
//go:embed welcome.php
var IndexPHP string

// Create makes home/{account}/public_html and subdomain.
func Create(home, account string) error {
	if err := ValidateAccount(account); err != nil {
		return err
	}
	dir := filepath.Join(home, account)
	if _, err := os.Stat(dir); err == nil {
		return errs.New("account already exists")
	}
	pub := filepath.Join(dir, PublicRoot)
	if err := os.MkdirAll(pub, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "subdomain"), 0o755); err != nil {
		return err
	}
	return writeIfMissing(filepath.Join(pub, "index.php"), IndexPHP)
}

// AddSubdomain creates home/{account}/subdomain/{name}.
func AddSubdomain(home, account, name string) error {
	if err := ValidateLabel(name); err != nil {
		return errs.New("subdomain name must use lowercase letters, numbers, and hyphens")
	}
	if reserved[name] {
		return errs.New("subdomain name is reserved on Windows")
	}
	if account != filepath.Base(account) || strings.ContainsAny(account, `/\`) {
		return errs.New("account not found")
	}
	dir := filepath.Join(home, account)
	if _, err := os.Stat(dir); err != nil {
		return errs.New("account not found")
	}
	dest := filepath.Join(dir, "subdomain", name)
	if _, err := os.Stat(dest); err == nil {
		return errs.New("subdomain already exists")
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	return writeIfMissing(filepath.Join(dest, "index.php"), IndexPHP)
}

func writeIfMissing(path, content string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}
