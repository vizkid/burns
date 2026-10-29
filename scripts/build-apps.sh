#!/bin/sh
# Builds Burns for Mac and installs it in /Applications, or builds Burns for iOS and installs it
# on a paired iPhone (docs/SPEC.md, "Apps and widgets").
#
#   scripts/build-apps.sh mac
#   scripts/build-apps.sh ios [device name or identifier]
#
# Signing comes from ~/.config/burns/signing.env (BURNS_TEAM, BURNS_ID_DOMAIN), which is not
# committed. Xcode must be signed in to that team: the build provisions automatically.
set -eu
cd "$(dirname "$0")/../apps"
env="${XDG_CONFIG_HOME:-$HOME/.config}/burns/signing.env"
[ -f "$env" ] || { echo "missing $env: set BURNS_TEAM and BURNS_ID_DOMAIN there" >&2; exit 1; }
. "$env"
: "${BURNS_TEAM:?BURNS_TEAM is not set in $env}" "${BURNS_ID_DOMAIN:?BURNS_ID_DOMAIN is not set in $env}"
if [ -z "${DEVELOPER_DIR:-}" ] && [ -d /Applications/Xcode.app/Contents/Developer ]; then
	export DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer
fi

xcodegen generate --quiet
build="$PWD/.build-apps"
sign="BURNS_TEAM=$BURNS_TEAM BURNS_ID_DOMAIN=$BURNS_ID_DOMAIN"

case "${1:-}" in
mac)
	# shellcheck disable=SC2086
	xcodebuild -project Burns.xcodeproj -scheme BurnsMac -configuration Release -destination 'generic/platform=macOS' \
		-derivedDataPath "$build" -allowProvisioningUpdates -quiet $sign build
	app="$build/Build/Products/Release/Burns.app"
	pkill -x Burns 2>/dev/null || true
	rm -rf /Applications/Burns.app
	cp -R "$app" /Applications/Burns.app
	open /Applications/Burns.app
	echo "installed /Applications/Burns.app"
	;;
ios)
	device="${2:-}"
	if [ -z "$device" ]; then
		# The identifier is the field before "(UDID)": model names have spaces, so no fixed
		# column holds it. A phone still pairing has no "physical" tag yet, so skip simulators.
		device=$(xcrun devicectl list devices 2>/dev/null | awk '/iPhone/ && !/simulated/ && !/unavailable/ {
			for (i = 2; i <= NF; i++) if ($i == "(UDID)") { print $(i-1); exit } }')
	fi
	[ -n "$device" ] || { echo "no available iPhone: connect and unlock it, or name it" >&2; exit 1; }
	# shellcheck disable=SC2086
	xcodebuild -project Burns.xcodeproj -scheme Burns -configuration Release -destination 'generic/platform=iOS' \
		-derivedDataPath "$build" -allowProvisioningUpdates -allowProvisioningDeviceRegistration -quiet $sign build
	xcrun devicectl device install app --device "$device" "$build/Build/Products/Release-iphoneos/Burns.app"
	echo "installed Burns on $device"
	;;
*)
	echo "usage: $0 mac | ios [device]" >&2
	exit 2
	;;
esac
