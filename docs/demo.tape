# vhs script for the gopro-yank demo GIF.
#
# render with:  vhs docs/demo.tape
# output:       docs/demo.gif

Output docs/demo.gif

Set Shell zsh
Set FontSize 14
Set FontFamily "JetBrains Mono"
Set Width  1280
Set Height 760
Set Padding 24
Set Theme   "Dracula"
Set TypingSpeed 30ms

Hide
Type "PATH=/Users/justin/.local/bin:$PATH"
Enter
Sleep 300ms
Type "clear"
Enter
Show

# 1. The pitch: a single, short command.
Type "gopro-yank demo --count 14 --parallel 6"
Sleep 800ms
Enter

# Let the run play out — 14 fake items at 6-way parallelism.
Sleep 18s

# Hold on the finished screen so the result is readable.
Sleep 2s
