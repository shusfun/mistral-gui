#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
ARCH=${1:?amd64 or arm64 required}
case "$ARCH" in amd64) LABEL=x64;; arm64) LABEL=arm64;; *) exit 2;; esac
VERSION=${VERSION:-0.1.0}
APP="$ROOT/dist/mistral-gui.app"
rm -rf "$ROOT/dist/dmg-root"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources" "$ROOT/dist/dmg-root"
cp "$ROOT/dist/mistral-gui" "$APP/Contents/MacOS/mistral-gui"
chmod +x "$APP/Contents/MacOS/mistral-gui" "$ROOT/dist/mistral-gui-updater"
cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>mistral-gui</string>
<key>CFBundleIdentifier</key><string>com.shusfun.mistral-gui</string>
<key>CFBundleShortVersionString</key><string>$VERSION</string>
<key>CFBundleVersion</key><string>$VERSION</string>
<key>CFBundleExecutable</key><string>mistral-gui</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>LSMinimumSystemVersion</key><string>12.0</string>
<key>LSUIElement</key><true/>
<key>CFBundleLocalizations</key><array><string>en</string><string>zh_CN</string></array>
</dict></plist>
PLIST
if [ -n "${MAC_SIGN_IDENTITY:-}" ]; then
  codesign --force --options runtime --timestamp --sign "$MAC_SIGN_IDENTITY" "$APP"
  codesign --force --options runtime --timestamp --sign "$MAC_SIGN_IDENTITY" "$ROOT/dist/mistral-gui-updater"
  ditto -c -k --keepParent "$APP" "$ROOT/dist/notarize.zip"
  xcrun notarytool submit "$ROOT/dist/notarize.zip" --apple-id "$APPLE_ID" --password "$APPLE_APP_PASSWORD" --team-id "$APPLE_TEAM_ID" --wait
  xcrun stapler staple "$APP"
else
  codesign --force --sign - "$APP"
fi
ditto "$APP" "$ROOT/dist/dmg-root/mistral-gui.app"
ln -s /Applications "$ROOT/dist/dmg-root/Applications"
DMG="$ROOT/dist/mistral-gui-macos-$LABEL.dmg"
hdiutil create -volname "mistral-gui" -srcfolder "$ROOT/dist/dmg-root" -ov -format UDZO "$DMG"
if [ -n "${MAC_SIGN_IDENTITY:-}" ]; then
  codesign --force --timestamp --sign "$MAC_SIGN_IDENTITY" "$DMG"
  xcrun notarytool submit "$DMG" --apple-id "$APPLE_ID" --password "$APPLE_APP_PASSWORD" --team-id "$APPLE_TEAM_ID" --wait
  xcrun stapler staple "$DMG"
fi
