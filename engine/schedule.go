package main

// 매일 상점이 바뀌는 시각(00:00 UTC = 한국 오전 9시)에 계정들을 확인해서 위시리스트 알림을 보냅니다.
// 앱이 꺼져 있어도 되도록 macOS LaunchAgent(로그인 항목과 비슷한 것)로 엔진을 잠깐 실행해요.

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

const agentLabel = "com.nevertheless-d.valoroad.daily"

// dailyCheckTime is the local clock time of the store reset (+2 minutes).
func dailyCheckTime() (int, int) {
	now := time.Now().UTC()
	reset := time.Date(now.Year(), now.Month(), now.Day(), 0, 2, 0, 0, time.UTC).Local()
	return reset.Hour(), reset.Minute()
}

func dailyCheckLabel() string {
	h, m := dailyCheckTime()
	ampm := "오전"
	hh := h
	if h >= 12 {
		ampm = "오후"
		if h > 12 {
			hh = h - 12
		}
	}
	if hh == 0 {
		hh = 12
	}
	if m <= 5 {
		return fmt.Sprintf("%s %d시", ampm, hh)
	}
	return fmt.Sprintf("%s %d시 %d분", ampm, hh, m)
}

func agentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", agentLabel+".plist")
}

func agentPlist(engine, support string) []byte {
	h, m := dailyCheckTime()
	esc := func(s string) string {
		var b bytes.Buffer
		for _, r := range s {
			switch r {
			case '&':
				b.WriteString("&amp;")
			case '<':
				b.WriteString("&lt;")
			case '>':
				b.WriteString("&gt;")
			default:
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>--check</string>
	</array>
	<key>EnvironmentVariables</key>
	<dict>
		<key>VALOROAD_SUPPORT</key>
		<string>%s</string>
	</dict>
	<key>StartCalendarInterval</key>
	<dict>
		<key>Hour</key>
		<integer>%d</integer>
		<key>Minute</key>
		<integer>%d</integer>
	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Background</string>
	<key>LowPriorityIO</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, agentLabel, esc(engine), esc(support), h, m, esc(filepath.Join(support, "check.log")), esc(filepath.Join(support, "check.log"))))
}

func (a *App) engineBinary() string {
	return filepath.Join(a.binDir, "valoroad-engine")
}

// syncDailyAgent installs, updates or removes the LaunchAgent to match the setting.
func (a *App) syncDailyAgent() error {
	if runtime.GOOS != "darwin" && os.Getenv("VALOROAD_FAKE_AGENT") == "" {
		return nil
	}
	p := agentPath()
	uid := strconv.Itoa(os.Getuid())
	if !a.Settings().DailyCheck {
		if fileExists(p) {
			exec.Command("/bin/launchctl", "bootout", "gui/"+uid+"/"+agentLabel).Run()
			os.Remove(p)
		}
		return nil
	}
	if !fileExists(a.engineBinary()) {
		return errors.New("엔진 파일이 없어요")
	}
	want := agentPlist(a.engineBinary(), a.support)
	if old, err := os.ReadFile(p); err == nil && bytes.Equal(old, want) {
		return nil
	}
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, want, 0o644); err != nil {
		return err
	}
	if runtime.GOOS != "darwin" {
		return nil
	}
	exec.Command("/bin/launchctl", "bootout", "gui/"+uid+"/"+agentLabel).Run()
	if out, err := exec.Command("/bin/launchctl", "bootstrap", "gui/"+uid, p).CombinedOutput(); err != nil {
		log.Printf("launchctl bootstrap: %v %s", err, out)
		if out2, err2 := exec.Command("/bin/launchctl", "load", "-w", p).CombinedOutput(); err2 != nil {
			log.Printf("launchctl load: %v %s", err2, out2)
			return errors.New("자동 확인을 등록하지 못했어요")
		}
	}
	return nil
}

// ---------------------------------------------------------------- 앱이 켜져 있을 때

func (a *App) nextReset() time.Time {
	now := time.Now()
	next := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)
	for _, acc := range a.acc.List() {
		v := a.stores.Cached(acc.ID)
		if v == nil {
			continue
		}
		for _, t := range []int64{v.DailyEndsAt, v.NightEndsAt} {
			if t > now.UnixMilli() && time.UnixMilli(t).Before(next) {
				next = time.UnixMilli(t)
			}
		}
		for _, b := range v.Bundles {
			if b.EndsAt > now.UnixMilli() && time.UnixMilli(b.EndsAt).Before(next) {
				next = time.UnixMilli(b.EndsAt)
			}
		}
	}
	return next
}

func (a *App) resetLoop() {
	time.Sleep(3 * time.Second)
	a.refreshStale()
	for {
		next := a.nextReset().Add(20 * time.Second)
		wait := time.Until(next)
		if wait > 20*time.Minute {
			wait = 20 * time.Minute
		}
		if wait < 5*time.Second {
			wait = 5 * time.Second
		}
		time.Sleep(wait)
		a.refreshStale()
	}
}

// refreshStale reloads every account whose cached store has rotated (or was never loaded).
func (a *App) refreshStale() {
	if err := a.cat.Ensure(false); err != nil {
		log.Printf("catalog: %v", err)
	}
	for _, acc := range a.acc.List() {
		if acc.Status == "expired" {
			continue
		}
		v := a.stores.Cached(acc.ID)
		if !v.Stale() {
			continue
		}
		if nv, err := a.stores.Refresh(acc.ID); err == nil {
			if cur := a.acc.Get(acc.ID); cur != nil {
				a.stores.NotifyWishlist(cur, nv, a.nlog, a.Settings().Notify)
			}
		}
	}
}

// ---------------------------------------------------------------- --check (LaunchAgent)

func runCheck() {
	home, _ := os.UserHomeDir()
	support := os.Getenv("VALOROAD_SUPPORT")
	if support == "" {
		support = defaultSupportDir(home, "ValoROAD")
	}
	log.SetFlags(log.LstdFlags)
	app = &App{support: support, binDir: filepath.Join(support, "bin")}
	app.loadSettings()
	if !app.settings.DailyCheck {
		log.Printf("daily check is off")
		return
	}
	app.cat = NewCatalog(support)
	app.acc = NewAccounts(support)
	app.wish = NewWishlist(support)
	app.stores = NewStores(support, app.acc, app.cat, app.wish)
	app.nlog = &notifyLog{path: filepath.Join(support, "notified.json")}
	log.Printf("ValoROAD %s daily check", build)
	// 맥이 막 깨어났을 때를 위해 인터넷이 잡힐 때까지 조금 기다려요
	for i := 0; i < 12; i++ {
		if err := app.cat.Ensure(false); err == nil {
			break
		}
		time.Sleep(10 * time.Second)
	}
	for _, acc := range app.acc.List() {
		v := app.stores.Cached(acc.ID)
		if !v.Stale() {
			// 이미 오늘 상점을 받아 뒀으면 알림만 (이미 보낸 알림은 다시 안 보내요)
			app.stores.NotifyWishlist(&acc, v, app.nlog, app.settings.Notify)
			continue
		}
		nv, err := app.stores.Refresh(acc.ID)
		if err != nil {
			log.Printf("%s: %v", acc.Name, err)
			if errors.Is(err, errSessionExpired) && app.settings.Notify {
				day := time.Now().Format("2006-01-02")
				if app.nlog.firstTime("expired:"+acc.ID+":"+day, 0) {
					notifyOS("ValoROAD", acc.Name+"#"+acc.Tag+" 계정은 다시 로그인이 필요해요. 앱을 열어 다시 로그인해 주세요.")
				}
			}
			continue
		}
		cur := app.acc.Get(acc.ID)
		if cur == nil {
			cur = &acc
		}
		hits := app.stores.NotifyWishlist(cur, nv, app.nlog, app.settings.Notify)
		log.Printf("%s: %d daily, %d night, %d wishlist hits", acc.Name, len(nv.Daily), len(nv.Night), len(hits))
	}
}
