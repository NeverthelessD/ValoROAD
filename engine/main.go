// ValoROAD — 발로란트 상점 확인 (macOS, Apple Silicon)
// made by. Nevertheless_D
//
// 엔진: 계정 3개 · 자동 로그인(쿠키) · 계정별 상점/지갑/밤시장/번들 · 스킨 데이터 · 이미지 캐시 · 위시리스트 알림 · 업데이트 · 진단
// 화면(web/)은 이 엔진이 127.0.0.1 에서 보여 주고, ValoROAD.app(Swift, Dock 앱)이 앱 창으로 띄워요.
package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// build is the app version, set with -ldflags "-X main.build=1.2.3".
var build = "0.0.0-dev"

//go:embed web
var webFiles embed.FS

type Settings struct {
	Appearance string         `json:"appearance"` // system | light | dark
	AutoUpdate bool           `json:"autoUpdate"`
	WishAlert  bool           `json:"wishAlert"`  // 위시리스트 스킨이 오늘의 상점에 뜨면 알림
	NightAlert bool           `json:"nightAlert"` // 밤시장에 뜬 것도 알림
	Sound      bool           `json:"sound"`      // 알림 소리
	Wish       []string       `json:"wish"`       // 위시리스트 (스킨 uuid)
	Active     string         `json:"active"`     // 마지막으로 본 계정 id
	UI         map[string]any `json:"ui"`
}

type App struct {
	support string
	token   string
	port    int

	mu       sync.Mutex
	settings Settings

	upd      *Updater
	content  *Content
	accounts *Accounts
	notes    *Notifier
	prices   *Prices

	rev      atomic.Int64 // 화면이 다시 읽어야 할 때마다 늘어나요
	lastSeen atomic.Int64
}

var app *App

// started 는 단조 시계 기준점이에요 (잠자기 시간은 빼고 흘러요).
var started = time.Now()

func touch() { app.lastSeen.Store(int64(time.Since(started))) }

func (a *App) bump() { a.rev.Add(1) }

func env(k string) string { return os.Getenv("VALOROAD_" + k) }

func main() {
	log.SetFlags(log.LstdFlags)
	home, _ := os.UserHomeDir()
	support := env("SUPPORT")
	if support == "" {
		if runtime.GOOS == "darwin" {
			support = filepath.Join(home, "Library", "Application Support", "ValoROAD")
		} else {
			support = filepath.Join(home, ".valoroad")
		}
	}
	if env("SUPERVISOR") == "1" {
		os.MkdirAll(support, 0o755)
		runSupervisor(support)
		return
	}
	token := env("TOKEN")
	if token == "" {
		token = randID(16)
	}
	app = &App{support: support, token: token}
	os.MkdirAll(support, 0o755)
	os.Rename(filepath.Join(support, "engine.log"), filepath.Join(support, "engine.prev.log"))
	if f, err := os.OpenFile(filepath.Join(support, "engine.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	}
	app.loadSettings()
	touch()
	app.upd = NewUpdater(support)
	app.content = NewContent(support)
	app.notes = NewNotifier(support)
	app.prices = NewPrices(support)
	app.accounts = NewAccounts(support, app.content)

	go heartbeat(support)
	ln := listen(env("PORT"), env("PORT_WAIT") == "1")
	app.port = ln.Addr().(*net.TCPAddr).Port
	writeFileAtomic(filepath.Join(support, "token"), []byte(token), 0o600)
	writeFileAtomic(filepath.Join(support, "port"), []byte(strconv.Itoa(app.port)), 0o644)
	writeFileAtomic(filepath.Join(support, "lastport"), []byte(strconv.Itoa(app.port)), 0o644)
	log.Printf("ValoROAD %s listening on %d (pid %d, 지킴이 %v, 데모 %v)", build, app.port, os.Getpid(), env("CHILD") == "1", demoMode())

	// 다른 주소에서 켜졌더라도 진단 페이지만은 늘 기본 주소에서 볼 수 있게 해요
	if app.port != defaultPort {
		go func() {
			defer recoverLog("diag listener")
			if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", defaultPort)); err == nil {
				m := http.NewServeMux()
				m.HandleFunc("/diag", app.handleDiag)
				http.Serve(ln, hostGuard(m))
			}
		}()
	}
	go app.watchParent()
	go func() { defer recoverLog("content"); app.content.loop() }()
	go func() { defer recoverLog("accounts"); app.accounts.loop() }()
	go func() { defer recoverLog("autoLoop"); app.upd.autoLoop() }()
	srv := &http.Server{Handler: hostGuard(recordRequests(app.routes())), ErrorLog: log.Default()}
	log.Fatal(srv.Serve(ln))
}

// defaultPort: 항상 같은 주소를 쓰면 엔진이 다시 켜져도 열려 있던 창이 이어지고, 진단 페이지도 찾기 쉬워요.
const defaultPort = 47931

func listen(pref string, wait bool) net.Listener {
	p, err := strconv.Atoi(pref)
	if err != nil || p <= 0 {
		p = defaultPort
	}
	tries := 1
	if wait { // 다시 시작할 때는 이전 엔진이 주소를 놓을 때까지 잠깐 기다려요
		tries = 20
	}
	for i := 0; i < tries; i++ {
		if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p)); err == nil {
			return ln
		}
		time.Sleep(150 * time.Millisecond)
	}
	if p != defaultPort {
		if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", defaultPort)); err == nil {
			return ln
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	return ln
}

// ---------------------------------------------------------------- lifecycle

// nativeShell: Dock 에 보이는 맥 앱(app/shell/main.swift)이 엔진을 켰는지
func nativeShell() bool { return env("SHELL") == "native" }

// watchParent: 지킴이(부모)가 사라지면 엔진도 끝나요 — 혼자 남아 몰래 돌지 않게
func (a *App) watchParent() {
	if env("CHILD") != "1" {
		return
	}
	ppid := os.Getppid()
	for range time.Tick(2 * time.Second) {
		if os.Getppid() != ppid {
			a.shutdownWhy("지킴이가 사라짐")
		}
	}
}

func (a *App) shutdownWhy(why string) {
	log.Printf("shutdown: %s", why)
	markCleanExit(a.support, why)
	if b, err := os.ReadFile(filepath.Join(a.support, "port")); err == nil && string(b) == strconv.Itoa(a.port) {
		os.Remove(filepath.Join(a.support, "port"))
	}
	os.Exit(0)
}

// ---------------------------------------------------------------- settings

func (a *App) settingsPath() string { return filepath.Join(a.support, "settings.json") }

func (a *App) loadSettings() {
	a.settings = Settings{Appearance: "system", AutoUpdate: true, WishAlert: true, NightAlert: true, UI: map[string]any{}}
	if b, err := os.ReadFile(a.settingsPath()); err == nil {
		_ = json.Unmarshal(b, &a.settings)
	}
	if a.settings.UI == nil {
		a.settings.UI = map[string]any{}
	}
	if a.settings.Wish == nil {
		a.settings.Wish = []string{}
	}
}

func (a *App) saveSettings() {
	b, _ := json.MarshalIndent(a.settings, "", "  ")
	writeFileAtomic(a.settingsPath(), b, 0o644)
}

func (a *App) Settings() Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.settings
	s.UI = make(map[string]any, len(a.settings.UI))
	for k, v := range a.settings.UI {
		s.UI[k] = v
	}
	s.Wish = append([]string(nil), a.settings.Wish...)
	return s
}

func (a *App) wishSet() map[string]bool {
	s := a.Settings()
	m := make(map[string]bool, len(s.Wish))
	for _, id := range s.Wish {
		m[id] = true
	}
	return m
}

func (a *App) settingsJSON() map[string]any {
	s := a.Settings()
	return map[string]any{
		"appearance": s.Appearance,
		"autoUpdate": s.AutoUpdate,
		"wishAlert":  s.WishAlert,
		"nightAlert": s.NightAlert,
		"sound":      s.Sound,
		"active":     s.Active,
		"ui":         s.UI,
		"build":      build,
		"platform":   runtime.GOOS + "/" + runtime.GOARCH,
		"shell":      env("SHELL"),
		"demo":       demoMode(),
	}
}

// ---------------------------------------------------------------- routes

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(webFiles, "web")
	static := http.FileServer(http.FS(sub))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			if r.URL.Query().Get("t") != a.token {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				io.WriteString(w, "<meta charset=utf-8><body style='font:15px -apple-system;padding:40px;background:#0F1923;color:#ECE8E1'>ValoROAD 앱을 다시 실행해 주세요.</body>")
				return
			}
			b, _ := webFiles.ReadFile("web/index.html")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Write(b)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".js") || strings.HasSuffix(r.URL.Path, ".css") {
			w.Header().Set("Cache-Control", "no-cache")
		} else if strings.HasPrefix(r.URL.Path, "/fonts/") {
			w.Header().Set("Cache-Control", "public, max-age=604800")
		}
		static.ServeHTTP(w, r)
	})

	mux.HandleFunc("/diag", a.handleDiag)

	api := func(path string, h http.HandlerFunc) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			t := r.Header.Get("x-token")
			if t == "" {
				t = r.URL.Query().Get("t")
			}
			if t != a.token {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			touch()
			defer func() {
				if v := recover(); v != nil {
					log.Printf("PANIC in %s: %v\n%s", path, v, debug.Stack())
					jsonError(w, 500, "앱 안에서 문제가 생겼어요. 다시 시도해 주세요 (진단 정보에 남았어요)")
				}
			}()
			h(w, r)
		})
	}

	api("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "build": build, "shell": env("SHELL")})
	})
	api("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true})
		go func() { time.Sleep(300 * time.Millisecond); a.shutdownWhy("종료 버튼") }()
	})
	api("/api/clientlog", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Items []string `json:"items"`
		}
		readJSON(r, &in)
		for i, x := range in.Items {
			if i >= 30 {
				break
			}
			if len(x) > 600 {
				x = x[:600]
			}
			diag.clientErr(x)
			log.Printf("client: %s", x)
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/poll", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"build": build, "rev": a.rev.Load(), "update": a.upd.State(), "shell": env("SHELL")})
	})
	api("/api/state", a.handleState)
	api("/api/settings", a.handleSettings)

	// 계정 · 상점
	api("/api/store", a.handleStore)
	api("/api/store/refresh", a.handleStoreRefresh)
	api("/api/accounts/login", a.handleLogin)
	api("/api/accounts/remove", a.handleRemove)
	api("/api/accounts/check", a.handleAccountCheck)

	// 스킨 · 위시리스트 · 알림
	api("/api/skins", a.handleSkins)
	api("/api/wish", a.handleWish)
	api("/api/wishlist", a.handleWishlist)
	api("/api/notify/pending", a.handleNotifyPending)
	api("/api/notify/test", a.handleNotifyTest)
	api("/api/img", a.handleImg)
	api("/api/open-diag", func(w http.ResponseWriter, r *http.Request) {
		openURL(fmt.Sprintf("http://127.0.0.1:%d/diag", a.port))
		writeJSON(w, map[string]any{"ok": true})
	})

	api("/api/update/check", func(w http.ResponseWriter, r *http.Request) {
		a.upd.Check(true)
		writeJSON(w, a.upd.State())
	})
	api("/api/update/install", func(w http.ResponseWriter, r *http.Request) {
		go func() { defer recoverLog("install"); a.upd.Install() }()
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/update/rollback", func(w http.ResponseWriter, r *http.Request) {
		if err := a.upd.Rollback(); err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/update/restart", func(w http.ResponseWriter, r *http.Request) {
		if err := a.upd.Restart(a.port, a.token); err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	return mux
}

// Only answer requests addressed to 127.0.0.1 / localhost (DNS rebinding protection).
func hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"settings": a.settingsJSON(),
		"accounts": a.accounts.Views(),
		"content":  a.content.Status(),
		"update":   a.upd.State(),
		"rev":      a.rev.Load(),
	})
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var in map[string]json.RawMessage
		if err := readJSON(r, &in); err != nil {
			jsonError(w, 400, "bad request")
			return
		}
		a.mu.Lock()
		for k, v := range in {
			switch k {
			case "appearance":
				json.Unmarshal(v, &a.settings.Appearance)
			case "autoUpdate":
				json.Unmarshal(v, &a.settings.AutoUpdate)
			case "wishAlert":
				json.Unmarshal(v, &a.settings.WishAlert)
			case "nightAlert":
				json.Unmarshal(v, &a.settings.NightAlert)
			case "sound":
				json.Unmarshal(v, &a.settings.Sound)
			case "active":
				json.Unmarshal(v, &a.settings.Active)
			case "ui":
				var ui map[string]any
				if json.Unmarshal(v, &ui) == nil {
					for kk, vv := range ui {
						a.settings.UI[kk] = vv
					}
				}
			}
		}
		a.saveSettings()
		a.mu.Unlock()
		a.bump()
	}
	writeJSON(w, a.settingsJSON())
}

// ---------------------------------------------------------------- utils

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

func randID(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func writeFileAtomic(path string, b []byte, perm os.FileMode) {
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, perm) == nil {
		os.Rename(tmp, path)
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func init() {
	mime.AddExtensionType(".js", "text/javascript")
	mime.AddExtensionType(".woff2", "font/woff2")
}

func recoverLog(where string) {
	if v := recover(); v != nil {
		log.Printf("PANIC in %s: %v\n%s", where, v, debug.Stack())
	}
}
