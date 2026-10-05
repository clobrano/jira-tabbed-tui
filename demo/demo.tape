# VHS tape for the jira-tabbed-tui demo GIF.
# Render with:  vhs demo/demo.tape   (run from the repo root)
# See demo/README.md for prerequisites.
#
# https://github.com/charmbracelet/vhs

Output demo/demo.gif

Require go
Require python3

Set Shell bash
Set FontSize 14
Set Width 1200
Set Height 720
Set Padding 16
Set Theme "Catppuccin Mocha"
Set TypingSpeed 60ms

# --- setup (hidden) ----------------------------------------------------------
# Build the TUI from this checkout, start the mock REST API, point everything
# at the mocks, launch, and wait until the board is on screen. Building here
# means the recording always shows the current source, whatever name (or
# version) of the binary you may have installed.
Hide
Type "export DEMO_BIN=$(mktemp -d) && go build -o $DEMO_BIN/jira-tabbed-tui ."
Enter
Type "export PATH=$DEMO_BIN:$PWD/demo/bin:$PATH JIRA_API_TOKEN=demo-token JIRA_DEMO_STATE=$(mktemp)"
Enter
Type "timeout 600 python3 demo/mock-jira-api.py >/dev/null 2>&1 & MOCK_PID=$!"
Enter
Type "clear && jira-tabbed-tui --config demo/config.yaml"
Enter
Wait+Screen@3m /PLATFORM-/
Sleep 1s
Show

# --- browse the Assigned board -----------------------------------------------
Sleep 1s
Type "j"
Sleep 500ms
Type "j"
Sleep 500ms
Type "j"
Sleep 800ms

# --- switch across tabs ------------------------------------------------------
Tab
Sleep 1.2s
Tab
Sleep 1.2s
Type "1"
Sleep 1s
Type "kkk"
Sleep 800ms

# --- copy picker: grab the highlighted ticket's URL ----------------------------
Type "Y"
Sleep 2.5s
Type "u"
Sleep 1.5s

# --- open an issue and read the detail ---------------------------------------
Enter
Sleep 1.5s
Right
Sleep 1.5s
Right
Sleep 1.5s

# --- Links tab: walk the links, open one, go back and forth ------------------
Down
Sleep 800ms
Down
Sleep 800ms
Up
Sleep 1s
Enter
Sleep 2s
Backspace
Sleep 2s
Ctrl+I
Sleep 2s
Backspace
Sleep 2s

# --- change the status of the highlighted linked issue, without opening it ---
Type "m"
Sleep 2s
Down
Sleep 800ms
Enter
Sleep 2.5s

# --- copy picker in the detail view: mark key + summary, copy both ------------
Left
Sleep 1s
Left
Sleep 1s
Type "Y"
Sleep 2s
Space
Sleep 600ms
Down
Sleep 600ms
Space
Sleep 1.2s
Enter
Sleep 1.8s

# --- back to the list, then the help overlay ---------------------------------
Escape
Sleep 1s
Type "?"
Sleep 2.5s
Type "?"
Sleep 1s

# --- teardown (hidden): quit and stop the mock REST API ------------------------
Hide
Type "q"
Sleep 500ms
Type "kill $MOCK_PID"
Enter
