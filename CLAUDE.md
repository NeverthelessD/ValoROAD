# ValoROAD — notes for Claude

**Read `PROJECT_NOTES.md` first** (structure, decisions, progress checklist, Riot API notes) and update it at the end of every task.
ValoROAD is a personal macOS (Apple Silicon) Valorant store checker for **Nevertheless_D**, who is not a programmer:
up to 3 saved Riot accounts with auto re-login, per-account daily store / bundles / night market with real skin names and images,
and wishlist alerts. View only — no purchasing. Talk to the user in Korean, in plain words. Never ask them to use Terminal.
Never mention GitHub (or the repo name) in anything the user sees. Follows the `road-mac-app` skill and the MSROAD/SROAD/YTROAD pattern.

**ValoROAD.app is a real, visible Dock app** (Swift shell `app/shell/main.swift`, WKWebView window); the Go engine runs as its child
(app → supervisor → engine) only while the app is open, per macOS 27's "Background App Activity" rule — never add hidden helpers,
nohup, or SIGTERM-ignoring. Riot login happens in a login window inside the shell (Riot's own page); the app never sees passwords.
Riot cookies live in the macOS Keychain (`/usr/bin/security`, service `ValoROAD`). On a Mac, riotgames.com requests go through
`ValoROAD --riot-fetch` (URLSession) so Riot sees Apple's TLS stack.

The app can only be built on a Mac: pushing changes under app/, engine/ or VERSION triggers `.github/workflows/build-mac.yml`, which builds,
ad-hoc signs, runs real-Mac smoke tests (demo mode, no real Riot login) and force-pushes results to the `ci-results` branch
(this session's proxy blocks the Actions API, so read results via git). Release with `scripts/release.sh`.
Linux testing: `VALOROAD_DEMO=1` engine + Playwright (see PROJECT_NOTES.md).
`signal.Ignore` can't be undone with `signal.Reset`. In tests use `pkill -x`, never `pkill -f valoroad-engine`.
Commit author: `Nevertheless_D <NeverthelessD@users.noreply.github.com>`.
