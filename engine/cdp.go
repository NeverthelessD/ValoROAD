package main

// Riot 로그인은 사용자의 크롬(또는 엣지·브레이브·웨일) 창에서 직접 합니다.
// 계정마다 따로 쓰는 브라우저 프로필 폴더를 만들고, 크롬 개발자 도구 프로토콜(CDP)로
// 로그인이 끝나는 순간의 주소(토큰)와 로그인 쿠키만 읽어 옵니다. 비밀번호는 앱이 볼 수 없어요.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Browser struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

var errLoginClosed = errors.New("로그인 창을 닫았어요")
var errNoBrowser = errors.New("크롬 브라우저를 찾지 못했어요")

func findBrowser() *Browser {
	if p := os.Getenv("VALOROAD_BROWSER"); p != "" {
		name := os.Getenv("VALOROAD_BROWSER_NAME")
		if name == "" {
			name = "Chromium"
		}
		return &Browser{Name: name, Path: p}
	}
	if runtime.GOOS != "darwin" {
		for _, p := range []string{"/usr/bin/google-chrome", "/usr/bin/chromium", "/usr/bin/chromium-browser"} {
			if fileExists(p) {
				return &Browser{Name: "Chrome", Path: p}
			}
		}
		return nil
	}
	home, _ := os.UserHomeDir()
	for _, b := range []struct{ app, bin, name string }{
		{"Google Chrome", "Google Chrome", "크롬"},
		{"Microsoft Edge", "Microsoft Edge", "엣지"},
		{"Brave Browser", "Brave Browser", "브레이브"},
		{"Naver Whale", "Naver Whale", "웨일"},
		{"Chromium", "Chromium", "크로미움"},
	} {
		for _, root := range []string{"/Applications", filepath.Join(home, "Applications")} {
			p := filepath.Join(root, b.app+".app", "Contents", "MacOS", b.bin)
			if fileExists(p) {
				return &Browser{Name: b.name, Path: p}
			}
		}
	}
	return nil
}

type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	Secure   bool    `json:"secure"`
	HTTPOnly bool    `json:"httpOnly"`
}

// ---------------------------------------------------------------- CDP connection

type cdpMsg struct {
	ID        int                       `json:"id,omitempty"`
	Method    string                    `json:"method,omitempty"`
	Params    json.RawMessage           `json:"params,omitempty"`
	Result    json.RawMessage           `json:"result,omitempty"`
	Error     *struct{ Message string } `json:"error,omitempty"`
	SessionID string                    `json:"sessionId,omitempty"`
}

type cdpConn struct {
	ws      *wsConn
	mu      sync.Mutex
	next    int
	pending map[int]chan cdpMsg
	events  chan cdpMsg
	done    chan struct{}
}

func newCDP(ws *wsConn) *cdpConn {
	c := &cdpConn{ws: ws, pending: map[int]chan cdpMsg{}, events: make(chan cdpMsg, 256), done: make(chan struct{})}
	go c.readLoop()
	return c
}

func (c *cdpConn) readLoop() {
	defer close(c.done)
	for {
		b, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		var m cdpMsg
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		if m.ID != 0 {
			c.mu.Lock()
			ch := c.pending[m.ID]
			delete(c.pending, m.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- m
			}
			continue
		}
		select {
		case c.events <- m:
		default:
		}
	}
}

func (c *cdpConn) Call(method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.next++
	id := c.next
	ch := make(chan cdpMsg, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	if params == nil {
		params = map[string]any{}
	}
	b, _ := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err := c.ws.WriteText(b); err != nil {
		return nil, err
	}
	select {
	case m := <-ch:
		if m.Error != nil {
			return nil, errors.New(m.Error.Message)
		}
		return m.Result, nil
	case <-c.done:
		return nil, errLoginClosed
	case <-time.After(15 * time.Second):
		return nil, fmt.Errorf("%s: no answer", method)
	}
}

// ---------------------------------------------------------------- browser process

type chromeRun struct {
	cmd  *exec.Cmd
	cdp  *cdpConn
	dir  string
	exit chan struct{}
}

func browserArgs(profileDir string, headless bool, startURL string) []string {
	args := []string{
		"--user-data-dir=" + profileDir,
		"--remote-debugging-port=0",
		"--remote-allow-origins=*",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-default-apps",
		"--disable-sync",
		"--lang=ko-KR",
	}
	if extra := os.Getenv("VALOROAD_BROWSER_ARGS"); extra != "" {
		args = append(args, strings.Fields(extra)...)
	}
	if headless {
		args = append(args, "--headless=new", "--window-size=1200,900", "--user-agent="+chromeUA, startURL)
	} else {
		args = append(args, "--window-size=520,820", "--app="+startURL)
	}
	return args
}

func startChrome(b *Browser, profileDir string, headless bool, startURL string) (*chromeRun, error) {
	if b == nil {
		return nil, errNoBrowser
	}
	os.MkdirAll(profileDir, 0o700)
	portFile := filepath.Join(profileDir, "DevToolsActivePort")
	os.Remove(portFile)
	os.Remove(filepath.Join(profileDir, "SingletonLock"))
	cmd := exec.Command(b.Path, browserArgs(profileDir, headless, startURL)...)
	cmd.Stdout, cmd.Stderr = nil, nil
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("브라우저를 열지 못했어요: %v", err)
	}
	run := &chromeRun{cmd: cmd, dir: profileDir, exit: make(chan struct{})}
	go func() { cmd.Wait(); close(run.exit) }()

	var wsURL string
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(portFile); err == nil {
			lines := strings.Split(strings.TrimSpace(string(b)), "\n")
			if len(lines) >= 2 {
				wsURL = "ws://127.0.0.1:" + strings.TrimSpace(lines[0]) + strings.TrimSpace(lines[1])
				break
			}
		}
		select {
		case <-run.exit:
			return nil, errors.New("브라우저가 바로 닫혔어요. 다시 시도해 주세요")
		case <-time.After(150 * time.Millisecond):
		}
	}
	if wsURL == "" {
		run.kill()
		return nil, errors.New("브라우저와 연결하지 못했어요")
	}
	ws, err := wsDial(wsURL, 10*time.Second)
	if err != nil {
		run.kill()
		return nil, fmt.Errorf("브라우저와 연결하지 못했어요: %v", err)
	}
	run.cdp = newCDP(ws)
	return run, nil
}

func (r *chromeRun) close() {
	if r.cdp != nil {
		go r.cdp.Call("Browser.close", nil)
		select {
		case <-r.exit:
		case <-time.After(4 * time.Second):
		}
		r.cdp.ws.Close()
	}
	r.kill()
}

func (r *chromeRun) kill() {
	select {
	case <-r.exit:
		return
	default:
	}
	killProcessGroup(r.cmd)
	select {
	case <-r.exit:
	case <-time.After(3 * time.Second):
	}
}

func (r *chromeRun) cookies() ([]Cookie, error) {
	res, err := r.cdp.Call("Storage.getCookies", nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Cookies []Cookie `json:"cookies"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		return nil, err
	}
	keep := []Cookie{}
	for _, c := range out.Cookies {
		if cookieForRiot(c.Domain) {
			keep = append(keep, c)
		}
	}
	return keep, nil
}

type targetInfo struct {
	TargetID string `json:"targetId"`
	Type     string `json:"type"`
	URL      string `json:"url"`
}

func (r *chromeRun) pageTargets() []targetInfo {
	res, err := r.cdp.Call("Target.getTargets", nil)
	if err != nil {
		return nil
	}
	var out struct {
		TargetInfos []targetInfo `json:"targetInfos"`
	}
	json.Unmarshal(res, &out)
	pages := []targetInfo{}
	for _, t := range out.TargetInfos {
		if t.Type == "page" {
			pages = append(pages, t)
		}
	}
	return pages
}

// ---------------------------------------------------------------- login / re-login

type authResult struct {
	AccessToken string
	IDToken     string
	ExpiresIn   int
	Cookies     []Cookie
}

// parseTokenURL reads the #access_token=…&id_token=… fragment Riot redirects to.
func parseTokenURL(u string) (*authResult, bool) {
	if !strings.HasPrefix(u, riot.redirectURI) {
		return nil, false
	}
	i := strings.Index(u, "#")
	if i < 0 {
		return nil, false
	}
	v, err := url.ParseQuery(u[i+1:])
	if err != nil || v.Get("access_token") == "" {
		return nil, false
	}
	exp := 3600
	fmt.Sscan(v.Get("expires_in"), &exp)
	return &authResult{AccessToken: v.Get("access_token"), IDToken: v.Get("id_token"), ExpiresIn: exp}, true
}

func isLoginPage(u string) bool {
	return strings.Contains(u, riot.loginMarker)
}

// chromeAuth opens the Riot sign-in flow in the given profile and waits until Riot hands out a token.
// visible: a normal app window the person signs in to. headless: silent re-login using the saved profile.
func chromeAuth(ctx context.Context, b *Browser, profileDir string, headless bool, onStage func(string)) (*authResult, error) {
	start := riot.authorizeURL()
	if headless {
		start = "about:blank"
	}
	run, err := startChrome(b, profileDir, headless, start)
	if err != nil {
		return nil, err
	}
	defer run.close()
	c := run.cdp
	if _, err := c.Call("Target.setDiscoverTargets", map[string]any{"discover": true}); err != nil {
		return nil, err
	}
	if headless {
		if _, err := c.Call("Target.createTarget", map[string]any{"url": riot.authorizeURL()}); err != nil {
			return nil, err
		}
	}

	finish := func(u string) (*authResult, error) {
		res, _ := parseTokenURL(u)
		cs, err := run.cookies()
		if err != nil {
			log.Printf("cookies: %v", err)
		}
		res.Cookies = cs
		return res, nil
	}
	// 이미 토큰 주소에 도착했을 수도 있어요
	for _, t := range run.pageTargets() {
		if _, ok := parseTokenURL(t.URL); ok {
			return finish(t.URL)
		}
	}

	var loginSince time.Time
	tick := time.NewTicker(700 * time.Millisecond)
	defer tick.Stop()
	sawPage := false
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-run.exit:
			return nil, errLoginClosed
		case <-c.done:
			return nil, errLoginClosed
		case m := <-c.events:
			switch m.Method {
			case "Target.targetCreated", "Target.targetInfoChanged":
				var p struct {
					TargetInfo targetInfo `json:"targetInfo"`
				}
				json.Unmarshal(m.Params, &p)
				if p.TargetInfo.Type != "page" {
					continue
				}
				sawPage = true
				u := p.TargetInfo.URL
				if _, ok := parseTokenURL(u); ok {
					if onStage != nil {
						onStage("finishing")
					}
					return finish(u)
				}
				if isLoginPage(u) {
					if loginSince.IsZero() {
						loginSince = time.Now()
					}
					if onStage != nil {
						onStage("login")
					}
				} else {
					loginSince = time.Time{}
				}
			case "Target.targetDestroyed":
				if !headless && len(run.pageTargets()) == 0 {
					return nil, errLoginClosed
				}
			}
		case <-tick.C:
			if headless && !loginSince.IsZero() && time.Since(loginSince) > 2*time.Second {
				return nil, errSessionExpired
			}
			if !headless && sawPage && len(run.pageTargets()) == 0 {
				return nil, errLoginClosed
			}
		}
	}
}
