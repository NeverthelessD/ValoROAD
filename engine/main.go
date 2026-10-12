// ValoROAD — 발로란트 오늘의 상점 · 야시장 확인 앱 (macOS)
// made by. Nevertheless_D
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
	"os/exec"
	"path/filepath"
	"runtime"
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
	Appearance  string `json:"appearance"` // system | light | dark
	Notify      bool   `json:"notify"`     // 위시리스트 알림
	DailyCheck  bool   `json:"dailyCheck"` // 매일 9시 자동 확인
	AutoUpdate  bool   `json:"autoUpdate"`
	LastAccount string `json:"lastAccount"`
}

type App struct {
	support string
	token   string
	port    int
	binDir  string

	mu       sync.Mutex
	settings Settings

	acc    *Accounts
	cat    *Catalog
	wish   *Wishlist
	stores *Stores
	login  *LoginManager
	upd    *Updater
	nlog   *notifyLog

	lastSeen atomic.Int64
}

var app *App

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--check" {
		runCheck()
		return
	}
	runEngine()
}

func runEngine() {
	log.SetFlags(log.LstdFlags)
	home, _ := os.UserHomeDir()
	support := os.Getenv("VALOROAD_SUPPORT")
	if support == "" {
		support = defaultSupportDir(home, "ValoROAD")
	}
	token := os.Getenv("VALOROAD_TOKEN")
	if token == "" {
		token = randID(16)
	}
	app = &App{support: support, token: token, binDir: filepath.Join(support, "bin")}
	os.MkdirAll(app.binDir, 0o755)
	os.MkdirAll(filepath.Join(support, "cache"), 0o755)
	os.MkdirAll(filepath.Join(support, "accounts"), 0o700)
	if f, err := os.OpenFile(filepath.Join(support, "engine.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	}
	app.loadSettings()
	app.lastSeen.Store(time.Now().Unix())
	cleanupLoginLeftovers(support)

	app.cat = NewCatalog(support)
	app.acc = NewAccounts(support)
	app.wish = NewWishlist(support)
	app.stores = NewStores(support, app.acc, app.cat, app.wish)
	app.nlog = &notifyLog{path: filepath.Join(support, "notified.json")}
	app.login = &LoginManager{acc: app.acc, onDone: func(id string) {
		app.mu.Lock()
		app.settings.LastAccount = id
		app.saveSettings()
		app.mu.Unlock()
		app.stores.dropCache(id)
		if v, err := app.stores.Refresh(id); err == nil {
			if acc := app.acc.Get(id); acc != nil {
				app.stores.NotifyWishlist(acc, v, app.nlog, app.Settings().Notify)
			}
		}
	}}
	app.upd = NewUpdater(support)

	ln := listen(os.Getenv("VALOROAD_PORT"))
	app.port = ln.Addr().(*net.TCPAddr).Port
	writeFileAtomic(filepath.Join(support, "token"), []byte(token), 0o600)
	writeFileAtomic(filepath.Join(support, "port"), []byte(strconv.Itoa(app.port)), 0o644)
	log.Printf("ValoROAD %s listening on %d", build, app.port)

	go func() {
		if err := app.syncDailyAgent(); err != nil {
			log.Printf("daily agent: %v", err)
		}
	}()
	go func() { app.cat.Ensure(false) }()
	go app.idleWatcher()
	go app.upd.autoLoop()
	go app.resetLoop()
	srv := &http.Server{Handler: hostGuard(app.routes())}
	log.Fatal(srv.Serve(ln))
}

func cleanupLoginLeftovers(support string) {
	ents, _ := os.ReadDir(filepath.Join(support, "accounts"))
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "_new-") {
			os.RemoveAll(filepath.Join(support, "accounts", e.Name()))
		}
	}
}

func defaultSupportDir(home, name string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", name)
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), name)
	default:
		return filepath.Join(home, "."+strings.ToLower(name))
	}
}

func listen(pref string) net.Listener {
	if p, err := strconv.Atoi(pref); err == nil && p > 0 {
		for i := 0; i < 20; i++ {
			if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p)); err == nil {
				return ln
			}
			time.Sleep(150 * time.Millisecond)
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	return ln
}

// ---------------------------------------------------------------- lifecycle

// 창을 닫고 10분 동안 아무 요청이 없으면 스스로 종료합니다. (매일 9시 확인은 따로 실행돼요)
func (a *App) idleWatcher() {
	for range time.Tick(15 * time.Second) {
		idle := time.Now().Unix() - a.lastSeen.Load()
		if idle > 600 && !a.login.Busy() && !a.upd.Busy() {
			a.shutdown()
		}
	}
}

func (a *App) shutdown() {
	a.login.Cancel()
	if b, err := os.ReadFile(filepath.Join(a.support, "port")); err == nil && string(b) == strconv.Itoa(a.port) {
		os.Remove(filepath.Join(a.support, "port"))
	}
	time.Sleep(200 * time.Millisecond)
	os.Exit(0)
}

// ---------------------------------------------------------------- settings

func (a *App) settingsPath() string { return filepath.Join(a.support, "settings.json") }

func (a *App) loadSettings() {
	a.settings = Settings{Appearance: "system", Notify: true, DailyCheck: true, AutoUpdate: true}
	if b, err := os.ReadFile(a.settingsPath()); err == nil {
		_ = json.Unmarshal(b, &a.settings)
	}
	if a.settings.Appearance != "light" && a.settings.Appearance != "dark" {
		a.settings.Appearance = "system"
	}
}

func (a *App) saveSettings() {
	b, _ := json.MarshalIndent(a.settings, "", "  ")
	writeFileAtomic(a.settingsPath(), b, 0o644)
}

func (a *App) Settings() Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings
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
				io.WriteString(w, "<meta charset=utf-8><body style='font:15px -apple-system;padding:40px;background:#0f1923;color:#ece8e1'>ValoROAD 앱을 다시 실행해 주세요.</body>")
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
		}
		static.ServeHTTP(w, r)
	})

	api := func(path string, h http.HandlerFunc) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("x-token") != a.token {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			a.lastSeen.Store(time.Now().Unix())
			h(w, r)
		})
	}

	api("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "build": build})
	})
	api("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true})
		go func() { time.Sleep(300 * time.Millisecond); a.shutdown() }()
	})
	api("/api/poll", a.handlePoll)
	api("/api/settings", a.handleSettings)

	api("/api/accounts/login", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Relogin string `json:"relogin"`
		}
		readJSON(r, &in)
		writeJSON(w, a.login.Start(in.Relogin))
	})
	api("/api/accounts/login/cancel", func(w http.ResponseWriter, r *http.Request) {
		a.login.Cancel()
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/accounts/login/reload", func(w http.ResponseWriter, r *http.Request) {
		a.login.Reload()
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/accounts/login/dismiss", func(w http.ResponseWriter, r *http.Request) {
		a.login.Dismiss()
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/accounts/remove", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		readJSON(r, &in)
		a.acc.Remove(in.ID)
		a.stores.dropCache(in.ID)
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/accounts/select", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		readJSON(r, &in)
		a.mu.Lock()
		a.settings.LastAccount = in.ID
		a.saveSettings()
		a.mu.Unlock()
		writeJSON(w, map[string]any{"ok": true})
	})

	api("/api/store", a.handleStore)
	api("/api/catalog", a.handleCatalog)
	api("/api/skin", a.handleSkin)
	api("/api/wishlist", a.handleWishlist)

	api("/api/notify/test", func(w http.ResponseWriter, r *http.Request) {
		notifyOS("🎯 ValoROAD 알림 테스트", "위시리스트 스킨이 상점에 뜨면 이렇게 알려 드려요.")
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/open-url", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			URL string `json:"url"`
		}
		readJSON(r, &in)
		if strings.HasPrefix(in.URL, "https://") {
			openPath(in.URL)
		}
		writeJSON(w, map[string]any{"ok": true})
	})

	api("/api/update/state", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, a.upd.State()) })
	api("/api/update/check", func(w http.ResponseWriter, r *http.Request) {
		a.upd.Check(true)
		writeJSON(w, a.upd.State())
	})
	api("/api/update/install", func(w http.ResponseWriter, r *http.Request) {
		go a.upd.Install()
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

type accountView struct {
	Account
	Busy     bool   `json:"busy"`
	Error    string `json:"error,omitempty"`
	HasStore bool   `json:"hasStore"`
	VP       int    `json:"vp"`
	RP       int    `json:"rp"`
}

func (a *App) handlePoll(w http.ResponseWriter, r *http.Request) {
	a.acc.reload()
	states, seq := a.stores.States()
	list := a.acc.List()
	sortAccounts(list)
	out := []accountView{}
	for _, x := range list {
		av := accountView{Account: x}
		if st, ok := states[x.ID]; ok {
			av.Busy, av.Error = st.Busy, st.Error
			if st.Code == "expired" {
				av.Status = "expired"
			}
		}
		if v := a.stores.Cached(x.ID); v != nil {
			av.HasStore, av.VP, av.RP = true, v.Wallet.VP, v.Wallet.RP
		}
		out = append(out, av)
	}
	hits := []map[string]any{}
	for _, x := range list {
		if v := a.stores.Cached(x.ID); v != nil && !v.Stale() {
			for _, h := range a.stores.Hits(v) {
				hits = append(hits, map[string]any{"account": x.ID, "name": h.Name, "skin": h.Skin, "where": h.Where})
			}
		}
	}
	a.cat.mu.RLock()
	catErr := a.cat.err
	a.cat.mu.RUnlock()
	b := findBrowser()
	writeJSON(w, map[string]any{
		"build":        build,
		"accounts":     out,
		"seq":          seq,
		"login":        a.login.State(),
		"catalogReady": a.cat.Ready(),
		"catalogError": catErr,
		"browser":      b,
		"update":       a.upd.State(),
		"wishCount":    len(a.wish.Items()),
		"hits":         hits,
		"now":          time.Now().UnixMilli(),
	})
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	dailyChanged := false
	if r.Method == http.MethodPost {
		var in map[string]json.RawMessage
		if err := readJSON(r, &in); err != nil {
			a.mu.Unlock()
			jsonError(w, 400, "bad request")
			return
		}
		for k, v := range in {
			switch k {
			case "appearance":
				json.Unmarshal(v, &a.settings.Appearance)
			case "notify":
				json.Unmarshal(v, &a.settings.Notify)
			case "dailyCheck":
				json.Unmarshal(v, &a.settings.DailyCheck)
				dailyChanged = true
			case "autoUpdate":
				json.Unmarshal(v, &a.settings.AutoUpdate)
			}
		}
		if a.settings.Appearance != "light" && a.settings.Appearance != "dark" {
			a.settings.Appearance = "system"
		}
		a.saveSettings()
	}
	s := a.settings
	a.mu.Unlock()
	agentErr := ""
	if dailyChanged {
		if err := a.syncDailyAgent(); err != nil {
			agentErr = err.Error()
		}
	}
	writeJSON(w, map[string]any{
		"appearance":  s.Appearance,
		"notify":      s.Notify,
		"dailyCheck":  s.DailyCheck,
		"dailyTime":   dailyCheckLabel(),
		"agentError":  agentErr,
		"autoUpdate":  s.AutoUpdate,
		"lastAccount": s.LastAccount,
		"build":       build,
		"platform":    runtime.GOOS + "/" + runtime.GOARCH,
		"maxAccounts": maxAccounts,
	})
}

func (a *App) handleStore(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if a.acc.Get(id) == nil {
		jsonError(w, 404, "계정을 찾을 수 없어요")
		return
	}
	force := r.URL.Query().Get("refresh") == "1"
	v := a.stores.Cached(id)
	if force || v.Stale() {
		nv, err := a.stores.Refresh(id)
		if err != nil {
			code := ""
			if err == errSessionExpired {
				code = "expired"
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(502)
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error(), "code": code, "cached": a.stores.WithWish(v)})
			return
		}
		if acc := a.acc.Get(id); acc != nil {
			a.stores.NotifyWishlist(acc, nv, a.nlog, a.Settings().Notify)
		}
		v = nv
	}
	writeJSON(w, map[string]any{"ok": true, "store": a.stores.WithWish(v)})
}

type skinBrief struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Weapon   string `json:"weapon"`
	WeaponID string `json:"weaponId"`
	Category string `json:"category"`
	Tier     string `json:"tier,omitempty"`
	Icon     string `json:"icon,omitempty"`
	Video    bool   `json:"video,omitempty"`
}

func (a *App) handleCatalog(w http.ResponseWriter, r *http.Request) {
	if !a.cat.Ready() {
		if err := a.cat.Ensure(false); err != nil {
			jsonError(w, 502, err.Error())
			return
		}
	}
	d := a.cat.Data()
	skins := make([]skinBrief, 0, len(d.Skins))
	for _, s := range d.Skins {
		if s.Default {
			continue
		}
		vid := false
		for _, l := range s.Levels {
			vid = vid || l.Video != ""
		}
		for _, c := range s.Chromas {
			vid = vid || c.Video != ""
		}
		skins = append(skins, skinBrief{s.UUID, s.Name, s.Weapon, s.WeaponID, s.Category, s.Tier, s.Icon, vid})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"version": d.Version, "skins": skins, "weapons": d.Weapons, "tiers": d.Tiers, "currencies": d.Currencies})
}

func (a *App) handleSkin(w http.ResponseWriter, r *http.Request) {
	a.cat.Ensure(false)
	s := a.cat.SkinByLevel(r.URL.Query().Get("id"))
	if s == nil {
		jsonError(w, 404, "스킨 정보를 찾을 수 없어요")
		return
	}
	var tier any
	if t, ok := a.cat.Data().Tiers[s.Tier]; ok {
		tier = t
	}
	writeJSON(w, map[string]any{"skin": s, "tier": tier, "wish": a.wish.Set()[s.UUID]})
}

func (a *App) handleWishlist(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var in struct {
			Skin string `json:"skin"`
			On   bool   `json:"on"`
		}
		readJSON(r, &in)
		if in.Skin != "" {
			sk := a.cat.SkinByLevel(in.Skin)
			id := in.Skin
			if sk != nil {
				id = sk.UUID
			}
			a.wish.Toggle(id, in.On)
		}
	}
	items := a.wish.Items()
	out := []map[string]any{}
	for _, it := range items {
		m := map[string]any{"skin": it.Skin, "addedAt": it.AddedAt}
		if s := a.cat.Skin(it.Skin); s != nil {
			m["name"], m["weapon"], m["icon"], m["tier"], m["category"] = s.Name, s.Weapon, s.Icon, s.Tier, s.Category
		}
		out = append(out, m)
	}
	writeJSON(w, map[string]any{"items": out})
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
	return json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(v)
}

func randID(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func writeFileAtomic(path string, b []byte, perm os.FileMode) {
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if os.WriteFile(tmp, b, perm) == nil {
		os.Rename(tmp, path)
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		s = s[i+1:]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func openPath(p string) {
	if runtime.GOOS == "darwin" {
		exec.Command("/usr/bin/open", p).Start()
	}
}

func init() {
	mime.AddExtensionType(".js", "text/javascript")
	mime.AddExtensionType(".woff2", "font/woff2")
	mime.AddExtensionType(".svg", "image/svg+xml")
}
