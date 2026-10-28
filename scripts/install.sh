#!/bin/sh
# Install smithers and smithersd into ~/.local/bin. On macOS, also run
# smithersd as a launchd login agent and restart it. Agent hooks are left to
# the user: smithersd changes no agent configuration (docs/SPEC.md, Installing).
set -eu
cd "$(dirname "$0")/.."

bin="${PREFIX:-$HOME/.local}/bin"
label=com.vizkid.burns.smithersd
config="${XDG_CONFIG_HOME:-$HOME/.config}/burns/accounts.json"

mkdir -p "$bin"
# install(1) writes a new file, so a running smithersd keeps its old binary.
install -m 0755 bin/smithersd "$bin/smithersd"
install -m 0755 bin/smithers "$bin/smithers"
echo "installed $("$bin/smithers" version) into $bin"

if [ ! -f "$config" ]; then
	echo "missing $config: copy docs/accounts.example.json there and edit it" >&2
	exit 1
fi

if [ "$(uname -s)" != Darwin ]; then
	echo "not macOS: start smithersd yourself, for example from a systemd user unit"
	exit 0
fi

logs="$HOME/Library/Logs/burns"
plist="$HOME/Library/LaunchAgents/$label.plist"
mkdir -p "$logs" "$(dirname "$plist")"
cat >"$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>$label</string>
	<key>ProgramArguments</key>
	<array>
		<string>$bin/smithersd</string>
		<string>--config</string>
		<string>$config</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ProcessType</key>
	<string>Background</string>
	<key>ThrottleInterval</key>
	<integer>10</integer>
	<key>StandardOutPath</key>
	<string>$logs/smithersd.log</string>
	<key>StandardErrorPath</key>
	<string>$logs/smithersd.log</string>
</dict>
</plist>
EOF
plutil -lint -s "$plist"

domain="gui/$(id -u)"
launchctl bootout "$domain/$label" 2>/dev/null || true
# bootout returns before the job is gone; bootstrap fails until it is.
tries=0
until launchctl bootstrap "$domain" "$plist" 2>/dev/null; do
	tries=$((tries + 1))
	if [ "$tries" -ge 20 ]; then
		launchctl bootstrap "$domain" "$plist"
	fi
	sleep 0.25
done
echo "started $label; log in $logs/smithersd.log"
