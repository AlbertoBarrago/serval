// Package rust implements the analyzer.Analyzer contract for Rust
// source files.
//
// v0.1 scope (see docs/architecture.md for the full rationale):
//   - out-of-line module declarations (`mod x;`, any visibility, any
//     attributes), reported with ModDecl set; inline `mod x { ... }`
//     blocks are not imports, but they are tracked so every statement
//     carries the inline modules enclosing it (RawImport.Scope)
//   - `use` declarations, with use trees expanded into one specifier
//     per leaf: `use crate::{a::X, b::{self, c}};` yields crate::a::X,
//     crate::b and crate::b::c; `as` renames are dropped, and a glob
//     (`a::*`) yields its module path a
//   - `#[path = "..."]` attributes, macro-generated `mod`/`use`, raw
//     string literals containing an odd number of `"`, and nested block
//     comments are out of scope
package rust

import (
	"regexp"
	"sort"
	"strings"

	"github.com/AlbertoBarrago/serval/internal/analyzer"
	"github.com/AlbertoBarrago/serval/internal/strip"
)

// Matches a char literal: one character, or an escape (\n, \x7f,
// \u{1F600}, an escaped quote). A lifetime ('a) never matches, since it
// has no closing quote right after its first character.
var reCharLiteral = regexp.MustCompile(`'(?:[^'\\\n]|\\(?:u\{[0-9a-fA-F]+\}|x[0-9a-fA-F]{2}|.))'`)

// Matches the use keyword. The tree after it runs up to the next
// semicolon (a use tree never contains one); whitespace after the
// keyword is optional (use{a, b}; and use::dep::X; are valid).
var reUseKeyword = regexp.MustCompile(`\buse\b`)

// ident matches a Rust identifier, Unicode letters included (mod café).
const ident = `[\p{L}\p{N}_]+`

// Matches an out-of-line module declaration, capturing its name.
var reModDecl = regexp.MustCompile(`\bmod\s+(?:r#)?(` + ident + `)\s*;`)

// Matches the opening of an inline module block, capturing its name.
var reModInline = regexp.MustCompile(`\bmod\s+(?:r#)?(` + ident + `)\s*\{`)

// Matches an `as` rename inside a use tree (`a::b as c`, `a as _`).
var reAsRename = regexp.MustCompile(`\s+as\s+` + ident)

// Analyzer is the Rust implementation of analyzer.Analyzer.
type Analyzer struct{}

// New returns a Rust analyzer.
func New() *Analyzer {
	return &Analyzer{}
}

// CanHandle reports whether path is a Rust source file.
func (a *Analyzer) CanHandle(path string) bool {
	return strings.HasSuffix(path, ".rs")
}

// ExtractImports scans content with the same lightweight regex-based
// approach used by the other analyzers (see docs/architecture.md).
// Rust needs one extra step: string contents are blanked too, not just
// comments, because brace counting (for RawImport.Scope) must not see
// the braces of format strings like "{}".
func (a *Analyzer) ExtractImports(path string, content []byte) ([]analyzer.RawImport, error) {
	src := reCharLiteral.ReplaceAllString(string(content), "'_'")
	src = blankStrings(strip.Comments(src, strip.RustQuotes))

	type statement struct {
		offset int
		imp    analyzer.RawImport
	}
	var stmts []statement

	for _, m := range reUseKeyword.FindAllStringIndex(src, -1) {
		if m[0] > 0 && src[m[0]-1] == '#' {
			// r#use is an identifier, not the keyword.
			continue
		}
		end := strings.IndexByte(src[m[1]:], ';')
		if end < 0 {
			continue
		}
		for _, spec := range expandUseTree(src[m[1] : m[1]+end]) {
			stmts = append(stmts, statement{offset: m[0], imp: analyzer.RawImport{Specifier: spec}})
		}
	}
	for _, m := range reModDecl.FindAllStringSubmatchIndex(src, -1) {
		stmts = append(stmts, statement{offset: m[0], imp: analyzer.RawImport{Specifier: src[m[2]:m[3]], ModDecl: true}})
	}
	sort.SliceStable(stmts, func(i, j int) bool { return stmts[i].offset < stmts[j].offset })

	scopes := scopesAt(src, len(stmts), func(i int) int { return stmts[i].offset })

	seen := make(map[string]bool)
	var out []analyzer.RawImport
	for i, s := range stmts {
		s.imp.Scope = scopes[i]
		key := strings.Join(s.imp.Scope, "::") + "|" + s.imp.Specifier
		if s.imp.ModDecl {
			key = "mod:" + key
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s.imp)
	}
	return out, nil
}

// scopesAt walks src once, tracking which braces open an inline module,
// and returns for each of the n offsets (ascending, read through
// offsetAt) the inline modules enclosing it, outermost first.
func scopesAt(src string, n int, offsetAt func(int) int) [][]string {
	modBraces := make(map[int]string)
	for _, m := range reModInline.FindAllStringSubmatchIndex(src, -1) {
		modBraces[m[1]-1] = src[m[2]:m[3]]
	}

	// One entry per open brace: the module name, or "" for any other block.
	var stack []string
	current := func() []string {
		var names []string
		for _, name := range stack {
			if name != "" {
				names = append(names, name)
			}
		}
		return names
	}

	scopes := make([][]string, n)
	next := 0
	for i := 0; i <= len(src) && next < n; i++ {
		for next < n && offsetAt(next) == i {
			scopes[next] = current()
			next++
		}
		if i == len(src) {
			break
		}
		switch src[i] {
		case '{':
			stack = append(stack, modBraces[i])
		case '}':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return scopes
}

// blankStrings replaces the contents of every "..." literal in src
// (comments already removed) with spaces, keeping offsets and newlines
// intact so later matches still line up with the source.
func blankStrings(src string) string {
	b := []byte(src)
	inString, escaped := false, false
	for i, c := range b {
		if !inString {
			if c == '"' {
				inString = true
			}
			continue
		}
		switch {
		case escaped:
			escaped = false
		case c == '\\':
			escaped = true
		case c == '"':
			inString = false
			continue
		}
		if c != '\n' {
			b[i] = ' '
		}
	}
	return string(b)
}

// expandUseTree flattens a use tree into one "::"-joined path per leaf.
// A leading "::" (an explicit extern path, `use ::std::fmt`) is kept as
// an empty first segment so the resolver can tell it apart.
func expandUseTree(tree string) []string {
	// Raw identifiers name the same module as their plain form
	// (crate::r#type is the file type.rs).
	tree = strings.ReplaceAll(tree, "r#", "")
	tree = reAsRename.ReplaceAllString(tree, "")
	tree = strings.Join(strings.Fields(tree), "")
	var out []string
	expandInto(&out, "", tree)
	return out
}

func expandInto(out *[]string, prefix, tree string) {
	for _, item := range splitTopLevel(tree) {
		if item == "" {
			continue
		}
		if open := strings.IndexByte(item, '{'); open >= 0 && strings.HasSuffix(item, "}") {
			inner := item[open+1 : len(item)-1]
			if prefix == "" && item[:open] == "::" {
				// use ::{dep::X}: keep each leaf an explicit extern path.
				var leaves []string
				expandInto(&leaves, "", inner)
				for _, leaf := range leaves {
					*out = append(*out, "::"+leaf)
				}
				continue
			}
			head := strings.TrimSuffix(item[:open], "::")
			expandInto(out, joinPath(prefix, head), inner)
			continue
		}
		switch {
		case item == "self":
			if prefix != "" {
				*out = append(*out, prefix)
			}
		case item == "*":
			if prefix != "" {
				*out = append(*out, prefix)
			}
		default:
			*out = append(*out, strings.TrimSuffix(joinPath(prefix, item), "::*"))
		}
	}
}

func joinPath(prefix, rest string) string {
	switch {
	case prefix == "":
		return rest
	case rest == "":
		return prefix
	default:
		return prefix + "::" + rest
	}
}

// splitTopLevel splits s on commas that are not inside braces.
func splitTopLevel(s string) []string {
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}
