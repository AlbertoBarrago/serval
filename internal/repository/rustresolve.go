package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlbertoBarrago/serval/internal/analyzer"
	"github.com/AlbertoBarrago/serval/internal/analyzer/rust"
)

// externalRustRoots are path roots that always name a crate outside the
// repository, so resolution stops right away.
var externalRustRoots = map[string]bool{"std": true, "core": true, "alloc": true}

// rustTargetDirs are the Cargo directories whose top-level .rs files
// are each the root of their own crate.
var rustTargetDirs = map[string]bool{"tests": true, "examples": true, "benches": true}

// RustResolver resolves Rust `mod` declarations and `use` paths to the
// module file they name. Each file belongs to the crate of the nearest
// Cargo.toml above it. Paths into another crate (std, dependencies,
// other workspace members) are recorded as external, matching the
// treatment of JS/TS bare specifiers and Go module dependencies; the
// one exception is the crate's own library, named from its binaries,
// tests and examples.
type RustResolver struct {
	// crates caches the crate (nil for none) found from each directory.
	crates map[string]*RustCrate
	// rootMods caches, per crate root file, the names of the modules it
	// declares at file level (`mod x;`).
	rootMods map[string]map[string]bool
}

// NewRustResolver builds a resolver with empty caches.
func NewRustResolver() *RustResolver {
	return &RustResolver{
		crates:   make(map[string]*RustCrate),
		rootMods: make(map[string]map[string]bool),
	}
}

// crateRoot describes the crate a file belongs to: its root file
// (lib.rs, main.rs, a bin/test/example file; "" when unknown) and the
// module directory that `crate::` paths start from.
type crateRoot struct {
	file string
	dir  string
}

// Resolve returns the module file imp refers to from fromFile, or nil
// when it is external or not found. For a `use` path that names an item
// rather than a module (crate::a::b::Item), the longest prefix that is
// a module file wins: like Python's from-import, item vs submodule
// isn't decidable without parsing the target.
func (r *RustResolver) Resolve(fromFile string, imp analyzer.RawImport) []string {
	crate := r.crateFor(filepath.Dir(fromFile))
	root := r.rootOf(crate, fromFile)
	current := filepath.Join(append([]string{moduleDir(fromFile, root)}, imp.Scope...)...)

	var target string
	if imp.ModDecl {
		target = moduleFile(filepath.Join(current, imp.Specifier), root)
	} else {
		target = resolveUse(crate, root, current, strings.Split(imp.Specifier, "::"))
	}
	if target == "" || target == fromFile {
		return nil
	}
	return []string{target}
}

func resolveUse(crate *RustCrate, root crateRoot, current string, segs []string) string {
	switch first := segs[0]; {
	case first == "" || externalRustRoots[first]:
		return ""
	case first == "crate":
		if root.dir == "" {
			return ""
		}
		return longestModule(root.dir, segs[1:], 0, root)
	case first == "self":
		return longestModule(current, segs[1:], 0, root)
	case first == "super":
		dir := current
		for len(segs) > 0 && segs[0] == "super" {
			if dir == root.dir {
				// super:: past the crate root does not compile.
				return ""
			}
			dir = filepath.Dir(dir)
			segs = segs[1:]
		}
		return longestModule(dir, segs, 0, root)
	}

	// A bare path (2018 edition) names a child of the current module,
	// or else an external crate: require at least one segment to match
	// a local module file before claiming it. A local child shadows the
	// crate's own library name, as it does in rustc.
	if f := longestModule(current, segs, 1, root); f != "" {
		return f
	}
	if crate != nil && crate.libRoot != "" && root.file != crate.libRoot && segs[0] == crate.LibName {
		lib := crateRoot{file: crate.libRoot, dir: filepath.Dir(crate.libRoot)}
		return longestModule(lib.dir, segs[1:], 0, lib)
	}
	return ""
}

// longestModule returns the module file for the longest prefix of segs
// (at least minLen segments) under base, or "". With no segment left
// (k == 0), base itself may be an inline module with no file of its
// own (`mod outer { mod child; }` in lib.rs): it is then defined in the
// nearest ancestor module file, up to the crate root.
func longestModule(base string, segs []string, minLen int, root crateRoot) string {
	for k := len(segs); k >= minLen; k-- {
		dir := filepath.Join(append([]string{base}, segs[:k]...)...)
		if f := moduleFile(dir, root); f != "" {
			return f
		}
	}
	if minLen > 0 || root.dir == "" {
		return ""
	}
	for dir := base; strings.HasPrefix(dir, root.dir+string(filepath.Separator)); {
		dir = filepath.Dir(dir)
		if f := moduleFile(dir, root); f != "" {
			return f
		}
	}
	return ""
}

// moduleFile returns the file defining the module whose children live
// in dir: the crate root file for the crate root directory, else dir.rs
// or dir/mod.rs.
func moduleFile(dir string, root crateRoot) string {
	if dir == root.dir && isFile(root.file) {
		return root.file
	}
	if isFile(dir + ".rs") {
		return dir + ".rs"
	}
	if f := filepath.Join(dir, "mod.rs"); isFile(f) {
		return f
	}
	return ""
}

// moduleDir returns the directory holding the child modules of the
// module defined by file: its own directory for a crate root or a
// mod.rs, else a sibling directory named after it (src/a.rs -> src/a,
// and src/a/main.rs -> src/a/main when it is not a crate root). Without
// a known crate, lib.rs and main.rs are assumed to be crate roots.
func moduleDir(file string, root crateRoot) string {
	dir := filepath.Dir(file)
	base := filepath.Base(file)
	switch {
	case base == "mod.rs" || file == root.file:
		return dir
	case root.file == "" && (base == "lib.rs" || base == "main.rs"):
		return dir
	}
	return strings.TrimSuffix(file, ".rs")
}

// rootOf locates the crate root that file belongs to, following Cargo's
// default target layout: src/ (lib.rs or main.rs, see srcRoot),
// src/bin/x.rs or src/bin/x/main.rs, and the same two shapes under
// tests/, examples/ and benches/. Anything else (build.rs) is its own
// root. Without a crate, the root is unknown and crate:: paths stay
// unresolved.
func (r *RustResolver) rootOf(crate *RustCrate, file string) crateRoot {
	if crate == nil {
		return crateRoot{}
	}
	rel, err := filepath.Rel(crate.dir, file)
	if err != nil {
		return crateRoot{}
	}
	segs := strings.Split(rel, string(filepath.Separator))

	// target describes a directory where each top-level file is a
	// crate root, and each subdirectory one rooted at its main.rs. A
	// subdirectory without one (tests/common/mod.rs) is a helper shared
	// by several roots: its crate:: paths are ambiguous, so the root
	// file stays unknown rather than naming a file that doesn't exist.
	target := func(base string, segs []string) crateRoot {
		if len(segs) == 1 {
			return crateRoot{file: file, dir: base}
		}
		dir := filepath.Join(base, segs[0])
		if main := filepath.Join(dir, "main.rs"); isFile(main) {
			return crateRoot{file: main, dir: dir}
		}
		return crateRoot{dir: base}
	}

	switch {
	case len(segs) >= 3 && segs[0] == "src" && segs[1] == "bin":
		return target(filepath.Join(crate.dir, "src", "bin"), segs[2:])
	case len(segs) >= 2 && segs[0] == "src":
		return r.srcRoot(crate, file, segs[1])
	case len(segs) >= 2 && rustTargetDirs[segs[0]]:
		return target(filepath.Join(crate.dir, segs[0]), segs[1:])
	default:
		return crateRoot{file: file, dir: filepath.Dir(file)}
	}
}

// srcRoot picks the root of a file under src/ (first names the entry
// right below src/). With both a library and src/main.rs, the binary
// owns the modules that main.rs declares and the library does not
// (`mod cli;` in main.rs makes src/cli.rs and src/cli/ binary modules);
// everything else belongs to the library.
func (r *RustResolver) srcRoot(crate *RustCrate, file, first string) crateRoot {
	src := filepath.Join(crate.dir, "src")
	main := crateRoot{file: filepath.Join(src, "main.rs"), dir: src}
	if crate.libRoot == "" || file == main.file {
		return main
	}
	lib := crateRoot{file: crate.libRoot, dir: filepath.Dir(crate.libRoot)}
	if !isFile(main.file) || file == lib.file {
		return lib
	}
	name := strings.TrimSuffix(first, ".rs")
	if r.declaredMods(main.file)[name] && !r.declaredMods(lib.file)[name] {
		return main
	}
	return lib
}

// declaredMods returns the modules that root declares at file level.
// An unreadable root is reported on stderr and treated as declaring
// none, the same way Scanner reports unreadable files.
func (r *RustResolver) declaredMods(root string) map[string]bool {
	if mods, ok := r.rootMods[root]; ok {
		return mods
	}
	mods := make(map[string]bool)
	r.rootMods[root] = mods

	content, err := os.ReadFile(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: reading %s: %v\n", root, err)
		return mods
	}
	imports, err := rust.New().ExtractImports(root, content)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: skipping imports of %s: %v\n", root, err)
		return mods
	}
	for _, imp := range imports {
		if imp.ModDecl && len(imp.Scope) == 0 {
			mods[imp.Specifier] = true
		}
	}
	return mods
}

// crateFor returns the crate of the nearest Cargo.toml at or above dir,
// or nil. A manifest that can't be read is reported on stderr and
// treated as absent, the same way Scanner reports unreadable files:
// that file's crate:: paths go unresolved rather than aborting the scan.
func (r *RustResolver) crateFor(dir string) *RustCrate {
	if crate, ok := r.crates[dir]; ok {
		return crate
	}
	crate, err := LoadRustCrate(dir)
	if err != nil {
		// The manifest exists but can't be read: stop here instead of
		// walking up, or the file would be misattributed to an outer crate.
		fmt.Fprintf(os.Stderr, "warning: ignoring %s: %v\n", filepath.Join(dir, "Cargo.toml"), err)
		r.crates[dir] = nil
		return nil
	}
	if crate == nil {
		if parent := filepath.Dir(dir); parent != dir {
			crate = r.crateFor(parent)
		}
	}
	r.crates[dir] = crate
	return crate
}
