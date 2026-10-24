> **이 파일을 읽은 Claude는 작업이 끝날 때마다 이 파일을 업데이트할 것.**
> (구조가 바뀌면 「구조」, 새로 정한 것은 「결정 사항」, 한 일·남은 일은 「진행 사항」에 적고 커밋·푸시까지 할 것)

# ValoROAD — 프로젝트 노트

발로란트 상점 확인 앱 (계정 3개 · 자동 로그인 · 계정별 오늘의 상점/번들/밤시장 · 실제 스킨 이름과 이미지 · 위시리스트 알림).
macOS (Apple Silicon). made by. Nevertheless_D
사용자는 개발자가 아니에요. 한국어로 쉽게 설명하고, 터미널 사용을 요구하지 말 것. 결과물은 더블클릭으로 쓸 수 있게.
구매 기능은 없어요(확인 전용).

---

## 진행 사항

작업이 중간에 끊기면 아래 체크리스트에서 **처음으로 체크 안 된 항목부터** 이어서 하면 돼요. 각 단계가 끝날 때마다 커밋 · 푸시.

- [x] 0. 디자인 시안 → 사용자 확정 (2026-10-24, Design 캔버스 'ValoROAD 디자인 시안' v2 + 아이콘 B · SCOPE)
- [x] 1. 라이엇 로그인 · 상점 API 최신 방식 확인 (아래 「라이엇 연결」)
- [x] 2. 저장소 뼈대 (이 파일, CLAUDE.md, VERSION, .gitignore)
- [ ] 3. 엔진: 계정 3개 · 키체인 · 쿠키 재로그인 · 상점/지갑/밤시장/번들 · 스킨 데이터(valorant-api.com) · 이미지 캐시 · 위시리스트 · 알림 · 업데이트 · 진단
- [ ] 4. 화면: 상점 · 위시리스트 · 설정 · 계정 추가 안내, 라이트/다크, 폰트 동봉
- [ ] 5. 맥 앱 껍데기(Swift): Dock 앱 · 엔진 지킴이 · 라이엇 로그인 창 · 알림 · --riot-fetch 도우미
- [ ] 6. 리눅스에서 데모 모드 + Playwright 로 화면 시험
- [ ] 7. 맥 빌드 서버(빌드 · smoke) 통과
- [ ] 8. 사용 가이드 이미지 · v1.0.0 릴리스 (releases/latest.json, 처음 설치용 zip)
- [ ] 9. 사용자 맥에서 실제 라이엇 로그인 · 상점 확인

### 작업 기록
- 2026-10-24 디자인 확정. 사용자 요청 정리: 앱 이름 ValoROAD, 발로란트/FPS 느낌, 계정 3개 저장 + 자동 로그인 유지,
  계정을 누르면 로그아웃 없이 그 계정 상점, 실제 스킨 이름 · 이미지, 위시리스트 스킨이 뜨면 알림, 구매 기능 없음, 로그인 문제 잘 해결.

---

## 결정 사항

- 방식: MSROAD 1.1.0~ 와 같은 **Dock 에 보이는 정식 맥 앱** (road-mac-app 스킬). Swift 껍데기 + WKWebView, Go 엔진은 앱의 자식으로만.
- 주소 127.0.0.1:47931 + 토큰. 환경 변수 접두사 `VALOROAD_`. 엔진 파일 `valoroad-engine`. 번들 id `com.nevertheless-d.valoroad`.
- 사용자에게 보이는 곳에는 GitHub · 저장소 이름을 쓰지 않는다. 화면 아래 · 앱 정보 · 가이드에 `made by. Nevertheless_D`.
- 화면 곳곳에 "ValoROAD는 Riot Games가 만들거나 보증한 앱이 아니에요" 문구 (Riot 팬 콘텐츠 규정 정신).
- **디자인 (확정)**: 남색 바탕 #0F1923 · 패널 #182633 · 크림 #ECE8E1 · 빨강 #FF4655 · 위시 적중 청록 #3DDBC4.
  라이트 모드: 크림 바탕 #ECE8E1, 글자 #0F1923, 빨간 글자 #C42536(대비 4.5:1). 글꼴: Anton(큰 영어 제목) ·
  Barlow Condensed(라벨 · 숫자) · IBM Plex Sans KR(한글) — 모두 OFL, engine/web/fonts 에 동봉.
  모양: 모서리 한쪽을 비스듬히 자른 카드/버튼(clip-path), 큰 외곽선 글자(STORE · WISHLIST · SETTINGS) 배경,
  위쪽 탭 + 빨간 밑줄, 보고 있는 계정 = 빨간 블록, 등급 색으로 물든 스킨 카드 + 마름모 등급 표시.
  등급 색: 셀렉트 #5A9FE2 · 디럭스 #2FC48D · 프리미엄 #D1548D · 익스클루시브 #F5955B · 울트라 #FAD663.
- **아이콘 (확정 B · SCOPE)**: 남색 둥근 사각형, 빨간 조준경 고리(4곳 끊김) + 바깥 눈금 4개, 가운데 크림색 'VR'
  (Anton 글자 모양을 그대로 따서 path 로). app/AppIcon.svg.
- 게임 로고 · 라이엇 실제 화면 · 유료 게임 폰트는 쓰지 않는다. 스킨 이미지와 이름은 앱이 실행 중에 valorant-api.com 에서 받는다.
- 계정은 최대 3개(슬롯 1~3). 같은 계정(PUUID)을 다시 로그인하면 그 슬롯을 새로 고친다.
- 위시리스트 알림: 앱이 켜져 있는(Dock) 동안만 확인. 매일 상점이 바뀌면(한국 오전 9시) 확인, 앱이 꺼져 있었다면 다음에 켤 때 알림.
  백그라운드 도우미로 몰래 돌지 않는다(스킬 원칙).
- 커밋 작성자: `Nevertheless_D <NeverthelessD@users.noreply.github.com>`

## 라이엇 연결 (2026-10 확인)

공식 API 가 아니라서 바뀔 수 있음. 참고: valapidocs.techchrism.me, 공개 상점 확인 앱들.
- **로그인**: 앱 안의 로그인 창(WKWebView, 계정마다 새 비영구 저장소)에 라이엇 공식 페이지
  `https://auth.riotgames.com/authorize?redirect_uri=https%3A%2F%2Fplayvalorant.com%2Fopt_in&client_id=play-valorant-web-prod&response_type=token%20id_token&nonce=1&scope=account%20openid`
  를 띄우고, 사용자가 직접 로그인(2단계 인증 · 캡차 포함) → `playvalorant.com/opt_in#access_token=…&id_token=…` 로 가는 순간 가로챔.
  그 창의 riotgames.com 쿠키(ssid 등)를 엔진에 넘김. **비밀번호는 앱이 보지도 저장하지도 않음.**
- **자동 로그인(쿠키 재로그인)**: 같은 authorize 주소를 쿠키와 함께 GET(리디렉트 따라가지 않음) → Location 의 # 뒤에 새 토큰.
  login 페이지로 가면 만료 → 그 계정만 '다시 로그인'. 쿠키는 맥 키체인(서비스 ValoROAD)에 보관, 새 Set-Cookie 로 갱신.
  맥에서는 riotgames.com 요청을 앱 껍데기의 `--riot-fetch` 도우미(URLSession, 애플 TLS)로 보냄 — 브라우저가 아닌 TLS 를 막는 경우 대비.
- 계정 정보: `GET https://auth.riotgames.com/userinfo` (sub = PUUID, acct.game_name · tag_line)
- 지역: `PUT https://riot-geo.pas.si.riotgames.com/pas/v1/product/valorant` body `{"id_token":…}` → affinities.live (kr/ap/eu/na/latam/br → shard)
- 엔타이틀먼트: `POST https://entitlements.auth.riotgames.com/api/token/v1` → entitlements_token
- 상점: `POST https://pd.{shard}.a.pvp.net/store/v3/storefront/{puuid}` body `{}` (v2 GET 은 더 이상 안 씀)
  헤더: Authorization Bearer · X-Riot-Entitlements-JWT · X-Riot-ClientPlatform(base64) · X-Riot-ClientVersion(valorant-api.com/v1/version 의 riotClientVersion)
  SkinsPanelLayout.SingleItemStoreOffers(오늘의 상점, 보상 ItemID = 스킨 레벨 uuid) · FeaturedBundle.Bundles · BonusStore(밤시장)
- 지갑: `GET https://pd.{shard}.a.pvp.net/store/v1/wallet/{puuid}` — VP 85ad13f7-3d1b-5128-9eb2-7cd8ee0b5741 ·
  RP e59aa87c-4cbf-517a-5983-6e81511be9b7 · KC 85ca954a-41f2-ce94-9b45-8ca3dd39a00d
- 스킨 데이터: `https://valorant-api.com/v1/weapons?language=ko-KR`(+en-US 로 영어 무기 이름) · contenttiers · bundles. 버전이 바뀔 때만 다시 받아 Support/content 에 줄여서 저장.
  이미지는 media.valorant-api.com 만 엔진이 받아 Support/cache/img 에 저장해 보여 줌(/api/img).

## 구조

(엔진 · 껍데기를 만든 뒤 채울 것)

## 업데이트 내보내는 법 (Claude용)

1. 수정 → `VERSION` 올리기 → 커밋 · 푸시 (app/ · engine/ · VERSION 이 바뀌면 맥 빌드 서버가 자동으로 돎, 5~10분)
2. `git ls-remote origin ci-results` 가 바뀔 때까지 기다린 뒤 결과 확인:
   `git fetch origin +refs/heads/ci-results:refs/remotes/origin/ci-results && git archive origin/ci-results | tar -x -C <임시폴더>`
   → status.json(build/smoke 모두 ok), results.txt, shot-*.png 확인 (화면이 확정 시안과 같은지도)
3. `scripts/release.sh --notes "바뀐 점 1" "바뀐 점 2"` → 커밋 · 푸시. 처음 설치용은 dist/ValoROAD.zip.
4. `releases/`에는 최신 zip 3개만. zip 이 없거나 version.txt 가 다른 latest.json 은 절대 올리지 않는다.
