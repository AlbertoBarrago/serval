package rust_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/AlbertoBarrago/serval/internal/analyzer"
	"github.com/AlbertoBarrago/serval/internal/analyzer/rust"
)

func extract(t *testing.T, src string) []analyzer.RawImport {
	t.Helper()
	imports, err := rust.New().ExtractImports("lib.rs", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return imports
}

// key renders an import as "mod:" or "use:", its scope, then specifier.
func key(imp analyzer.RawImport) string {
	kind := "use:"
	if imp.ModDecl {
		kind = "mod:"
	}
	return kind + strings.Join(imp.Scope, "/") + "|" + imp.Specifier
}

func keys(imports []analyzer.RawImport) []string {
	var out []string
	for _, imp := range imports {
		out = append(out, key(imp))
	}
	return out
}

func TestExtractImports_UseTrees(t *testing.T) {
	got := keys(extract(t, `
use std::collections::HashMap;
pub use crate::auth::Token;
pub(crate) use super::config as cfg;
use crate::{
    db::{self, Pool},
    api::*,
    util::fmt as _,
};
use ::serde::Deserialize;
#[cfg(test)] use self::helpers::fixture;
`))
	want := []string{
		"use:|std::collections::HashMap",
		"use:|crate::auth::Token",
		"use:|super::config",
		"use:|crate::db",
		"use:|crate::db::Pool",
		"use:|crate::api",
		"use:|crate::util::fmt",
		"use:|::serde::Deserialize",
		"use:|self::helpers::fixture",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestExtractImports_ModDeclsAndScope(t *testing.T) {
	got := keys(extract(t, `
mod auth;
pub mod db;
#[cfg(test)]
mod integration;

mod inline {
    use super::auth::Token;
    mod nested;
}

fn handler() {
    let s = format!("{} mod fake; use fake::x;", 1);
    let c = '{';
    let q = '"';
}

#[cfg(test)]
mod tests { use super::*; }
`))
	want := []string{
		"mod:|auth",
		"mod:|db",
		"mod:|integration",
		"use:inline|super::auth::Token",
		"mod:inline|nested",
		"use:tests|super",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestExtractImports_IgnoresCommentsAndLifetimes(t *testing.T) {
	got := keys(extract(t, `
// use commented::out;
/* mod blocked; */
fn longest<'a>(x: &'a str, y: &'a str) -> &'a str { x }
use crate::after::lifetimes;
`))
	want := []string{"use:|crate::after::lifetimes"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCanHandle(t *testing.T) {
	a := rust.New()
	if !a.CanHandle("src/main.rs") {
		t.Error("expected CanHandle(src/main.rs) to be true")
	}
	if a.CanHandle("main.go") {
		t.Error("expected CanHandle(main.go) to be false")
	}
}

func TestExtractImports_CompactAndRawForms(t *testing.T) {
	got := keys(extract(t, `
use{crate::a::X};
use::dep::Y;
use crate::r#type::Z;
use crate::b as r#async;
use ::{dep::W};
mod r#use { use crate::c::V; }
mod café { use super::super::d::U; }
`))
	want := []string{
		"use:|crate::a::X",
		"use:|::dep::Y",
		"use:|crate::type::Z",
		"use:|crate::b",
		"use:|::dep::W",
		"use:use|crate::c::V",
		"use:café|super::super::d::U",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}
