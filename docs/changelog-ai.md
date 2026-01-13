## [HEAD] - 2026-01-13

### Added
- Add ICD-10-CM loader and fix lint issues

### Fixed
- Add ListRecursive to Provider interface and fix S3 path parsing

### Documentation
- Update backlog status for ICD-10-CM and matching engine

### Tests
- Add section mapper TemplateOID tests

### Build
- Improve Makefile with dev-setup and golangci-lint v2

### CI
- Check gofmt only on source directories

### Maintenance
- Enable gosec linter with targeted nolint directives

### Other
- Enable errcheck linter for unchecked error returns

### Style
- Fix struct field alignment in fuzzy_test.go
