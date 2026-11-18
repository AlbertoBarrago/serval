package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// RustCrate holds what RustResolver needs from a Cargo.toml: where the
// crate lives, how its library target is named in Rust paths, which
// local crates it depends on, and the workspace it declares, if any.
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
	// deps are the crate's dependencies that may name a local crate:
	// `path = "..."` or `workspace = true`, from every dependency table
	// ([dependencies], [dev-dependencies], [build-dependencies] and
	// their [target.'cfg(...)'] forms), in manifest order.
	deps []rustDep
	// workspace is the [workspace] this manifest declares, or nil.
	workspace *rustWorkspace
}

// rustDep is one dependency entry that can point at a local crate.
type rustDep struct {
	// key is the dependency name as written in the manifest.
	key string
	// renamed is true when the entry sets `package = "..."`: the crate
	// is then known in Rust paths by key, not by its own library name.
	renamed bool
	// dir is the dependency's directory (absolute), "" when it has no
	// `path`.
	dir string
	// inherited is true for `workspace = true`: the entry of the same
	// key in [workspace.dependencies] gives the directory and rename.
	inherited bool
}

// rustWorkspace is the [workspace] section of a root Cargo.toml.
type rustWorkspace struct {
	dir string
	// members are the directories listed in `members` (globs expanded,
	// keeping those with a Cargo.toml), minus `exclude`.
	members map[string]bool
	// exclude are the directories listed in `exclude` (absolute).
	exclude []string
	// deps maps a [workspace.dependencies] key with a `path` to its
	// entry, for members that inherit it with `workspace = true`.
	deps map[string]rustDep
}

// hasMember reports whether the crate in dir is a member of w: the
// root package itself, or a directory listed in `members`.
func (w *rustWorkspace) hasMember(dir string) bool {
	return dir == w.dir || w.members[dir]
}

// contains reports whether dir lies inside the workspace and is not
// excluded. Cargo makes such a directory a member when a member
// depends on it by path, even if `members` does not list it.
func (w *rustWorkspace) contains(dir string) bool {
	if dir != w.dir && !strings.HasPrefix(dir, w.dir+string(filepath.Separator)) {
		return false
	}
	for _, ex := range w.exclude {
		if dir == ex || strings.HasPrefix(dir, ex+string(filepath.Separator)) {
			return false
		}
	}
	return true
}

// LoadRustCrate reads dir/Cargo.toml. It returns nil, without an error,
// when dir has no Cargo.toml.
//
// The manifest is read with readManifest, a minimal line-based reader
// for the few keys used here ([package] name, [lib] name and path,
// dependency `path`/`package`/`workspace`, [workspace] members and
// exclude) instead of with a TOML parser: those keys are plain strings,
// booleans and string arrays in practice, and a full parser would be a
// new dependency for a handful of fields.
func LoadRustCrate(dir string) (*RustCrate, error) {
	data, err := os.ReadFile(filepath.Join(dir, "Cargo.toml"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	crate := &RustCrate{dir: dir}
	var pkgName, libName, libPath string
	var ws *rustWorkspace
	var memberPatterns []string
	// Dependencies declared as [dependencies.foo] tables collect their
	// fields over several lines; depIndex finds the entry being built.
	depIndex := make(map[string]int)
	wsDepFields := make(map[string]map[string]string)
	var wsDepOrder []string

	addDepField := func(table, key, field, value string) {
		id := table + "|" + key
		i, ok := depIndex[id]
		if !ok {
			i = len(crate.deps)
			depIndex[id] = i
			crate.deps = append(crate.deps, rustDep{key: key})
		}
		applyDepField(&crate.deps[i], dir, field, value)
	}

	for _, e := range readManifest(string(data)) {
		table := e.table
		switch {
		case len(table) == 1 && table[0] == "package" && len(e.key) == 1 && e.key[0] == "name":
			pkgName, _ = tomlString(e.value)
		case len(table) == 1 && table[0] == "lib" && !e.arrayTable && len(e.key) == 1:
			switch e.key[0] {
			case "name":
				libName, _ = tomlString(e.value)
			case "path":
				libPath, _ = tomlString(e.value)
			}
		case len(table) == 1 && table[0] == "workspace":
			if ws == nil {
				ws = &rustWorkspace{dir: dir}
			}
			switch strings.Join(e.key, ".") {
			case "members":
				memberPatterns = tomlStrings(e.value)
			case "exclude":
				for _, ex := range tomlStrings(e.value) {
					ws.exclude = append(ws.exclude, filepath.Join(dir, filepath.FromSlash(ex)))
				}
			}
		case len(table) >= 2 && table[0] == "workspace" && table[1] == "dependencies":
			if ws == nil {
				ws = &rustWorkspace{dir: dir}
			}
			name, fields := depFields(table[2:], e.key, e.value)
			if name == "" {
				continue
			}
			if wsDepFields[name] == nil {
				wsDepFields[name] = make(map[string]string)
				wsDepOrder = append(wsDepOrder, name)
			}
			for k, v := range fields {
				wsDepFields[name][k] = v
			}
		default:
			depTable, rest := dependencyTable(table)
			if depTable == "" {
				continue
			}
			name, fields := depFields(rest, e.key, e.value)
			if name == "" {
				continue
			}
			for field, value := range fields {
				addDepField(depTable, name, field, value)
			}
		}
	}

	// Keep only the entries that can name a local crate.
	local := crate.deps[:0]
	for _, d := range crate.deps {
		if d.dir != "" || d.inherited {
			local = append(local, d)
		}
	}
	crate.deps = local

	if ws != nil {
		ws.members = expandMembers(dir, memberPatterns, ws)
		ws.deps = make(map[string]rustDep)
		for _, name := range wsDepOrder {
			d := rustDep{key: name}
			for field, value := range wsDepFields[name] {
				applyDepField(&d, dir, field, value)
			}
			if d.dir != "" {
				ws.deps[name] = d
			}
		}
		crate.workspace = ws
	}

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

// applyDepField records one field of a dependency entry, with a
// relative `path` taken from base.
func applyDepField(d *rustDep, base, field, value string) {
	switch field {
	case "path":
		if p, ok := tomlString(value); ok && p != "" {
			d.dir = filepath.Clean(filepath.Join(base, filepath.FromSlash(p)))
		}
	case "package":
		if _, ok := tomlString(value); ok {
			d.renamed = true
		}
	case "workspace":
		d.inherited = strings.TrimSpace(value) == "true"
	}
}

// dependencyTable reports whether a table header names a dependency
// table, returning its kind ("dependencies", "dev-dependencies",
// "build-dependencies", prefixed by the target spec for
// [target.'cfg(...)'.dependencies]) and the header keys after it
// ([dependencies.foo] -> ["foo"]).
func dependencyTable(table []string) (string, []string) {
	isDeps := func(s string) bool {
		return s == "dependencies" || s == "dev-dependencies" || s == "build-dependencies" ||
			s == "dev_dependencies" || s == "build_dependencies"
	}
	switch {
	case len(table) >= 1 && isDeps(table[0]):
		return table[0], table[1:]
	case len(table) >= 3 && table[0] == "target" && isDeps(table[2]):
		return "target." + table[1] + "." + table[2], table[3:]
	}
	return "", nil
}

// depFields returns the dependency name and its fields for one entry of
// a dependency table. rest are the header keys after the table name
// ([dependencies.foo] gives ["foo"]), key the entry's dotted key:
//
//	[dependencies]      foo = { path = "x" }   -> foo, {path: "x"}
//	[dependencies]      foo.workspace = true   -> foo, {workspace: true}
//	[dependencies]      foo = "1.0"            -> foo, {}
//	[dependencies.foo]  path = "x"             -> foo, {path: "x"}
func depFields(rest, key []string, value string) (string, map[string]string) {
	switch {
	case len(rest) == 0 && len(key) == 1:
		fields := make(map[string]string)
		if t, ok := tomlInlineTable(value); ok {
			for k, v := range t {
				fields[k] = v
			}
		}
		return key[0], fields
	case len(rest) == 0 && len(key) == 2:
		return key[0], map[string]string{key[1]: value}
	case len(rest) == 1 && len(key) == 1:
		return rest[0], map[string]string{key[0]: value}
	}
	return "", nil
}

// expandMembers expands the `members` patterns of the workspace rooted
// at dir (Cargo accepts globs such as crates/*), keeping directories
// that hold a Cargo.toml and are not excluded. A malformed pattern is
// reported on stderr and skipped, so its crates resolve as external.
func expandMembers(dir string, patterns []string, ws *rustWorkspace) map[string]bool {
	members := make(map[string]bool)
	for _, p := range patterns {
		matches, err := filepath.Glob(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: ignoring workspace member pattern %q in %s: %v\n", p, filepath.Join(dir, "Cargo.toml"), err)
			continue
		}
		for _, m := range matches {
			m = filepath.Clean(m)
			if isFile(filepath.Join(m, "Cargo.toml")) && ws.contains(m) {
				members[m] = true
			}
		}
	}
	return members
}

func isFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// manifestEntry is one key/value pair of a Cargo.toml, with the table
// header it sits under.
type manifestEntry struct {
	// table are the header's dotted keys ([target.'cfg(unix)'.dependencies]
	// gives ["target", "cfg(unix)", "dependencies"]), nil before any header.
	table []string
	// arrayTable is true under an array-of-tables header ([[bin]]), so a
	// [[bin]]'s name and path are not read as the preceding [lib]'s.
	arrayTable bool
	// key are the entry's dotted keys (foo.workspace gives ["foo", "workspace"]).
	key []string
	// value is the raw value, comments removed; a multi-line array is
	// joined onto one line.
	value string
}

// Matches the opening of a multi-line basic or literal string.
var reMultilineString = regexp.MustCompile(`^("""|''')`)

// readManifest splits a Cargo.toml into key/value entries. It is not a
// TOML parser: it understands table and array-of-tables headers, dotted
// and quoted keys, comments, multi-line arrays (members = [ ... ]), and
// skips multi-line strings so their contents are never read as keys.
// Inline tables are left as raw values for tomlInlineTable.
func readManifest(data string) []manifestEntry {
	var out []manifestEntry
	var table []string
	arrayTable := false
	lines := strings.Split(data, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(cutComment(lines[i]))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			arrayTable = strings.HasPrefix(line, "[[")
			header := strings.TrimPrefix(strings.TrimPrefix(line, "["), "[")
			header = strings.TrimSuffix(strings.TrimSuffix(header, "]"), "]")
			table = splitDotted(header)
			continue
		}
		eq := indexOutsideQuotes(line, '=')
		if eq < 0 {
			continue
		}
		key := splitDotted(line[:eq])
		value := strings.TrimSpace(line[eq+1:])

		if m := reMultilineString.FindString(value); m != "" && !strings.Contains(value[len(m):], m) {
			// Skip to the line closing the multi-line string.
			for i++; i < len(lines) && !strings.Contains(lines[i], m); i++ {
			}
			continue
		}
		for bracketDepth(value) > 0 && i+1 < len(lines) {
			i++
			value += " " + strings.TrimSpace(cutComment(lines[i]))
		}
		out = append(out, manifestEntry{table: table, arrayTable: arrayTable, key: key, value: value})
	}
	return out
}

// cutComment removes a trailing # comment, ignoring # inside strings.
func cutComment(line string) string {
	if i := indexOutsideQuotes(line, '#'); i >= 0 {
		return line[:i]
	}
	return line
}

// scanOutsideStrings calls visit for each byte of s that is not inside
// a basic ("...", with \ escapes) or literal ('...') string, quotes
// included, and returns the first index visit accepts, or -1.
func scanOutsideStrings(s string, visit func(i int) bool) int {
	var quote byte
	for i := 0; i < len(s); i++ {
		switch {
		case quote == '"' && s[i] == '\\':
			i++
		case quote != 0:
			if s[i] == quote {
				quote = 0
			}
		case s[i] == '"' || s[i] == '\'':
			quote = s[i]
		case visit(i):
			return i
		}
	}
	return -1
}

// indexOutsideQuotes returns the index of the first c in s that is not
// inside a string, or -1.
func indexOutsideQuotes(s string, c byte) int {
	return scanOutsideStrings(s, func(i int) bool { return s[i] == c })
}

// bracketDepth returns how many [ or { in s are still open, ignoring
// those inside strings.
func bracketDepth(s string) int {
	depth := 0
	scanOutsideStrings(s, func(i int) bool {
		switch s[i] {
		case '[', '{':
			depth++
		case ']', '}':
			depth--
		}
		return false
	})
	return depth
}

// splitDotted splits a dotted key (a.'b.c'."d") into its parts, with
// surrounding whitespace and quotes removed.
func splitDotted(s string) []string {
	var parts []string
	for {
		i := indexOutsideQuotes(s, '.')
		part := s
		if i >= 0 {
			part = s[:i]
		}
		part = strings.TrimSpace(part)
		if v, ok := tomlString(part); ok {
			part = v
		}
		parts = append(parts, part)
		if i < 0 {
			return parts
		}
		s = s[i+1:]
	}
}

// tomlString returns the contents of a basic ("...") or literal ('...')
// single-line string. Escapes are not decoded: the values read here
// (crate names, relative paths) do not use them in practice.
func tomlString(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1], true
	}
	return "", false
}

// Matches one basic or literal string inside an array.
var reTOMLArrayString = regexp.MustCompile(`"([^"]*)"|'([^']*)'`)

// tomlStrings returns the strings of an array value (["a", 'b']).
func tomlStrings(v string) []string {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "[") {
		return nil
	}
	var out []string
	for _, m := range reTOMLArrayString.FindAllStringSubmatch(v, -1) {
		out = append(out, m[1]+m[2]) // only one of the two alternatives matched
	}
	return out
}

// tomlInlineTable returns the raw values of an inline table's top-level
// keys ({ path = "x", features = ["a, b"] } gives path and features).
func tomlInlineTable(v string) (map[string]string, bool) {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "{") || !strings.HasSuffix(v, "}") {
		return nil, false
	}
	fields := make(map[string]string)
	for _, item := range splitOutside(v[1:len(v)-1], ',') {
		eq := indexOutsideQuotes(item, '=')
		if eq < 0 {
			continue
		}
		key := splitDotted(item[:eq])
		if len(key) == 1 {
			fields[key[0]] = strings.TrimSpace(item[eq+1:])
		}
	}
	return fields, true
}

// splitOutside splits s on sep where it is outside strings, brackets
// and braces.
func splitOutside(s string, sep byte) []string {
	var parts []string
	start, depth := 0, 0
	scanOutsideStrings(s, func(i int) bool {
		switch s[i] {
		case '[', '{':
			depth++
		case ']', '}':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
		return false
	})
	return append(parts, s[start:])
}
