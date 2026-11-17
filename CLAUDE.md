# Serval: Project Notes

## VCS: plain git, directly on `main`
The working VCS is plain **git**. Work happens **directly on `main`**:
no feature branches, no pull requests. This overrides the global
branch-first rule for this repo.

This repo is worked from more than one machine/session, so always
`git fetch` before starting non-trivial work and before pushing. If
`git push` is rejected because `origin/main` moved, don't force it:
inspect the new remote commit(s) (`git log HEAD..origin/main`), then
`git pull --rebase` and push again. Never `git push --force` on `main`.
Push only on explicit confirmation.
