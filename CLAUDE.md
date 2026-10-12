# ValoROAD — notes for Claude

ValoROAD is a personal macOS app (Apple Silicon first; the Intel engine is bundled too) for **Nevertheless_D**, who is not a programmer.
It shows a VALORANT account's daily store, night market, featured bundle, wallet and owned skins without launching the game.
Talk to them in Korean, in plain words. Never ask them to use Terminal; give them a double-clickable result.
A Windows version is planned later (same design as YTROAD's Windows build: one .exe = launcher + engine).

## What it is

A "script-type" app, same structure as YTROAD: `ValoROAD.app/Contents/MacOS/ValoROAD` is a bash launcher (`app/launcher.sh`). It copies
`Contents/Resources/valoroad-<arm64|x86_64>` (Go binaries) to `~/Library/Application Support/ValoROAD/bin/valoroad-engine`, starts it on
127.0.0.1 with a random token and opens the UI in a Chrome/Edge/Brave/Whale `--app` window.

- `engine/` — Go, stdlib only. Embeds `web/`.
  - `main.go` routes, settings (appearance, notify, dailyCheck, autoUpdate, lastAccount), poll, store/catalog/skin/wishlist handlers
  - `cdp.go` + `ws.go` — Riot sign-in happens in the person's own Chrome: a separate Chrome process per account profile
    (`Support/accounts/<puuid>/browser`) with `--remote-debugging-port=0`; a tiny websocket/CDP client watches for the redirect to
    `playvalorant.com/opt_in#access_token=…` and reads the riotgames.com cookies. Headless mode of the same profile is the silent re-login fallback.
  - `riot.go` — unofficial endpoints: cookie reauth (`auth.riotgames.com/authorize`, no redirects), entitlements, userinfo, riot-geo (region→shard),
    `pd.<shard>.a.pvp.net` storefront v3 (POST, falls back to v2 GET), wallet, owned skins, account-xp, playerloadout.
  - `accounts.go` — up to 3 accounts (`accounts.json`), cookies in the macOS keychain (service "ValoROAD", via /usr/bin/security; file fallback),
    1-hour session tokens in `accounts/<id>/session.json` (0600), login manager (stages opening/waiting/login/finishing/done/error/cancelled).
  - `catalog.go` — valorant-api.com (ko-KR) weapons/skins/levels/chromas/videos, content tiers, bundles; cached in `cache/catalog.json` (bump `catalogFormat` when the shape changes).
  - `store.go` — builds the screen model, wishlist (`wishlist.json`, skin uuids), hits, notifications (deduped in `notified.json`).
  - `schedule.go` — reset loop while the app is open, and the LaunchAgent `com.nevertheless-d.valoroad.daily` that runs `valoroad-engine --check`
    at 00:02 UTC (= 09:02 KST) + RunAtLoad. `--check` refreshes every account and sends wishlist / "다시 로그인" notifications.
  - `update.go` — app auto-update from `releases/latest.json` in this repo (raw.githubusercontent.com), sha256 check, backup + rollback, restart on same port/token.
- `engine/web/` — `index.html`, `css/app.css` (FPS/tactical look, Valorant red #ff4655, navy #0f1923, bone #ece8e1, clip-path cut corners,
  Chakra Petch + Pretendard), `js/art.js` (SVG illustrations), `js/app.js` (all UI logic).
- Footer must say `made by. Nevertheless_D`. Default theme follows macOS; light/dark switch in the header and settings.

## Releasing an update (this is how the user's app updates itself)

1. Make the change. Bump `VERSION` (semver).
2. `scripts/build.sh --notes "바뀐 점 1" "바뀐 점 2"` (Korean, user-facing). Writes `releases/ValoROAD-<v>.zip` + `releases/latest.json`
   and `dist/ValoROAD.zip` (fresh-install zip: app + usage guide).
3. Commit everything (including the zip and latest.json) and push to `main`. Installed apps pick it up within 6 hours or via the version badge.
4. Keep only the three newest zips in `releases/`.

Never publish a `latest.json` whose zip is missing or whose `version.txt` differs — the app verifies both.
Commit as `Nevertheless_D <NeverthelessD@users.noreply.github.com>`.
Icon: edit `app/AppIcon.svg`, render to `app/AppIcon.png` (1024, e.g. with Playwright), then `scripts/make_icns.py app/AppIcon.png app/AppIcon.icns`
and copy a 256px version to `engine/web/icon.png`.

## Testing without a Mac or a Riot account

- `python3 scripts/mock_riot.py 9900` — fake Riot auth/login page/pd servers using real item ids from valorant-api.com
  (admin: `/admin/next?acct=a1|a2|a3`, `/admin/expire?acct=`, `/admin/block?on=1` (blocks non-browser reauth → tests silent browser re-login),
  `/admin/force` with a JSON list of skin uuids to put in a1's daily store, `/admin/rotate`). Its login page auto-submits after ~1.2s.
- Run the engine with `VALOROAD_SUPPORT`, `VALOROAD_TOKEN`, `VALOROAD_PORT`, `VALOROAD_MOCK=http://127.0.0.1:9900`,
  `VALOROAD_BROWSER=<playwright chromium>`, `VALOROAD_BROWSER_ARGS="--no-sandbox --headless=new"`, `VALOROAD_BROWSER_NAME=크롬`,
  `VALOROAD_NOTIFY_LOG=<file>` (notifications are appended there), `VALOROAD_UPDATE_URL=<local latest.json>`, `VALOROAD_NO_KEYCHAIN=1`.
- Drive the UI with Playwright. Playwright's Chromium can't play the Riot mp4 videos (no H.264) — the detail view falls back to the image.
- Update test: fake `ValoROAD.app` with a Linux engine named `valoroad-x86_64`, `HOME` = temp dir, run the launcher with bash.
- Careful: `pkill -f valoroad-engine` also matches your own shell command line; use `pgrep -f '^/tmp/valoroad-engine'` and kill the pids.

Only a real Mac can confirm: Chrome login window + CDP, keychain, LaunchAgent, osascript notifications, Gatekeeper, real Riot responses.

## User-facing text

Never mention GitHub (or the repo name) in anything the user sees — app screens, update messages, usage-guide images.
Say "새 버전이 나오면 앱이 알아서 받아요" instead. Always remind that the app is unofficial (not made by Riot) and read-only.
