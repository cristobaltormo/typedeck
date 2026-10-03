## What and why

<!-- What does this change, and why is it needed? Link the issue if there is one. -->

## How it was tested

- [ ] `make check` passes (gofmt, `go vet` for the three systems, race-tested unit tests)
- [ ] `make e2e-linux` passes, or I explain why it does not apply
- [ ] `make selftest` passes if I touched the editor
- [ ] I ran hardware tests (`tests/e2e/`) and say which in this description, or the change does not need them

## Checklist

- [ ] No new dependencies in the Go program
- [ ] Everything that depends on the operating system is behind `internal/platform`
- [ ] User-facing text exists in English and Spanish (`web/ui/js/i18n.js`)
- [ ] `CHANGELOG.md` has a line under Unreleased
