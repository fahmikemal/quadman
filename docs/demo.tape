# quadman demo: list, filter, logs, problems, updates, actions.
# Render with VHS (https://github.com/charmbracelet/vhs):
#   vhs docs/demo.tape
# The tape expects the demo units from e2e/README.md
# (~/.config/containers/systemd/demo-web.container and friends).

Output docs/demo.gif
Require quadman

Set FontSize 14
Set Width 1200
Set Height 700
Set Theme "Catppuccin Mocha"

Type "quadman"
Enter
Wait 3s

# filter to the demo unit, then open its file
Type "/web"
Wait 1s
Enter
Wait 500ms
Enter
Wait 2s
Type "q"
Wait 500ms

# live journal with search
Type "l"
Wait 2s
Type "/"
Wait 300ms
Type "demo"
Wait 1s
Type Escape
Wait 500ms
Type "q"
Wait 500ms

# problems view and dependency tree
Type "v"
Wait 2s
Type Escape
Wait 500ms
Type "t"
Wait 2s
Type Escape
Wait 500ms

# auto-update screen and recent actions
Type "u"
Wait 2s
Type "q"
Wait 500ms
Type "A"
Wait 2s
Type "q"
Wait 500ms

Type "q"
Wait 500ms
