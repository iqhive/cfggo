Output hotreload.gif

Set Shell        "bash"
Set FontSize     22
Set Width        1200
Set Height       600
Set Padding      24
Set Margin       24
Set MarginFill   "#674EFF"
Set BorderRadius 12
Set WindowBar     Colorful
Set Theme        "Catppuccin Mocha"

Type "cd examples/demo" Sleep 400ms Enter
Sleep 500ms
Type "go build -o .demo-app ." Sleep 500ms Enter
Sleep 1s
Type "./.demo-app &" Sleep 500ms Enter
Sleep 2.5s

Type "cat > config.json <<'EOF'" Sleep 600ms Enter
Type "{" Enter
Type '  "port": 3000,' Enter
Type '  "log_level": "debug"' Enter
Type "}" Enter
Type "EOF" Enter
Sleep 3s

Type "cat > config.json <<'EOF'" Sleep 600ms Enter
Type "{" Enter
Type '  "port": 8443,' Enter
Type '  "log_level": "warn"' Enter
Type "}" Enter
Type "EOF" Enter
Sleep 3s

Hide
Type "kill %1" Enter
Type "rm -f .demo-app" Enter
Type "cp config.initial.json config.json" Enter
