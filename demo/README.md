# Demo assets

This directory records the animated demo (`demo.gif`) shown in the top-level
README. It runs the **real** TUI against a **mock** `jira` backend, so the
recording is deterministic and needs no live Jira instance, network access, or
credentials.

```
demo/
├── bin/jira      # mock jira CLI — returns canned JSON (not a real client!)
├── config.yaml   # demo config, points backend.cli at the mock
├── demo.tape     # VHS script that drives and records the TUI
└── demo.gif      # generated output (committed so the README renders)
```

## Regenerating `demo.gif`

Prerequisites:

- [`vhs`](https://github.com/charmbracelet/vhs) (and its dependencies, `ttyd`
  and `ffmpeg`)
- The `jira-tui` binary on your `PATH`

From the repository root:

```sh
# 1. Build and install the binary so `vhs` can find it.
go build -o jira-tui .
sudo mv jira-tui /usr/local/bin/      # or anywhere on your PATH

# 2. Record the GIF.
vhs demo/demo.tape
```

The tape prepends `demo/bin` to `PATH` so the mock `jira` shadows any real one
for the duration of the recording, launches `jira-tui --config demo/config.yaml`,
then browses the board, switches tabs, opens an issue, and shows the help
overlay. Output is written to `demo/demo.gif`.

## Notes

- `demo/bin/jira` is a stand-in for [`ankitpokhrel/jira-cli`][jira-cli] that
  understands only what the demo exercises (`me`, `issue list --raw`,
  `issue view --raw`). Do **not** add it to your real `PATH`.
- To change what the board shows, edit the canned JSON in `demo/bin/jira`.
- To change the choreography (which keys are pressed, timing, theme, size),
  edit `demo/demo.tape`.

[jira-cli]: https://github.com/ankitpokhrel/jira-cli
