# Changelog

## 0.1.0 (2026-09-12)


### ⚠ BREAKING CHANGES

* Go source builds now run from `app/`. Use `mise run check` for repository checks and `mise run //app:format` or `mise run //app:build` instead of Makefile targets. Releases are prepared and published through the generated Release Please PR; `release:prepare`,

### Build System

* separate components and publish checked release artifacts ([#42](https://github.com/azohra/gopro-yank/issues/42)) ([f3339cd](https://github.com/azohra/gopro-yank/commit/f3339cd7c575740fa0b77835813858fc9ad39f14))

## Changelog
