#!/bin/sh
# Install smithers and smithersd into ~/.local/bin, then run smithersd as a
# launchd login agent (macOS) or a systemd user service (Linux) and restart it.
# Agent hooks are left to the user: smithersd changes no agent configuration
# (docs/SPEC.md, Installing).
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

# A service's PATH may lack rex; an empty --rex leaves bindings unchecked.
rex=$(command -v rex || true)

if [ "$(uname -s)" != Darwin ]; then
	if ! command -v systemctl >/dev/null || ! systemctl --user show-environment >/dev/null 2>&1; then
		echo "no systemd user manager: start $bin/smithersd yourself"
		exit 0
	fi
	unit="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/burns-smithersd.service"
	mkdir -p "$(dirname "$unit")"
	cat >"$unit" <<EOF
[Unit]
Description=Burns smithersd

[Service]
ExecStart="$bin/smithersd" --config "$config" --rex "$rex"
Restart=always
RestartSec=10

[Install]
WantedBy=default.target
EOF
	systemctl --user daemon-reload
	systemctl --user enable burns-smithersd.service >/dev/null
	systemctl --user restart burns-smithersd.service
	echo "started burns-smithersd; log: journalctl --user -u burns-smithersd"
	# Without lingering, the service stops at logout.
	if [ "$(loginctl show-user "$(id -un)" -p Linger --value 2>/dev/null)" != yes ]; then
		echo "to keep it running after logout: loginctl enable-linger $(id -un)"
	fi
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
		<string>--rex</string>
		<string>$rex</string>
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
