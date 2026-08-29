Output advanced.gif

Require go

Set Shell "bash"
Set FontSize 20
Set Width 1200
Set Height 720
Set Padding 24
Set Margin 24
Set MarginFill "#674EFF"
Set BorderRadius 12
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set TypingSpeed 35ms

Hide
Type "clear" Enter
Show

Type "# Advanced example: custom loggers and error wrappers" Sleep 700ms Enter
Sleep 900ms
Type "go run ./examples/advanced 2>&1 | sed -E 's|^[0-9]{4}/[^ ]+ [0-9:]+ |LOG |' | sed -n '1,22p'" Sleep 500ms Enter
Sleep 6s
