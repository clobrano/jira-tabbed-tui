# VHS tape for the jira-tabbed-tui demo GIF.
# Render with:  vhs demo/demo.tape   (run from the repo root)
# See demo/README.md for prerequisites.
#
# https://github.com/charmbracelet/vhs

Output demo/demo.gif

Require jira-tui

Set Shell bash
Set FontSize 14
Set Width 1200
Set Height 720
Set Padding 16
Set Theme "Catppuccin Mocha"
Set TypingSpeed 60ms

# --- setup (hidden): point at the mock backend, then launch the TUI ----------
Hide
Type "export PATH=$PWD/demo/bin:$PATH"
Enter
Type "export JIRA_API_TOKEN=demo-token"
Enter
Type "clear && jira-tui --config demo/config.yaml"
Enter
Sleep 2s
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

# --- open an issue and read the detail ---------------------------------------
Enter
Sleep 1.5s
Right
Sleep 1.5s
Right
Sleep 1.5s
Left
Sleep 1s

# --- back to the list, then the help overlay ---------------------------------
Escape
Sleep 1s
Type "?"
Sleep 2.5s
Type "?"
Sleep 1s
