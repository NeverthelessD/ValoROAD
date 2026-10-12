# ValoROAD

발로란트 오늘의 상점 · 야시장 확인 앱 (macOS) — made by. Nevertheless_D

게임을 켜지 않고 Riot 계정으로 한 번만 로그인하면 오늘의 상점 · 야시장 · 번들 · 잔액을 볼 수 있어요. 개인적인 용도로만 사용해 주세요.

- **계정 3개**: 왼쪽에서 계정을 누르면 그 계정의 상점이 바로 보여요. 로그아웃 없이 바로 전환
- **자동 로그인 유지**: 비밀번호는 저장하지 않고, Riot이 준 로그인 정보만 맥의 키체인에 보관해요
- **오늘의 상점 · 야시장 · 번들**: 스킨 이름 · 이미지 · 등급 · 가격 · 할인율 · 남은 시간 · 이미 가진 스킨 표시
- **스킨 자세히 보기**: 레벨별 인게임 영상 · 색상(크로마)
- **위시리스트 알림**: 담아 둔 스킨이 계정 3개 중 어느 상점에 떠도 맥 알림. 매일 오전 9시(상점 교체 시각)에 앱이 꺼져 있어도 자동 확인
- **화면**: 라이트 / 다크 / 시스템 설정 따르기(기본)
- **자동 업데이트**: 앱이 이 저장소의 `releases/latest.json`을 보고 새 버전을 받아요. 오른쪽 위 버전 표시에서 수동 확인 · 되돌리기

> ValoROAD는 Riot Games가 만들거나 승인한 앱이 아니에요. 게임 클라이언트가 쓰는 비공식 방식으로 상점을 **읽기만** 해요.

## 설치 — macOS (Apple Silicon · Intel)

1. [`releases/`](releases) 폴더에서 가장 최신 `ValoROAD-x.y.z.zip`을 받아 압축을 풀어요.
2. `ValoROAD.app`을 **응용 프로그램** 폴더로 옮겨요. (업데이트가 동작하려면 꼭 옮겨 주세요)
3. 처음 열 때 경고가 뜨면 **시스템 설정 → 개인정보 보호 및 보안 → "그래도 열기"**.
4. Riot 로그인은 구글 크롬(또는 엣지 · 브레이브 · 웨일) 창에서 해요.

모든 구성요소는 `~/Library/Application Support/ValoROAD` 한 폴더에만 설치돼요.
지우려면 설정에서 **매일 자동 확인을 끄고**, 앱을 휴지통에 버린 뒤 그 폴더도 지우면 끝이에요.
사용 가이드: [`docs/ValoROAD_사용가이드.png`](docs)

## 사용한 오픈소스 · 자료

- 스킨 이름 · 이미지 · 영상: [valorant-api.com](https://valorant-api.com) (커뮤니티 API)
- 글꼴: [Pretendard](https://github.com/orioncactus/pretendard), [Chakra Petch](https://fonts.google.com/specimen/Chakra+Petch) (SIL OFL)

## 폴더 구조

```
engine/     Go 엔진 (로컬 서버 · Riot 로그인/상점 · 위시리스트 알림 · 매일 확인 · 업데이트)
engine/web/ 앱 화면 (HTML · CSS · JS · 일러스트)
app/        ValoROAD.app 껍데기 (실행 스크립트 · Info.plist · 아이콘)
scripts/    빌드 · 아이콘 · 테스트용 가짜 Riot 서버
releases/   업데이트 파일 (ValoROAD-x.y.z.zip · latest.json)
docs/       사용 가이드 이미지
```
