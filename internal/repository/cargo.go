package repository

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// RustCrate holds what RustResolver needs from a Cargo.toml: where the
// crate lives and how its library target can be named from the
// crate's own binaries, tests and examples.
type RustCrate struct {
	// dir is the directory containing Cargo.toml.
	dir string
	// LibName is the library target's name as written in Rust paths
	// ([lib] name, else [package] name, with "-" turned into "_");
	// empty for a crate without a [package] (e.g. a virtual workspace
	// manifest).
	LibName string
	// libRoot is the library's root file ([lib] path, else src/lib.rs),
	// or "" when the crate has no library target on disk.
	libRoot string
}

// Matches a TOML table header, [package] or [lib], or an array-of-tables
// header, [[bin]] or [[test]], capturing it whole. Array tables must be
// recognized too, or a [[bin]]'s name and path would be read as the
// preceding [lib]'s.
var reTOMLTable = regexp.MustCompile(`^(\[\[?\s*[\w.-]+\s*\]\]?)`)

// Matches a string key/value pair, capturing the key and the value of
// either a basic string (name = "my-crate") or a literal string
// (name = 'my-crate').
var reTOMLString = regexp.MustCompile(`^([\w-]+)\s*=\s*(?:"([^"]*)"|'([^']*)')`)

// LoadRustCrate reads dir/Cargo.toml. It returns nil, without an error,
// when dir has no Cargo.toml.
//
// The manifest is read line by line for the few keys v0.1 uses
// ([package] name, [lib] name and path) instead of with a TOML parser:
// those keys are plain basic or literal strings in practice, and a full parser
// would be a new dependency for three fields.
func LoadRustCrate(dir string) (*RustCrate, error) {
	data, err := os.ReadFile(filepath.Join(dir, "Cargo.toml"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var table, pkgName, libName, libPath string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if m := reTOMLTable.FindStringSubmatch(line); m != nil {
			table = strings.Join(strings.Fields(m[1]), "")
			continue
		}
		m := reTOMLString.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		value := m[2] + m[3] // only one of the two alternatives matched
		switch {
		case table == "[package]" && m[1] == "name":
			pkgName = value
		case table == "[lib]" && m[1] == "name":
			libName = value
		case table == "[lib]" && m[1] == "path":
			libPath = value
		}
	}

	crate := &RustCrate{dir: dir}
	if libName == "" {
		libName = pkgName
	}
	crate.LibName = strings.ReplaceAll(libName, "-", "_")

	if libPath == "" {
		libPath = filepath.Join("src", "lib.rs")
	}
	if candidate := filepath.Join(dir, filepath.FromSlash(libPath)); isFile(candidate) {
		crate.libRoot = candidate
	}
	return crate, nil
}

func isFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}
