# Demo assets

This directory records the animated demo (`demo.gif`) shown in the top-level
README. It runs the **real** TUI against a **mock** `jira` backend, so the
recording is deterministic and needs no live Jira instance, network access, or
credentials.

```
demo/
├── bin/jira      # mock jira CLI — returns canned JSON (not a real client!)
├── bin/xclip     # mock clipboard tool — swallows what the copy picker writes
├── mock-jira-api.py  # mock Jira REST API — transitions + web links
├── config.yaml   # demo config, points backend.cli at the mock
├── demo.tape     # VHS script that drives and records the TUI
└── demo.gif      # generated output (committed so the README renders)
```

## Regenerating `demo.gif`

Prerequisites:

- [`vhs`](https://github.com/charmbracelet/vhs) v0.8+ (and its dependencies,
  `ttyd` and `ffmpeg`)
- Go, to build the TUI
- Python 3, for the mock REST API

From the repository root:

```sh
vhs demo/demo.tape
```

You don't need the app installed: the tape builds it from the checkout into a
temporary directory first, so the recording always reflects the current source.
It prepends that directory and `demo/bin` to `PATH` so the mock `jira` shadows
any real one for the duration of the recording, starts the mock REST API on
`localhost:8642`, launches `jira-tabbed-tui --config demo/config.yaml`, then:

1. browses the board and switches tabs;
2. copies an issue URL with the copy picker (`Y` then `u`);
3. opens the issue and walks its body tabs to **Links**;
4. shows the grouped links (Jira issues with their type, pull requests with
   their state, web links), then follows links three issues deep and steps back
   (`Ctrl+o`) and forward (`Ctrl+i`) — the breadcrumb tracks the depth and the
   same link is highlighted each time;
5. changes the status of the highlighted linked issue right from the Links tab
   (`m`), without opening it;
6. copies the issue's key and summary (`Y`, mark both, `Enter`);
7. shows the help overlay, then quits and stops the mock API.

Output is written to `demo/demo.gif`.

## Notes

- `demo/bin/jira` is a stand-in for [`ankitpokhrel/jira-cli`][jira-cli] that
  understands only what the demo exercises (`me`, `issue list --raw`,
  `issue view --raw`, `issue move`). Do **not** add it to your real `PATH`.
- `demo/mock-jira-api.py` serves the REST calls the TUI makes directly
  (an issue's transitions and web links), plus GitHub's pull request API under
  `/github` — the tape sets `GITHUB_API_URL` there so the PR links show their
  state. `demo/config.yaml` points
  `backend.url` at it, which is why copied URLs in the GIF start with
  `http://localhost:8642`.
- Status changes (`issue move`) are remembered in a temporary state file
  (`$JIRA_DEMO_STATE`) shared by both mocks, so the moved issue shows its new
  status afterwards.
- `demo/bin/xclip` stands in for the clipboard tool so the copy steps behave
  the same on any machine (no X display needed). It discards its input.
- To change what the board shows, edit the canned JSON in `demo/bin/jira`.
- To change the choreography (which keys are pressed, timing, theme, size),
  edit `demo/demo.tape`.

[jira-cli]: https://github.com/ankitpokhrel/jira-cli
