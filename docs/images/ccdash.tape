# Regenerate the README screenshot from synthetic data:
#   vhs docs/images/ccdash.tape
# scripts/demo-env.sh builds a made-up home directory and agent sessions on a
# private tmux server, so no real session, workspace or cost is recorded.
# Needs JetBrains Mono and an emoji font (e.g. Noto Color Emoji) visible to
# fontconfig, or the status icons render as empty boxes.
Set Shell bash
Set FontFamily "JetBrains Mono"
Set FontSize 14
Set Width 1640
Set Height 430
Set Padding 12
Set Theme "Catppuccin Mocha"

Hide
Type "make build VERSION=$(cat VERSION) >/dev/null && DEMO=$(mktemp -d) && scripts/demo-env.sh $DEMO >/dev/null && clear && env -u TMUX HOME=$DEMO/home TMUX_TMPDIR=/tmp/ccdash-demo-$(id -u) PATH=$DEMO/bin:$PATH ./bin/ccdash"
Enter
Sleep 30s
Show
Sleep 2s
Screenshot docs/images/ccdash.png
Hide
Type "q"
Sleep 1s
Type "scripts/demo-env.sh $DEMO stop; rm -rf $DEMO"
Enter
Sleep 1s
