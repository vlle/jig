Output docs/demo.gif

Require jig

Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 700
Set Padding 24
Set Framerate 24
Set TypingSpeed 55ms
Set Theme "Catppuccin Mocha"

Hide
Type "bash docs/demo/setup.sh /tmp/jig-demo && export JIG_ROOT=/tmp/jig-demo JIG_LOG=/tmp/jig-demo/.runs.jsonl PS1='$ ' && cd /tmp/jig-demo && clear"
Enter
Sleep 3s
Show

Type "jig"
Sleep 300ms
Enter
Sleep 4.5s

Type "stock"
Sleep 1.6s
Enter
Sleep 300ms
Tab
Sleep 2.6s
Tab
Sleep 700ms
Escape
Sleep 500ms
Escape
Sleep 1.2s
Escape
Sleep 700ms

Type "jig new stock-recount"
Sleep 300ms
Enter
Sleep 3s

Type "clear && jig demo --speed 1.6"
Sleep 300ms
Enter
Sleep 11s
