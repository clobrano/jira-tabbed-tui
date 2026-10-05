# Demo assets

This directory records the animated demo (`demo.gif`) shown in the top-level
README. It runs the **real** TUI against a **mock** `jira` backend, so the
recording is deterministic and needs no live Jira instance, network access, or
credentials.

```
demo/
├── bin/jira      # mock jira CLI — returns canned JSON (not a real client!)
├── bin/xclip     # mock clipboard tool — swallows what the copy picker writes
├── config.yaml   # demo config, points backend.cli at the mock
├── demo.tape     # VHS script that drives and records the TUI
└── demo.gif      # generated output (committed so the README renders)
```

## Regenerating `demo.gif`

Prerequisites:

- [`vhs`](https://github.com/charmbracelet/vhs) v0.8+ (and its dependencies,
  `ttyd` and `ffmpeg`)
- Go, to build the TUI

From the repository root:

```sh
vhs demo/demo.tape
```

You don't need the app installed: the tape builds it from the checkout into a
temporary directory first, so the recording always reflects the current source.
It prepends that directory and `demo/bin` to `PATH` so the mock `jira` shadows
any real one for the duration of the recording, launches
`jira-tabbed-tui --config demo/config.yaml`,
then browses the board, switches tabs, copies an issue URL with the copy picker
(`Y` then `u`), opens an issue, copies its key and summary (`Y`, mark both,
`Enter`), and shows the help overlay. Output is written to `demo/demo.gif`.

## Notes

- `demo/bin/jira` is a stand-in for [`ankitpokhrel/jira-cli`][jira-cli] that
  understands only what the demo exercises (`me`, `issue list --raw`,
  `issue view --raw`). Do **not** add it to your real `PATH`.
- `demo/bin/xclip` stands in for the clipboard tool so the copy steps behave
  the same on any machine (no X display needed). It discards its input.
- To change what the board shows, edit the canned JSON in `demo/bin/jira`.
- To change the choreography (which keys are pressed, timing, theme, size),
  edit `demo/demo.tape`.

[jira-cli]: https://github.com/ankitpokhrel/jira-cli
