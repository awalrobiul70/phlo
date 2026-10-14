package runtime

import (
	"os"
	"path/filepath"

	"phlo/internal/paths"
)

// MigrateLegacyDirs moves runtime/{kind}/{os}-{arch} into runtime/{kind}/{version}
// when the version folder is missing. Existing version folders are left alone.
func MigrateLegacyDirs(manifest Manifest, layout paths.Layout, goos, goarch string) error {
	type item struct {
		kind string
		art  Artifact
		err  error
	}
	items := []item{
		{kind: "nginx", art: firstArt(manifest.LookupNginx(goos, goarch))},
		{kind: "mariadb", art: firstArt(manifest.LookupMariaDB(goos, goarch))},
		{kind: "redis", art: firstArt(manifest.LookupRedis(goos, goarch))},
		{kind: "memcached", art: firstArt(manifest.LookupMemcached(goos, goarch))},
		{kind: "mailpit", art: firstArt(manifest.LookupMailpit(goos, goarch))},
	}
	for _, entry := range items {
		if entry.art.Version == "" {
			continue
		}
		if err := migrateOne(layout, entry.kind, entry.art.Version, goos, goarch); err != nil {
			return err
		}
	}
	return nil
}

func firstArt(art Artifact, err error) Artifact {
	if err != nil {
		return Artifact{}
	}
	return art
}

func migrateOne(layout paths.Layout, kind, version, goos, goarch string) error {
	legacy := layout.ToolDir(kind, goos, goarch)
	if kind == "nginx" {
		legacy = layout.NginxDir(goos, goarch)
	}
	if kind == "mariadb" {
		legacy = layout.MariaDBDir(goos, goarch)
	}
	info, err := os.Stat(legacy)
	if err != nil || !info.IsDir() {
		return nil
	}
	dest := layout.VersionedToolDir(kind, version)
	if destInfo, err := os.Stat(dest); err == nil && destInfo.IsDir() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.Rename(legacy, dest)
}
