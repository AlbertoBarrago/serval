package repository_test

import (
	"path/filepath"
	"testing"

	"github.com/AlbertoBarrago/serval/internal/analyzer"
	"github.com/AlbertoBarrago/serval/internal/repository"
)

// rustCrate lays out a crate with a library, a binary, an integration
// test and a module tree mixing a.rs and a/mod.rs styles.
func rustCrate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "Cargo.toml"), "[package]\nname = \"my-app\"\nversion = \"0.1.0\"\n")
	for _, f := range []string{
		"src/lib.rs",
		"src/main.rs",
		"src/auth.rs",
		"src/auth/token.rs",
		"src/db/mod.rs",
		"src/db/pool.rs",
		"src/bin/tool/main.rs",
		"src/bin/tool/args.rs",
		"tests/api.rs",
	} {
		mustWriteFile(t, filepath.Join(root, filepath.FromSlash(f)), "")
	}
	return root
}

func resolveRust(t *testing.T, root, from string, imp analyzer.RawImport) string {
	t.Helper()
	got := repository.NewRustResolver().Resolve(filepath.Join(root, filepath.FromSlash(from)), imp)
	switch len(got) {
	case 0:
		return ""
	case 1:
		rel, err := filepath.Rel(root, got[0])
		if err != nil {
			t.Fatal(err)
		}
		return filepath.ToSlash(rel)
	default:
		t.Fatalf("expected at most one target, got %v", got)
		return ""
	}
}

func TestRustResolver_ModDecl(t *testing.T) {
	root := rustCrate(t)
	cases := []struct {
		from string
		imp  analyzer.RawImport
		want string
	}{
		{"src/lib.rs", analyzer.RawImport{Specifier: "auth", ModDecl: true}, "src/auth.rs"},
		{"src/lib.rs", analyzer.RawImport{Specifier: "db", ModDecl: true}, "src/db/mod.rs"},
		{"src/auth.rs", analyzer.RawImport{Specifier: "token", ModDecl: true}, "src/auth/token.rs"},
		{"src/db/mod.rs", analyzer.RawImport{Specifier: "pool", ModDecl: true}, "src/db/pool.rs"},
		{"src/bin/tool/main.rs", analyzer.RawImport{Specifier: "args", ModDecl: true}, "src/bin/tool/args.rs"},
		{"src/lib.rs", analyzer.RawImport{Specifier: "missing", ModDecl: true}, ""},
		// mod pool; inside an inline `mod db { }` of lib.rs -> src/db/pool.rs
		{"src/lib.rs", analyzer.RawImport{Specifier: "pool", ModDecl: true, Scope: []string{"db"}}, "src/db/pool.rs"},
	}
	for _, c := range cases {
		if got := resolveRust(t, root, c.from, c.imp); got != c.want {
			t.Errorf("%s: mod %s (scope %v) = %q, want %q", c.from, c.imp.Specifier, c.imp.Scope, got, c.want)
		}
	}
}

func TestRustResolver_UsePaths(t *testing.T) {
	root := rustCrate(t)
	cases := []struct {
		from  string
		spec  string
		scope []string
		want  string
	}{
		// crate:: picks the longest prefix that is a module file.
		{"src/db/pool.rs", "crate::auth::token::Token", nil, "src/auth/token.rs"},
		{"src/db/pool.rs", "crate::auth::Session", nil, "src/auth.rs"},
		{"src/auth.rs", "crate::Config", nil, "src/lib.rs"},
		// self:: and super:: are relative to the current module.
		{"src/auth.rs", "self::token::Token", nil, "src/auth/token.rs"},
		{"src/db/pool.rs", "super::Db", nil, "src/db/mod.rs"},
		{"src/auth/token.rs", "super::super::db::pool", nil, "src/db/pool.rs"},
		{"src/lib.rs", "super::x", nil, ""},
		// super inside an inline `mod tests { }` is the file itself: no edge.
		{"src/auth.rs", "super", []string{"tests"}, ""},
		// A bare path resolves to a child module, else stays external.
		{"src/lib.rs", "auth::token", nil, "src/auth/token.rs"},
		{"src/lib.rs", "serde::Deserialize", nil, ""},
		{"src/lib.rs", "std::collections::HashMap", nil, ""},
		{"src/lib.rs", "::auth::token", nil, ""},
		// The crate's own library, by its name, from main.rs and tests/.
		{"src/main.rs", "my_app::db::pool::Pool", nil, "src/db/pool.rs"},
		{"tests/api.rs", "my_app::auth", nil, "src/auth.rs"},
		// crate:: in a binary target is rooted at that binary.
		{"src/bin/tool/args.rs", "crate::Cli", nil, "src/bin/tool/main.rs"},
	}
	for _, c := range cases {
		imp := analyzer.RawImport{Specifier: c.spec, Scope: c.scope}
		if got := resolveRust(t, root, c.from, imp); got != c.want {
			t.Errorf("%s: use %s (scope %v) = %q, want %q", c.from, c.spec, c.scope, got, c.want)
		}
	}
}

func TestRustResolver_LibNameFromCargoToml(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "Cargo.toml"),
		"[package]\nname = \"pkg-name\"\n\n[lib]\nname = \"core_lib\"\npath = \"src/core.rs\"\n\n[[bin]]\nname = \"tool\"\npath = \"src/main.rs\"\n")
	mustWriteFile(t, filepath.Join(root, "src", "core.rs"), "")
	mustWriteFile(t, filepath.Join(root, "src", "net.rs"), "")
	mustWriteFile(t, filepath.Join(root, "src", "main.rs"), "")

	if got := resolveRust(t, root, "src/main.rs", analyzer.RawImport{Specifier: "core_lib::net::Socket"}); got != "src/net.rs" {
		t.Errorf("use core_lib::net::Socket = %q, want src/net.rs", got)
	}
	if got := resolveRust(t, root, "src/main.rs", analyzer.RawImport{Specifier: "pkg_name::net"}); got != "" {
		t.Errorf("use pkg_name::net = %q, want unresolved ([lib] name overrides the package name)", got)
	}
}

func TestRustResolver_NoCargoToml(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "lib.rs"), "")
	mustWriteFile(t, filepath.Join(root, "util.rs"), "")

	// mod declarations still resolve without a manifest...
	if got := resolveRust(t, root, "lib.rs", analyzer.RawImport{Specifier: "util", ModDecl: true}); got != "util.rs" {
		t.Errorf("mod util = %q, want util.rs", got)
	}
	// ...but crate:: has no known root.
	if got := resolveRust(t, root, "util.rs", analyzer.RawImport{Specifier: "crate::x"}); got != "" {
		t.Errorf("use crate::x = %q, want unresolved", got)
	}
}

func TestRustResolver_BinaryOwnsItsModules(t *testing.T) {
	root := rustCrate(t)
	// main.rs declares `mod cli;`, lib.rs does not: src/cli.rs and its
	// children belong to the binary, so crate:: there means main.rs.
	mustWriteFile(t, filepath.Join(root, "src", "main.rs"), "mod cli;\nfn main() {}\n")
	mustWriteFile(t, filepath.Join(root, "src", "lib.rs"), "pub mod auth;\npub mod db;\n")
	mustWriteFile(t, filepath.Join(root, "src", "cli.rs"), "")
	mustWriteFile(t, filepath.Join(root, "src", "cli", "args.rs"), "")

	cases := []struct{ from, spec, want string }{
		{"src/cli.rs", "crate::Config", "src/main.rs"},
		{"src/cli/args.rs", "crate::Config", "src/main.rs"},
		{"src/auth.rs", "crate::Config", "src/lib.rs"},
	}
	for _, c := range cases {
		if got := resolveRust(t, root, c.from, analyzer.RawImport{Specifier: c.spec}); got != c.want {
			t.Errorf("%s: use %s = %q, want %q", c.from, c.spec, got, c.want)
		}
	}
}

func TestRustResolver_SharedTestHelperHasNoPhantomRoot(t *testing.T) {
	root := rustCrate(t)
	mustWriteFile(t, filepath.Join(root, "tests", "common", "mod.rs"), "")

	// The helper is shared by several test crates: crate:: is ambiguous
	// and must not resolve to a nonexistent tests/common/main.rs.
	if got := resolveRust(t, root, "tests/common/mod.rs", analyzer.RawImport{Specifier: "crate::Fixture"}); got != "" {
		t.Errorf("use crate::Fixture from tests/common/mod.rs = %q, want unresolved", got)
	}
	if got := resolveRust(t, root, "tests/api.rs", analyzer.RawImport{Specifier: "common", ModDecl: true}); got != "tests/common/mod.rs" {
		t.Errorf("mod common from tests/api.rs = %q, want tests/common/mod.rs", got)
	}
}

func TestRustResolver_CustomLibPathDirectory(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "Cargo.toml"), "[package]\nname = 'demo'\n\n[lib]\npath = 'src/custom/root.rs'\n")
	for _, f := range []string{"src/custom/root.rs", "src/custom/util.rs", "src/custom/worker.rs", "src/util.rs"} {
		mustWriteFile(t, filepath.Join(root, filepath.FromSlash(f)), "")
	}

	if got := resolveRust(t, root, "src/custom/worker.rs", analyzer.RawImport{Specifier: "crate::util::T"}); got != "src/custom/util.rs" {
		t.Errorf("use crate::util::T = %q, want src/custom/util.rs", got)
	}
}

func TestRustResolver_InlineParentModule(t *testing.T) {
	root := rustCrate(t)
	// lib.rs: mod outer { pub struct T; mod child; } -> src/outer/child.rs
	mustWriteFile(t, filepath.Join(root, "src", "outer", "child.rs"), "")

	if got := resolveRust(t, root, "src/outer/child.rs", analyzer.RawImport{Specifier: "super::T"}); got != "src/lib.rs" {
		t.Errorf("use super::T = %q, want src/lib.rs (outer is inline in it)", got)
	}
	// A bare path must not fall back to an ancestor: serde stays external.
	if got := resolveRust(t, root, "src/outer/child.rs", analyzer.RawImport{Specifier: "serde::Deserialize"}); got != "" {
		t.Errorf("use serde::Deserialize = %q, want unresolved", got)
	}
}

func TestRustResolver_ModuleNamedMainIsNotARoot(t *testing.T) {
	root := rustCrate(t)
	mustWriteFile(t, filepath.Join(root, "src", "auth", "main.rs"), "")
	mustWriteFile(t, filepath.Join(root, "src", "auth", "main", "child.rs"), "")

	if got := resolveRust(t, root, "src/auth/main.rs", analyzer.RawImport{Specifier: "child", ModDecl: true}); got != "src/auth/main/child.rs" {
		t.Errorf("mod child in src/auth/main.rs = %q, want src/auth/main/child.rs", got)
	}
}

func TestRustResolver_LocalChildShadowsLibName(t *testing.T) {
	root := rustCrate(t)
	mustWriteFile(t, filepath.Join(root, "src", "auth", "my_app.rs"), "")

	if got := resolveRust(t, root, "src/auth.rs", analyzer.RawImport{Specifier: "my_app::T"}); got != "src/auth/my_app.rs" {
		t.Errorf("use my_app::T from src/auth.rs = %q, want the local child src/auth/my_app.rs", got)
	}
	// Inside the library itself, its own name is not a path root.
	if got := resolveRust(t, root, "src/db/pool.rs", analyzer.RawImport{Specifier: "my_app::auth"}); got != "" {
		t.Errorf("use my_app::auth from inside the library = %q, want unresolved", got)
	}
}
