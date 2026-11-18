# Review guide: Serval

Check only the rules whose trigger matches a path in the diff. For each finding, name the rule (R1, R2, ...) or say "outside REVIEW.md" when it is a bug no rule covers. Report real defects with a concrete failure scenario; skip anything listed under "Do not flag".

## Rules

### R1: Deterministic output
Trigger: `internal/graph/**`, `internal/output/**`, `internal/impact/**`, `internal/risk/**`, `internal/repository/tsconfig.go`, `cmd/**`
Check: every value that reaches output or drives a choice and comes from a Go map is sorted first (keys, edges, alias candidates). Two runs on the same repository state must print byte-identical results.
Why: map iteration order is random per process, so impact lists and tsconfig alias resolution changed between runs (fixes: 69fd0a9, bbf483a)

### R2: No silent skips
Trigger: `internal/repository/**`, `internal/ci/**`, `internal/git/**`, `cmd/inspect.go`, `cmd/diff.go`
Check: an unreadable file, a failed parse, a failed git call or CI discovery error is reported on stderr as `warning: ...` (or returned), never dropped with `_` or an empty `if err != nil {}`. A partial result must say it is partial.
Why: missing history or CI signals silently scored as zero and understated risk (fixes: 62e8111, bbf483a)

### R3: Git subprocesses go through runGit
Trigger: any new `exec.Command` / `exec.CommandContext` running `git`
Check: the call goes through `internal/git` `runGit` (30s timeout, `WaitDelay`, `core.quotePath=false`), not a bare `exec.Command("git", ...)`.
Why: a credential prompt or stale lockfile froze serval forever, and non-ASCII paths came back C-quoted (fixes: 62e8111)

### R4: Resolvers follow the language, not a guess
Trigger: `internal/repository/*resolve*.go`, `internal/repository/{tsconfig,gomod,cargo}.go`, `internal/analyzer/**`
Check: a resolution rule matches what the compiler/runtime actually does for that language (extension rewriting, test-file exclusion, module file lookup). When a case can't be decided, the import is recorded as external/unresolved; it never resolves to a plausible but wrong file, and never to a path that doesn't exist on disk (`isFile` before returning).
Why: NodeNext `./foo.js` never matched `foo.ts` and emptied impact for whole repos; Go package edges included `_test.go` files (fixes: f38bfe7, bbf483a)

### R5: Specifier and path slicing is bounds-safe
Trigger: `internal/repository/**`, `internal/analyzer/**`, `internal/ci/glob.go`, `internal/risk/criticalpath.go`
Check: every index or slice on a specifier, path segment or regex submatch is guarded for short or empty input (`""`, a single `.`, a bare `*`).
Why: `matchPathPattern` panicked on a short specifier (fixes: bbf483a)

### R6: Whole-segment path matching
Trigger: `internal/risk/criticalpath.go`, `internal/risk/highrisk.go`, `internal/ci/glob.go`, `internal/config/**`
Check: keyword and glob matching compares whole path segments; `**` spans whole segments only. A keyword must not match a substring of a longer segment.
Why: the `auth` critical-path keyword matched `author/` (fixes: bbf483a)

### R7: One shared comment stripper
Trigger: `internal/analyzer/**`, `internal/repository/tsconfig.go`, `internal/ci/**`, `internal/strip/**`
Check: comment removal uses `strip.Comments` with a preset (adding one if needed); no analyzer brings back its own comment-stripping state machine. A change to `strip.Comments` itself is checked against every preset's tests, since all languages share it.
Why: six near-identical local strippers drifted apart before being merged into one lexer (fixes: bbf483a)

### R8: Scanner exclusions cover build and environment output
Trigger: `internal/repository/scanner.go`, any new language analyzer
Check: a newly supported ecosystem's build, dependency and virtualenv directories are in `excludedDirs` (e.g. `target/` for Cargo and Maven), with a test in `scanner_exclude_test.go` or a fixture.
Why: virtualenvs and VCS metadata were scanned as source and polluted the graph (fixes: 62e8111)

### R9: A new language is wired end to end
Trigger: a new `internal/analyzer/<lang>/` package
Check: the diff also contains the `languageSupport` entry in `NewScanner`, a resolver with tests, a `testdata/fixtures/simple-<lang>` scanner test, an explicit `internal/testsignal` decision, the scope sections in `docs/usage.md` and `docs/architecture.md` (resolved / not resolved), the README language list, the `cmd/inspect.go` help line with its regenerated `docs/serval-inspect.1`, and a CHANGELOG entry.
Why: `docs/architecture.md` ("Adding a language") promises one registration point, and every existing language documents its scope limits (source: docs/architecture.md)

### R10: Help text and man pages stay in sync
Trigger: `cmd/**` help strings (`Short`, `Long`, flag usage), `docs/*.1`, `Makefile` `man-check`
Check: a help-text change ships with the regenerated `docs/*.1` content. `man-check` ignores only cobra's date lines (`.TH` date, `Auto generated` history), never real content.
Why: man-check flaked daily, then monthly, on generated dates, and must still catch real drift (fixes: ad07358, 291151e)

### R11: Release packaging ships what the docs promise
Trigger: `.goreleaser.yml`, install instructions in `README.md`, `docs/usage.md`, `TESTING.md`
Check: archives and the Homebrew formula still bundle `docs/*.1`; every install instruction uses the fully qualified tap name.
Why: `man serval` was empty after a Homebrew install, and a bare formula name installed an unrelated homebrew-core package (fixes: 26aa08c, 0c0c962)

### R12: --explain never alters the score and stays opt-in
Trigger: `internal/ai/**`, `cmd/explain.go`, `cmd/inspect.go`, `cmd/diff.go`, `internal/output/markdown.go`
Check: no network call or subprocess without `--explain`; the AI text never feeds back into the risk score; terminal output strips Markdown while `--json` keeps it raw; an explicit `--explain-model` is never swapped by the fallback.
Why: models returned raw Markdown and a missing default model produced an unactionable 404 (fixes: cf56c66, 1eacba3; source: README "can never alter the deterministic score")

### R13: Every risk point is documented
Trigger: `internal/risk/**`
Check: a new or changed weight, rule or threshold is reflected in the weight table in `docs/architecture.md`, and appears as a named line in the risk breakdown.
Why: the score's promise is that every point maps to a named rule (source: docs/usage.md "Risk scoring", docs/architecture.md)

## Do not flag

- Formatting, import order and vet-level issues: CI runs `gofmt -l`, `go vet`, `go test`, `go build` and `make man-check`.
- Limits listed as "Explicitly out of scope" or "does not (yet) resolve" in `docs/architecture.md` and `docs/usage.md`: they are documented approximations by design, not bugs.
- Regex-based import extraction instead of real parsers: a deliberate tradeoff (docs/architecture.md).
- Work committed directly on `main`, without branches or PRs: the project's chosen workflow (CLAUDE.md).
