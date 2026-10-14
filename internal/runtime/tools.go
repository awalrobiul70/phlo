package runtime

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"phlo/internal/errs"
	"phlo/internal/paths"
)

// ToolSpec describes one kind in the manifest tools map.
type ToolSpec struct {
	Kind    string
	Label   string
	Service bool
	// Toolchain kinds are put on the project Terminal PATH.
	Toolchain bool
	Names     []string
}

var toolSpecs = []ToolSpec{
	{Kind: "mongodb", Label: "MongoDB", Service: true, Names: []string{"bin/mongod.exe", "bin/mongod", "mongod.exe", "mongod"}},
	{Kind: "mongosh", Label: "mongosh", Names: []string{"bin/mongosh.exe", "bin/mongosh", "mongosh.exe", "mongosh"}},
	{Kind: "postgres", Label: "PostgreSQL", Service: true, Names: []string{"bin/postgres.exe", "bin/postgres"}},
	{Kind: "node", Label: "Node.js", Toolchain: true, Names: []string{"node.exe", "bin/node", "node"}},
	{Kind: "python", Label: "Python", Toolchain: true, Names: []string{"python.exe", "bin/python3", "bin/python", "python3"}},
	{Kind: "java", Label: "Java", Toolchain: true, Names: []string{"bin/java.exe", "bin/java"}},
	{Kind: "gcc", Label: "C++ (GCC)", Toolchain: true, Names: []string{"bin/g++.exe", "bin/g++", "bin/gcc.exe", "bin/gcc"}},
	{Kind: "docker", Label: "Docker CLI", Toolchain: true, Names: []string{"docker.exe", "docker", "bin/docker.exe", "bin/docker"}},
	{Kind: "compose", Label: "Docker Compose", Toolchain: true, Names: []string{"docker-compose.exe", "docker-compose"}},
}

// ToolSpecs lists every tools kind in display order.
func ToolSpecs() []ToolSpec {
	return append([]ToolSpec(nil), toolSpecs...)
}

// LookupToolSpec returns the spec for kind.
func LookupToolSpec(kind string) (ToolSpec, bool) {
	for _, spec := range toolSpecs {
		if spec.Kind == kind {
			return spec, true
		}
	}
	return ToolSpec{}, false
}

// ToolLabel is the display name for kind.
func ToolLabel(kind string) string {
	if spec, ok := LookupToolSpec(kind); ok {
		return spec.Label
	}
	return kind
}

// ResolveTool finds the binary for kind under runtime/{kind}/{version}.
// An empty version uses the installed manifest build, then the only dropped-in build.
func ResolveTool(manifest Manifest, layout paths.Layout, kind, version, goos, goarch string) (string, error) {
	spec, ok := LookupToolSpec(kind)
	if !ok {
		return "", errs.New("unknown tool " + kind)
	}
	if version == "" {
		version = SelectedVersion("", ListTool(manifest, layout, kind, goos, goarch))
	}
	if version == "" {
		return "", errs.New(spec.Label + " is not installed")
	}
	names := spec.Names
	if art, err := manifest.LookupTool(kind, version, goos, goarch); err == nil {
		names = append([]string{art.Bin}, names...)
	}
	if exe := findVersionBinary(layout, kind, version, goos, goarch, names...); exe != "" {
		return exe, nil
	}
	return "", errs.New(spec.Label + " " + version + " is not installed")
}

// ListTool returns the manifest builds for this platform and any version folder under runtime/{kind}.
func ListTool(manifest Manifest, layout paths.Layout, kind, goos, goarch string) []Install {
	byVersion := map[string]Install{}
	for _, art := range manifest.Tools[kind] {
		if art.OS == goos && art.Arch == goarch {
			byVersion[art.Version] = Install{Version: art.Version, Downloadable: true}
		}
	}
	entries, _ := os.ReadDir(filepath.Join(layout.Runtime(), kind))
	for _, entry := range entries {
		if !entry.IsDir() || !componentVersionRe.MatchString(entry.Name()) {
			continue
		}
		item := byVersion[entry.Name()]
		item.Version = entry.Name()
		byVersion[entry.Name()] = item
	}
	list := make([]Install, 0, len(byVersion))
	for version, item := range byVersion {
		_, err := ResolveTool(manifest, layout, kind, version, goos, goarch)
		item.Installed = err == nil
		list = append(list, item)
	}
	sort.Slice(list, func(i, j int) bool { return CompareVersions(list[i].Version, list[j].Version) < 0 })
	return list
}

// ToolPathDirs are the folders a project Terminal puts on PATH for kind.
func ToolPathDirs(kind, exe, goos string) []string {
	dir := filepath.Dir(exe)
	switch kind {
	case "python":
		if goos == "windows" {
			return []string{dir, filepath.Join(dir, "Scripts")}
		}
	}
	return []string{dir}
}

// ToolHome is the install root above a bin folder, used for JAVA_HOME.
func ToolHome(exe string) string {
	dir := filepath.Dir(exe)
	if strings.EqualFold(filepath.Base(dir), "bin") {
		return filepath.Dir(dir)
	}
	return dir
}

// FindSibling returns the first of names that exists next to exe.
func FindSibling(exe string, names ...string) string {
	return findBinary(filepath.Dir(exe), names...)
}

// CompareVersions orders dotted numeric versions, so 10.1 sorts after 9.6.
func CompareVersions(a, b string) int {
	left := strings.Split(a, ".")
	right := strings.Split(b, ".")
	for i := 0; i < len(left) || i < len(right); i++ {
		var x, y int
		if i < len(left) {
			x, _ = strconv.Atoi(left[i])
		}
		if i < len(right) {
			y, _ = strconv.Atoi(right[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return strings.Compare(a, b)
}
