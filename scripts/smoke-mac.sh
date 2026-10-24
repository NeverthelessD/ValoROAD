#!/bin/bash
# ValoROAD 실제 맥 시험 (맥 빌드 서버에서 dist/ValoROAD.app 을 /Applications 에 깔고 여러 상황을 시험해요)
# 라이엇 계정으로 실제 로그인은 할 수 없어서, 상점 화면은 '시험 모드(데모)'로 보고(실제 스킨 정보 · 이미지 사용),
# 라이엇 연결 도우미(--riot-fetch)와 키체인 보관은 따로 시험해요.
# 결과: $OUT/results.txt (PASS/FAIL 한 줄씩), 스크린샷, 기록 파일
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${OUT:-$ROOT/ci-out}"
mkdir -p "$OUT"
APPDIR="/Applications/ValoROAD.app"
SUP="$HOME/Library/Application Support/ValoROAD"
RES="$OUT/results.txt"
: > "$RES"

pass() { echo "PASS  $*" | tee -a "$RES"; }
fail() { echo "FAIL  $*" | tee -a "$RES"; }
check() { local name="$1"; shift; if "$@"; then pass "$name"; else fail "$name"; fi; }

app_pid() { pgrep -x ValoROAD | head -1; }
engines() { pgrep -x valoroad-engine | tr '\n' ' '; }
tok() { cat "$SUP/token" 2>/dev/null; }
port() { cat "$SUP/port" 2>/dev/null; }
api() { curl -fs -m 10 -H "x-token: $(tok)" "http://127.0.0.1:$(port)$1"; }
apipost() { curl -fs -m 10 -X POST -H "x-token: $(tok)" -H 'content-type: application/json' --data "$2" "http://127.0.0.1:$(port)$1"; }
ping_ok() { api /api/ping >/dev/null 2>&1; }
wait_ready() { for _ in $(seq 1 60); do [ -s "$SUP/port" ] && ping_ok && return 0; sleep 0.5; done; return 1; }
wait_gone() { for _ in $(seq 1 30); do [ -z "$(app_pid)" ] && [ -z "$(engines)" ] && return 0; sleep 0.5; done; return 1; }
snap() { echo "--- $1"; ps -axo pid,ppid,stat,comm | grep -E "ValoROAD|valoroad-engine" | grep -v grep; } >> "$OUT/procs.txt"
launch() { open "$APPDIR"; wait_ready; }
js() { python3 -c "import json,sys; d=json.load(sys.stdin); print($1)" 2>/dev/null; }

echo "▶ 준비: 앱 설치 · 시험 모드 · 설정"
rm -rf "$APPDIR"; ditto "$ROOT/dist/ValoROAD.app" "$APPDIR"
rm -rf "$SUP"; mkdir -p "$SUP"
touch "$SUP/demo-mode"
printf '{"appearance":"dark","autoUpdate":false,"wishAlert":true,"nightAlert":true,"sound":false,"wish":[],"ui":{}}' > "$SUP/settings.json"

echo "▶ R. 라이엇 연결 도우미 (앱 없이 바로 실행)"
REQ='{"method":"GET","url":"https://auth.riotgames.com/authorize?redirect_uri=https%3A%2F%2Fplayvalorant.com%2Fopt_in&client_id=play-valorant-web-prod&response_type=token%20id_token&nonce=1&scope=account%20openid","headers":{"User-Agent":"Mozilla/5.0 (Macintosh) Safari/605.1.15"},"noRedirect":true}'
echo "$REQ" | "$APPDIR/Contents/MacOS/ValoROAD" --riot-fetch > "$OUT/riot-fetch.json" 2>"$OUT/riot-fetch.err"
RST=$(js 'd.get("status")' < "$OUT/riot-fetch.json"); RLOC=$(js 'd.get("location","")[:80]' < "$OUT/riot-fetch.json")
echo "status=$RST location=$RLOC"
check "R1 도우미가 라이엇 로그인 주소에 연결 (상태 ${RST:-?} → ${RLOC:-없음})" bash -c "[ '${RST:-0}' -ge 300 ] && [ '${RST:-0}' -lt 400 ] && echo '$RLOC' | grep -qi 'riotgames.com'"
check "R2 로그인하지 않은 상태는 토큰을 주지 않음" bash -c "! echo '$RLOC' | grep -q 'access_token'"

echo "▶ A. 켜기 · 화면"
launch; check "A1 앱을 열면 엔진이 준비됨" ping_ok
check "A2 엔진이 Dock 앱 방식으로 켜짐(shell=native)" bash -c "curl -fs -H 'x-token: $(tok)' http://127.0.0.1:$(port)/api/ping | grep -q '\"shell\":\"native\"'"
check "A3 주소가 47931" test "$(port)" = "47931"
for _ in $(seq 1 90); do [ "$(api /api/state | js 'd["content"]["ready"]')" = "True" ] && break; sleep 1; done
NSK=$(api /api/state | js 'd["content"]["count"]')
check "A4 스킨 정보를 받음 (스킨 ${NSK:-0}개)" test "${NSK:-0}" -gt 500
NACC=$(api /api/state | js 'len(d["accounts"])')
check "A5 시험 계정 3개 (${NACC:-0})" test "${NACC:-0}" = "3"
for _ in $(seq 1 30); do api "/api/store?id=demoslot2" | js 'len(d["store"]["daily"])' | grep -q 4 && break; sleep 1; done
api "/api/store?id=demoslot2" > "$OUT/store2.json"
N2=$(js 'len(d["store"]["daily"])' < "$OUT/store2.json")
check "A6 계정 2 오늘의 상점 4개 (${N2:-0})" test "${N2:-0}" = "4"
IMG=$(js 'd["store"]["daily"][0]["img"]' < "$OUT/store2.json")
curl -fs -m 30 -H "x-token: $(tok)" "http://127.0.0.1:$(port)/api/img?u=$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1],safe=''))" "$IMG")" -o "$OUT/skin.png"
check "A7 실제 스킨 이미지를 받음" bash -c "file '$OUT/skin.png' | grep -q PNG"
SK=$(js 'd["store"]["daily"][0]["id"]' < "$OUT/store2.json")
api "/api/skin?id=$SK" > "$OUT/skin-detail.json"
NLV=$(js 'sum(1 for l in d["levels"] if l.get("video"))' < "$OUT/skin-detail.json"); NCH=$(js 'len(d["chromas"])' < "$OUT/skin-detail.json")
check "A11 미리보기 정보 (영상 있는 레벨 ${NLV:-0}개 · 변형 ${NCH:-0}개)" test "${NCH:-0}" -ge 1
# 계정 2 의 두 번째 스킨을 위시리스트에 담아 '위시리스트 적중' 화면을 봐요
WID=$(js 'd["store"]["daily"][1]["id"]' < "$OUT/store2.json")
apipost /api/wish "{\"id\":\"$WID\",\"on\":true}" > /dev/null
H=$(api /api/state | js 'sum(len(a["hits"]) for a in d["accounts"])')
check "A8 위시리스트 스킨이 상점에 뜬 것을 앎 (${H:-0}개)" test "${H:-0}" -ge 1
sleep 6
osascript -e 'tell application "ValoROAD" to activate' >/dev/null 2>&1; sleep 2
screencapture -x "$OUT/shot-app.png"
check "A9 화면이 앱 창(WKWebView)에서 열림" grep -q "ValoROADShell" "$SUP/requests.log"
apipost /api/settings '{"appearance":"light"}' > /dev/null; sleep 5
screencapture -x "$OUT/shot-light.png"
apipost /api/settings '{"appearance":"dark"}' > /dev/null
APID=$(app_pid); snap "A 실행 중"
SUPPID=$(ps -axo pid=,ppid=,comm= | awk -v a="$APID" '$2==a && /valoroad-engine/ {print $1}' | head -1)
CHILD=$(ps -axo pid=,ppid=,comm= | awk -v s="$SUPPID" '$2==s && /valoroad-engine/ {print $1}' | head -1)
check "A10 엔진이 ValoROAD 앱의 자식 (앱 $APID → 지킴이 ${SUPPID:-없음} → 엔진 ${CHILD:-없음})" test -n "$SUPPID" -a -n "$CHILD"

echo "▶ K. 키체인 보관 · 엔진의 라이엇 연결 방식"
K=$(api "/api/accounts/check?selftest=1"); echo "$K"
check "K1 키체인에 넣고 · 꺼내고 · 지움" bash -c "echo '$K' | grep -q '\"ok\":true'"
check "K2 엔진이 앱 도우미로 라이엇에 연결" bash -c "curl -fs http://127.0.0.1:$(port)/diag | grep -q '앱 도우미'"

echo "▶ N. 알림 (엔진 → 앱 → 맥 알림 센터)"
apipost /api/notify/test '{}' > /dev/null
for _ in $(seq 1 25); do grep -q "알림 보냄\|알림 실패" "$SUP/launcher.log" && break; sleep 1; done
check "N1 앱이 엔진의 알림을 가져가 맥 알림으로 보냄 ($(grep -o '알림 [보실][^:]*' "$SUP/launcher.log" | tail -1))" grep -q "알림 보냄\|알림 실패" "$SUP/launcher.log"

echo "▶ B. Dock 아이콘 다시 누르기(reopen)"
osascript -e 'tell application "ValoROAD" to reopen' >/dev/null 2>&1; sleep 1
check "B1 다시 열기 뒤에도 앱·엔진 그대로" test "$(app_pid)" = "$APID"

echo "▶ C. 엔진이 고장으로 꺼지면 지킴이가 다시 켬"
kill -SEGV "$CHILD" 2>/dev/null; sleep 4
check "C1 엔진 다시 켜짐" ping_ok
check "C2 앱은 그대로" test "$(app_pid)" = "$APID"
check "C3 다시 켜진 엔진도 같은 주소" test "$(port)" = "47931"

echo "▶ D. 앱 종료(⌘Q) → 엔진까지 모두 꺼짐"
osascript -e 'tell application "ValoROAD" to quit' >/dev/null 2>&1
check "D1 앱·엔진 모두 꺼짐" wait_gone
snap "D 종료 뒤"
cp "$SUP/clean-exit" "$OUT/clean-exit-D.txt" 2>/dev/null

echo "▶ E. 화면의 '종료' 버튼(= /api/quit) → 앱도 꺼짐"
launch
apipost /api/quit '{}' >/dev/null
check "E1 앱·엔진 모두 꺼짐" wait_gone

echo "▶ F. 앱이 강제로 꺼지면 엔진도 스스로 꺼짐"
launch; kill -9 "$(app_pid)"; sleep 1
check "F1 남은 엔진 없음" bash -c 'for i in $(seq 1 20); do [ -z "$(pgrep -x valoroad-engine)" ] && exit 0; sleep 0.5; done; exit 1'

echo "▶ G. 업데이트 뒤 다시 열기 (/api/update/restart)"
launch; OLD=$(app_pid)
apipost /api/update/restart '{}' >/dev/null
sleep 3
check "G1 새로 열린 앱이 엔진을 켬" wait_ready
NEW=$(app_pid)
check "G2 앱이 새로 열림 ($OLD → ${NEW:-없음})" test -n "$NEW" -a "$NEW" != "$OLD"
check "G3 앱 하나만 떠 있음 ($(pgrep -x ValoROAD | wc -l | tr -d ' ')개)" test "$(pgrep -x ValoROAD | wc -l | tr -d ' ')" = "1"
osascript -e 'tell application "ValoROAD" to quit' >/dev/null 2>&1
check "G4 끝나면 모두 꺼짐" wait_gone

echo "▶ 기록 모으기"
for f in launcher.log requests.log engine.log engine.prev.log engine.out engine.prev.out settings.json; do
  [ -f "$SUP/$f" ] && cp "$SUP/$f" "$OUT/$f"
done
security delete-generic-password -s ValoROAD >/dev/null 2>&1 || true
sw_vers > "$OUT/macos.txt"
echo
echo "== 결과 =="; cat "$RES"
! grep -q '^FAIL' "$RES"
