## What this changes

<!-- One paragraph: what the change does and why it is the right shape. -->

## Why

<!-- The problem it solves. Link the issue if there is one: Fixes #123 -->

## How it was checked

<!-- Which tests cover it, and anything you ran by hand. -->

- [ ] `go test ./...` passes in every module the change touches
- [ ] `gofmt -l .` is empty and `go vet ./...` is clean
- [ ] The core module still depends on nothing but the standard library
- [ ] Documentation figures regenerated if a render changed
      (`go run ./backend/gg/cmd/gallery`)
- [ ] A behaviour change is covered by a test that fails without it
- [ ] An architectural decision is recorded in `docs/adr/` if the change makes one
