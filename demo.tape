Output demo.gif

Require go

Set Shell "bash"
Set FontSize 22
Set Width 1200
Set Height 640
Set Padding 24
Set Margin 24
Set MarginFill "#674EFF"
Set BorderRadius 12
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set TypingSpeed 35ms

Type "cd examples/demo" Sleep 400ms Enter
Sleep 500ms
Type "# cfggo hot reload keeps typed accessors up to date" Sleep 500ms Enter
Sleep 200ms
Type "sed -n '12,35p' main.go" Sleep 300ms Enter
Sleep 4s

Type "clear" Enter
Type "# Start the app with config.json" Sleep 500ms Enter
Sleep 500ms
Type "bash run.sh &" Enter
Sleep 2.5s

Type "# Edit config.json while the process keeps running" Sleep 500ms Enter
Sleep 1s
Type "cat > config.json <<'EOF'" Enter
Sleep 400ms
Type "{" Enter
Type '  "port": 9090,' Enter
Type '  "log_level": "debug"' Enter
Type "}" Enter
Type "EOF" Enter
Sleep 4s

Type "# Typed accessors changed in place. No restart." Sleep 500ms Enter
Sleep 3s

Hide
Type "kill %1" Enter
Type "cp config.initial.json config.json" Enter
