#!/bin/bash
# 맥 빌드 서버에서 돌아요: 아이폰 앱을 빌드(시뮬레이터 · 실제 아이폰용 둘 다, 서명 없이) → 시뮬레이터에서 시험 모드로 켜서 화면을 찍음
# → 결과를 ci-results-ios 브랜치에 올림 (실패해도 기록은 올려요).
# 실제 아이폰 설치는 사용자 맥의 Xcode 에서 무료 애플 ID 로 해요 (docs/ValoROAD_아이폰_설치.png).
set -uo pipefail
ROOT="$(pwd)"
OUT="$ROOT/ci-out-ios"; rm -rf "$OUT"; mkdir -p "$OUT/shots"
VERSION="$(tr -d ' \n' < ios/VERSION)"
PROJ="ios/ValoROAD.xcodeproj"
BUNDLE="com.neverthelessd.valoroad.iphone"
DD="$ROOT/build/dd"

{
  echo "commit: ${GITHUB_SHA:-local}  ios version: $VERSION  started: $(date -u +%FT%TZ)"
  sw_vers; uname -m; xcodebuild -version
} > "$OUT/info.txt" 2>&1

SIM=fail; DEVICE=fail; SHOTS=skip
if xcodebuild -project "$PROJ" -scheme ValoROAD -configuration Debug -sdk iphonesimulator \
     -destination "generic/platform=iOS Simulator" -derivedDataPath "$DD" CODE_SIGNING_ALLOWED=NO build \
     > "$OUT/build-sim.log" 2>&1; then
  SIM=ok
fi
grep -E "error:|warning: .*(deprecated|never|unused)" "$OUT/build-sim.log" | sort -u | head -80 > "$OUT/build-sim-errors.txt"

if xcodebuild -project "$PROJ" -scheme ValoROAD -configuration Release -sdk iphoneos \
     -destination "generic/platform=iOS" -derivedDataPath "$DD-dev" CODE_SIGNING_ALLOWED=NO build \
     > "$OUT/build-device.log" 2>&1; then
  DEVICE=ok
fi
grep -E "error:" "$OUT/build-device.log" | sort -u | head -60 > "$OUT/build-device-errors.txt"

if [ "$SIM" = ok ]; then
  SHOTS=fail
  APP="$(find "$DD/Build/Products" -maxdepth 2 -name 'ValoROAD.app' -path '*iphonesimulator*' | head -1)"
  echo "app: $APP" >> "$OUT/info.txt"
  /usr/libexec/PlistBuddy -c Print "$APP/Info.plist" > "$OUT/Info.plist.txt" 2>&1
  ls -la "$APP" > "$OUT/app-files.txt" 2>&1
  UDID="$(xcrun simctl list devices available -j | python3 -c '
import json,sys
d=json.load(sys.stdin)["devices"]
best=None
for rt,devs in d.items():
    if "iOS" not in rt: continue
    for x in devs:
        n=x["name"]
        if not n.startswith("iPhone"): continue
        score=(rt, ("Pro" in n and "Max" not in n), n)
        if best is None or score>best[0]: best=(score,x["udid"],n,rt)
print(best[1] if best else "")
print(best[2]+" / "+best[3] if best else "", file=sys.stderr)
' 2>>"$OUT/info.txt")"
  if [ -n "$UDID" ]; then
    xcrun simctl boot "$UDID" 2>/dev/null
    xcrun simctl bootstatus "$UDID" -b > /dev/null 2>&1
    xcrun simctl status_bar "$UDID" override --time "9:41" --batteryState charged --batteryLevel 100 --cellularBars 4 --wifiBars 3 2>/dev/null
    xcrun simctl install "$UDID" "$APP" >> "$OUT/info.txt" 2>&1
    shot() { # 이름 · 기다릴 초 · 실행 인자…
      local name="$1" wait="$2"; shift 2
      xcrun simctl terminate "$UDID" "$BUNDLE" > /dev/null 2>&1
      xcrun simctl ui "$UDID" appearance dark > /dev/null 2>&1
      xcrun simctl launch "$UDID" "$BUNDLE" -demo "$@" >> "$OUT/launch.txt" 2>&1
      sleep "$wait"
      xcrun simctl io "$UDID" screenshot "$OUT/shots/$name.png" > /dev/null 2>&1 && echo "PASS  화면 $name" >> "$OUT/results.txt" || echo "FAIL  화면 $name" >> "$OUT/results.txt"
    }
    : > "$OUT/results.txt"
    shot 01-store-dark 25 -tab store -theme dark -active 1
    shot 02-store-hit 10 -tab store -theme dark -active 1
    shot 03-store-light 10 -tab store -theme light -active 2
    shot 04-store-bundle 12 -tab store -theme dark -active 2 -scroll bundle
    shot 05-store-night 12 -tab store -theme dark -active 2 -scroll night
    shot 06-store-expired 10 -tab store -theme dark -active 3
    shot 07-preview 10 -tab store -theme dark -active 2 -preview
    sleep 6; xcrun simctl io "$UDID" screenshot "$OUT/shots/07b-preview-later.png" > /dev/null 2>&1
    xcrun simctl spawn "$UDID" log show --last 25s --style compact --predicate 'process == "ValoROAD" AND (subsystem BEGINSWITH "com.apple.SwiftUI" OR subsystem BEGINSWITH "com.apple.UIKit" OR messageType >= 16)' 2>/dev/null | tail -200 > "$OUT/preview-log.txt"
    shot 08-wishlist 10 -tab wishlist -theme dark
    shot 09-settings 10 -tab settings -theme dark
    shot 10-login 16 -tab settings -theme dark -login
    # 시험 모드가 아니면(실제 첫 실행) 빈 상점 · 계정 추가 안내가 나와야 해요
    xcrun simctl terminate "$UDID" "$BUNDLE" > /dev/null 2>&1
    xcrun simctl privacy "$UDID" grant notifications "$BUNDLE" > /dev/null 2>&1
    xcrun simctl launch "$UDID" "$BUNDLE" >> "$OUT/launch.txt" 2>&1
    sleep 12
    xcrun simctl io "$UDID" screenshot "$OUT/shots/11-first-run.png" > /dev/null 2>&1
    DATA="$(xcrun simctl get_app_container "$UDID" "$BUNDLE" data 2>/dev/null)"
    cp "$DATA/Library/Application Support/ValoROAD/log.txt" "$OUT/app-log.txt" 2>/dev/null
    ls -la "$DATA/Library/Application Support/ValoROAD" > "$OUT/app-support.txt" 2>&1
    xcrun simctl spawn "$UDID" log show --last 6m --style compact --predicate 'process == "ValoROAD"' 2>/dev/null | tail -300 > "$OUT/sim-log.txt"
    N=$(ls "$OUT/shots" | wc -l | tr -d ' ')
    [ "$N" -ge 11 ] && SHOTS=ok
  else
    echo "아이폰 시뮬레이터를 찾지 못했어요" >> "$OUT/info.txt"
  fi
fi

printf '{"version":"%s","commit":"%s","sim":"%s","device":"%s","shots":"%s"}\n' "$VERSION" "${GITHUB_SHA:-local}" "$SIM" "$DEVICE" "$SHOTS" > "$OUT/status.json"
cat "$OUT/status.json"; cat "$OUT/build-sim-errors.txt"; tail -30 "$OUT/build-sim.log"

if [ -n "${GITHUB_TOKEN:-}" ]; then
  cd "$OUT"
  git init -q && git checkout -q -b ci-results-ios
  git add -A
  git -c user.name="ValoROAD build" -c user.email="NeverthelessD@users.noreply.github.com" commit -qm "ios v$VERSION sim=$SIM device=$DEVICE shots=$SHOTS (${GITHUB_SHA:-local})"
  git push -qf "https://x-access-token:${GITHUB_TOKEN}@github.com/${GITHUB_REPOSITORY}.git" ci-results-ios
fi
[ "$SIM" = ok ] && [ "$DEVICE" = ok ]
