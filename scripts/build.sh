#!/bin/bash
# ValoROAD 빌드 스크립트 — ValoROAD.app 을 만들고
# 업데이트용 zip 과 releases/latest.json 을 씁니다. (Windows 버전은 나중에)
#
#   scripts/build.sh                 # VERSION 파일의 버전으로 빌드
#   scripts/build.sh --notes "고친 점 1" "고친 점 2"
#
# 필요: Go 1.24+, python3 (+ Pillow: 아이콘을 다시 만들 때만). Linux/macOS 어디서든 크로스 빌드됩니다.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="$(tr -d ' \n' < "$ROOT/VERSION")"
NOTES=()
if [ "${1:-}" = "--notes" ]; then shift; NOTES=("$@"); fi

DIST="$ROOT/dist"
APP="$DIST/ValoROAD.app"
rm -rf "$DIST"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources" "$ROOT/releases"

# ── macOS (Apple Silicon + Intel) ─────────────────────────
for pair in "arm64:arm64" "amd64:x86_64"; do
  GA="${pair%%:*}"; MA="${pair##*:}"
  echo "▶ 엔진 빌드 (darwin/$GA, v$VERSION)"
  ( cd "$ROOT/engine" && CGO_ENABLED=0 GOOS=darwin GOARCH="$GA" \
      go build -trimpath -ldflags "-s -w -X main.build=$VERSION" -o "$APP/Contents/Resources/valoroad-$MA" . )
  chmod 755 "$APP/Contents/Resources/valoroad-$MA"
done

cp "$ROOT/app/launcher.sh" "$APP/Contents/MacOS/ValoROAD"
chmod 755 "$APP/Contents/MacOS/ValoROAD"
sed "s/__VERSION__/$VERSION/g" "$ROOT/app/Info.plist" > "$APP/Contents/Info.plist"
cp "$ROOT/app/AppIcon.icns" "$APP/Contents/Resources/AppIcon.icns"
printf '%s' "$VERSION" > "$APP/Contents/Resources/version.txt"
printf 'APPL????' > "$APP/Contents/PkgInfo"

ZIP="ValoROAD-$VERSION.zip"
python3 "$ROOT/scripts/zipapp.py" "$DIST" "ValoROAD.app" "$ROOT/releases/$ZIP"
python3 "$ROOT/scripts/feed.py" "$ROOT/releases/latest.json" "$VERSION" "$ROOT/releases/$ZIP" "${NOTES[@]+"${NOTES[@]}"}"

# 처음 설치용: ValoROAD 폴더 안에 앱 + 사용 가이드
mkdir -p "$DIST/first/ValoROAD"
cp -R "$APP" "$DIST/first/ValoROAD/"
if [ -f "$ROOT/docs/ValoROAD_사용가이드.png" ]; then cp "$ROOT/docs/ValoROAD_사용가이드.png" "$DIST/first/ValoROAD/"; fi
python3 "$ROOT/scripts/zipapp.py" "$DIST/first" "ValoROAD" "$DIST/ValoROAD.zip"
echo "✔ dist/ValoROAD.zip (처음 설치용), releases/$ZIP + latest.json (자동 업데이트용)"
