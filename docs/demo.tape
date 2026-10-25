# Records docs/demo.gif with VHS (https://github.com/charmbracelet/vhs).
#
#   go build -o /tmp/yurebin/yure . && vhs docs/demo.tape

Output docs/demo.gif

Set FontSize 18
Set Width 1200
Set Height 760
Set Padding 24
Set Theme "Catppuccin Mocha"
Set TypingSpeed 80ms

Hide
Type "export PATH=/tmp/yurebin:$PATH && clear"
Enter
Show

Type "yure --demo"
Sleep 600ms
Enter

# Reports arrive every 4 seconds, each with a ripple. A new report takes the
# selection, so the key presses below fit between two arrivals.
Sleep 16.5s

# Pick an older quake and replay its ripple.
Down
Sleep 600ms
Enter
Sleep 3s

Type "q"
Sleep 500ms
