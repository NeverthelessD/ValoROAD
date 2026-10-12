package main

// 계정 3개까지 저장 · 자동 로그인 유지.
// 비밀번호는 저장하지 않아요. Riot이 준 로그인 쿠키와 토큰만 macOS 키체인에 보관합니다.

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxAccounts = 3
const keychainService = "ValoROAD"

type Account struct {
	ID          string `json:"id"` // PUUID
	Name        string `json:"name"`
	Tag         string `json:"tag"`
	Region      string `json:"region"`
	Shard       string `json:"shard"`
	Level       int    `json:"level"`
	Card        string `json:"card,omitempty"`
	AddedAt     int64  `json:"addedAt"`
	Status      string `json:"status"` // ok | expired | error
	LastError   string `json:"lastError,omitempty"`
	LastRefresh int64  `json:"lastRefresh,omitempty"`
}

type secretBlob struct {
	Cookies []Cookie `json:"cookies"`
}

type Accounts struct {
	support string
	mu      sync.Mutex
	list    []*Account
	locks   map[string]*sync.Mutex
	mem     map[string]*riotSession
	browser func() *Browser
}

func NewAccounts(support string) *Accounts {
	a := &Accounts{support: support, locks: map[string]*sync.Mutex{}, mem: map[string]*riotSession{}, browser: findBrowser}
	a.load()
	return a
}

func (a *Accounts) path() string { return filepath.Join(a.support, "accounts.json") }

func (a *Accounts) load() {
	b, err := os.ReadFile(a.path())
	if err != nil {
		return
	}
	var list []*Account
	if json.Unmarshal(b, &list) == nil {
		a.list = list
	}
}

func (a *Accounts) saveLocked() {
	b, _ := json.MarshalIndent(a.list, "", "  ")
	writeFileAtomic(a.path(), b, 0o600)
}

// reload picks up changes another ValoROAD process (the 9시 check) wrote.
func (a *Accounts) reload() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.load()
}

func (a *Accounts) List() []Account {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Account, 0, len(a.list))
	for _, x := range a.list {
		out = append(out, *x)
	}
	return out
}

func (a *Accounts) Get(id string) *Account {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, x := range a.list {
		if x.ID == id {
			c := *x
			return &c
		}
	}
	return nil
}

func (a *Accounts) update(id string, fn func(*Account)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, x := range a.list {
		if x.ID == id {
			fn(x)
			a.saveLocked()
			return
		}
	}
}

func (a *Accounts) lock(id string) *sync.Mutex {
	a.mu.Lock()
	defer a.mu.Unlock()
	l := a.locks[id]
	if l == nil {
		l = &sync.Mutex{}
		a.locks[id] = l
	}
	return l
}

func (a *Accounts) accountDir(id string) string { return filepath.Join(a.support, "accounts", id) }
func (a *Accounts) profileDir(id string) string { return filepath.Join(a.accountDir(id), "browser") }

func (a *Accounts) Remove(id string) {
	a.mu.Lock()
	keep := []*Account{}
	for _, x := range a.list {
		if x.ID != id {
			keep = append(keep, x)
		}
	}
	a.list = keep
	delete(a.mem, id)
	a.saveLocked()
	a.mu.Unlock()
	deleteSecret(a.support, id)
	os.RemoveAll(a.accountDir(id))
	os.Remove(filepath.Join(a.support, "cache", "store-"+id+".json"))
}

// ---------------------------------------------------------------- secrets (키체인)
// 오래 쓰는 로그인 쿠키는 macOS 키체인에, 1시간짜리 토큰은 앱 폴더(나만 읽을 수 있는 파일)에 둡니다.

func secretFile(support, id string) string {
	return filepath.Join(support, "accounts", id, "secret.json")
}

func sessionFile(support, id string) string {
	return filepath.Join(support, "accounts", id, "session.json")
}

func useKeychain() bool {
	return runtime.GOOS == "darwin" && os.Getenv("VALOROAD_NO_KEYCHAIN") == ""
}

func keychainRead(id string) ([]byte, bool) {
	out, err := exec.Command("/usr/bin/security", "find-generic-password", "-a", id, "-s", keychainService, "-w").Output()
	if err != nil {
		return nil, false
	}
	txt := strings.TrimSpace(string(out))
	if !strings.HasPrefix(txt, "{") && !strings.HasPrefix(txt, "[") {
		if h, err := hex.DecodeString(txt); err == nil {
			txt = string(h)
		}
	}
	return []byte(txt), true
}

func keychainWrite(id string, b []byte) bool {
	hx := hex.EncodeToString(b)
	// 1) 표준 입력으로 전달 (다른 프로그램이 볼 수 없게)
	cmd := exec.Command("/usr/bin/security", "-i")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -a %s -s %s -l %s -X %s\n", id, keychainService, keychainService, hx))
	cmd.Run()
	if got, ok := keychainRead(id); ok && string(got) == string(b) {
		return true
	}
	// 2) 안 되면 일반 방식
	exec.Command("/usr/bin/security", "add-generic-password", "-U", "-a", id, "-s", keychainService, "-l", keychainService, "-X", hx).Run()
	got, ok := keychainRead(id)
	return ok && string(got) == string(b)
}

func saveCookies(support, id string, cookies []Cookie) {
	b, _ := json.Marshal(&secretBlob{Cookies: cookies})
	if useKeychain() {
		if keychainWrite(id, b) {
			os.Remove(secretFile(support, id))
			return
		}
		log.Printf("keychain save failed — using file")
	}
	p := secretFile(support, id)
	os.MkdirAll(filepath.Dir(p), 0o700)
	writeFileAtomic(p, b, 0o600)
}

func loadCookies(support, id string) ([]Cookie, error) {
	var raw []byte
	if useKeychain() {
		if b, ok := keychainRead(id); ok {
			raw = b
		}
	}
	if raw == nil {
		b, err := os.ReadFile(secretFile(support, id))
		if err != nil {
			return nil, errSessionExpired
		}
		raw = b
	}
	var sb secretBlob
	if err := json.Unmarshal(raw, &sb); err != nil || len(sb.Cookies) == 0 {
		return nil, errSessionExpired
	}
	return sb.Cookies, nil
}

func saveSessionFile(support, id string, s *riotSession) {
	p := sessionFile(support, id)
	os.MkdirAll(filepath.Dir(p), 0o700)
	b, _ := json.Marshal(s)
	writeFileAtomic(p, b, 0o600)
}

func loadSessionFile(support, id string) *riotSession {
	b, err := os.ReadFile(sessionFile(support, id))
	if err != nil {
		return nil
	}
	var s riotSession
	if json.Unmarshal(b, &s) != nil {
		return nil
	}
	return &s
}

func deleteSecret(support, id string) {
	if runtime.GOOS == "darwin" {
		exec.Command("/usr/bin/security", "delete-generic-password", "-a", id, "-s", keychainService).Run()
	}
	os.Remove(secretFile(support, id))
	os.Remove(sessionFile(support, id))
}

// ---------------------------------------------------------------- sessions

// Session returns a working Riot session for the account, signing in again silently when needed.
func (a *Accounts) Session(id string, forceNew bool) (*riotSession, error) {
	l := a.lock(id)
	l.Lock()
	defer l.Unlock()
	acc := a.Get(id)
	if acc == nil {
		return nil, errors.New("계정을 찾을 수 없어요")
	}
	a.mu.Lock()
	s := a.mem[id]
	a.mu.Unlock()
	if !forceNew && s.valid() {
		return s, nil
	}
	if fs := loadSessionFile(a.support, id); !forceNew && fs.valid() && fs.PUUID == id {
		a.remember(id, fs)
		return fs, nil
	}
	saved, err := loadCookies(a.support, id)
	if err != nil {
		a.markExpired(id)
		return nil, errSessionExpired
	}
	res, cookies, err := cookieReauth(saved)
	if errors.Is(err, errBlocked) {
		// 쿠키만으로 안 되면 저장된 브라우저 프로필로 조용히 다시 로그인
		log.Printf("[%s] trying silent browser re-login", acc.Name)
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		res, err = chromeAuth(ctx, a.browser(), a.profileDir(id), true, nil)
		cancel()
		if err == nil && len(res.Cookies) > 0 {
			cookies = res.Cookies
		}
		if err != nil && !errors.Is(err, errSessionExpired) && !errors.Is(err, errNoBrowser) {
			log.Printf("silent re-login: %v", err)
			err = errors.New("Riot 서버가 자동 로그인을 받아 주지 않았어요. 잠시 뒤 다시 시도해 주세요")
		}
	}
	if errors.Is(err, errSessionExpired) || errors.Is(err, errNoBrowser) {
		a.markExpired(id)
		return nil, errSessionExpired
	}
	if err != nil {
		return nil, err
	}
	ns, err := a.completeSession(res, acc.Shard, acc.Region)
	if err != nil {
		return nil, err
	}
	if ns.PUUID != id {
		return nil, errors.New("다른 계정으로 로그인되어 있어요. 이 계정으로 다시 로그인해 주세요")
	}
	saveCookies(a.support, id, cookies)
	saveSessionFile(a.support, id, ns)
	a.remember(id, ns)
	if acc.Status != "ok" {
		a.update(id, func(x *Account) { x.Status, x.LastError = "ok", "" })
	}
	return ns, nil
}

func (a *Accounts) remember(id string, s *riotSession) {
	a.mu.Lock()
	a.mem[id] = s
	a.mu.Unlock()
}

func (a *Accounts) markExpired(id string) {
	a.mu.Lock()
	delete(a.mem, id)
	a.mu.Unlock()
	a.update(id, func(x *Account) { x.Status, x.LastError = "expired", "로그인이 만료됐어요" })
}

// completeSession turns fresh tokens into a full session (entitlements, PUUID, shard).
func (a *Accounts) completeSession(res *authResult, shard, region string) (*riotSession, error) {
	ent, err := fetchEntitlement(res.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("인증 토큰을 받지 못했어요: %v", err)
	}
	ui, err := fetchUserInfo(res.AccessToken)
	if err != nil {
		return nil, err
	}
	if region == "" {
		if r, err := fetchRegion(res.AccessToken, res.IDToken); err == nil {
			region = r
		} else {
			log.Printf("region: %v", err)
			region = "kr"
		}
		shard = regionToShard(region)
	}
	exp := res.ExpiresIn
	if exp <= 0 {
		exp = 3600
	}
	return &riotSession{Access: res.AccessToken, IDToken: res.IDToken, Ent: ent, PUUID: strings.ToLower(ui.Sub),
		Shard: shard, Region: region, Expires: time.Now().Add(time.Duration(exp) * time.Second)}, nil
}

// ---------------------------------------------------------------- 로그인 창

type LoginState struct {
	ID        string `json:"id"`
	Stage     string `json:"stage"` // opening | waiting | login | finishing | done | error | cancelled
	Error     string `json:"error,omitempty"`
	Code      string `json:"code,omitempty"` // nobrowser | full
	AccountID string `json:"accountId,omitempty"`
	Relogin   string `json:"relogin,omitempty"`
	Browser   string `json:"browser,omitempty"`
	StartedAt int64  `json:"startedAt"`
}

type LoginManager struct {
	mu     sync.Mutex
	state  *LoginState
	cancel context.CancelFunc
	acc    *Accounts
	onDone func(id string)
}

func (m *LoginManager) State() *LoginState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == nil {
		return nil
	}
	c := *m.state
	return &c
}

func (m *LoginManager) Busy() bool {
	st := m.State()
	return st != nil && (st.Stage == "opening" || st.Stage == "waiting" || st.Stage == "login" || st.Stage == "finishing")
}

func (m *LoginManager) set(fn func(*LoginState)) {
	m.mu.Lock()
	if m.state != nil {
		fn(m.state)
	}
	m.mu.Unlock()
}

func (m *LoginManager) Start(relogin string) *LoginState {
	if m.Busy() {
		return m.State()
	}
	st := &LoginState{ID: randID(6), Stage: "opening", Relogin: relogin, StartedAt: time.Now().UnixMilli()}
	b := m.acc.browser()
	if b == nil {
		st.Stage, st.Code, st.Error = "error", "nobrowser", "로그인 창을 열 크롬 브라우저가 필요해요"
		m.mu.Lock()
		m.state = st
		m.mu.Unlock()
		return st
	}
	st.Browser = b.Name
	if relogin == "" && len(m.acc.List()) >= maxAccounts {
		st.Stage, st.Code, st.Error = "error", "full", "계정은 3개까지 저장할 수 있어요. 하나를 지운 뒤 추가해 주세요"
		m.mu.Lock()
		m.state = st
		m.mu.Unlock()
		return st
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	m.mu.Lock()
	m.state, m.cancel = st, cancel
	m.mu.Unlock()
	go m.run(ctx, st.ID, b, relogin)
	return m.State()
}

func (m *LoginManager) Cancel() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	if m.state != nil && m.state.Stage != "done" && m.state.Stage != "error" {
		m.state.Stage = "cancelled"
	}
	m.mu.Unlock()
}

func (m *LoginManager) Dismiss() {
	m.mu.Lock()
	if m.state != nil && m.state.Stage != "opening" && m.state.Stage != "waiting" && m.state.Stage != "login" && m.state.Stage != "finishing" {
		m.state = nil
	}
	m.mu.Unlock()
}

func (m *LoginManager) run(ctx context.Context, sid string, b *Browser, relogin string) {
	defer func() {
		m.mu.Lock()
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
		}
		m.mu.Unlock()
	}()
	a := m.acc
	profile := filepath.Join(a.support, "accounts", "_new-"+sid, "browser")
	if relogin != "" {
		profile = a.profileDir(relogin)
	}
	m.set(func(s *LoginState) { s.Stage = "waiting" })
	res, err := chromeAuth(ctx, b, profile, false, func(stage string) {
		m.set(func(s *LoginState) {
			if s.Stage != "cancelled" {
				s.Stage = stage
			}
		})
	})
	fail := func(msg string) {
		m.set(func(s *LoginState) {
			if s.Stage != "cancelled" {
				s.Stage, s.Error = "error", msg
			}
		})
		if relogin == "" {
			os.RemoveAll(filepath.Dir(profile))
		}
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, errLoginClosed) {
			m.set(func(s *LoginState) {
				if s.Stage != "error" {
					s.Stage = "cancelled"
				}
			})
			if relogin == "" {
				os.RemoveAll(filepath.Dir(profile))
			}
			return
		}
		if errors.Is(err, context.DeadlineExceeded) {
			fail("시간이 너무 오래 걸려서 로그인을 멈췄어요. 다시 시도해 주세요")
			return
		}
		fail(err.Error())
		return
	}
	m.set(func(s *LoginState) { s.Stage = "finishing" })
	old := (*Account)(nil)
	if relogin != "" {
		old = a.Get(relogin)
	}
	shard, region := "", ""
	if old != nil {
		shard, region = old.Shard, old.Region
	}
	sess, err := a.completeSession(res, shard, region)
	if err != nil {
		fail(err.Error())
		return
	}
	id := sess.PUUID
	ui, _ := fetchUserInfo(res.AccessToken)

	a.mu.Lock()
	var existing *Account
	for _, x := range a.list {
		if x.ID == id {
			existing = x
		}
	}
	if existing == nil && len(a.list) >= maxAccounts {
		a.mu.Unlock()
		fail("계정은 3개까지 저장할 수 있어요. 하나를 지운 뒤 추가해 주세요")
		return
	}
	if existing == nil {
		existing = &Account{ID: id, AddedAt: time.Now().UnixMilli()}
		a.list = append(a.list, existing)
	}
	if ui != nil {
		existing.Name, existing.Tag = ui.Acct.GameName, ui.Acct.TagLine
	}
	existing.Region, existing.Shard = sess.Region, sess.Shard
	existing.Status, existing.LastError = "ok", ""
	a.mem[id] = sess
	a.saveLocked()
	a.mu.Unlock()

	// 브라우저 프로필을 계정 폴더로 옮겨서 다음부터 자동 로그인에 씁니다
	if relogin == "" || relogin != id {
		dst := a.profileDir(id)
		os.MkdirAll(filepath.Dir(dst), 0o700)
		os.RemoveAll(dst)
		if err := os.Rename(profile, dst); err != nil {
			log.Printf("move profile: %v", err)
		}
		if relogin == "" {
			os.RemoveAll(filepath.Dir(profile))
		}
	}
	saveCookies(a.support, id, res.Cookies)
	saveSessionFile(a.support, id, sess)
	m.set(func(s *LoginState) { s.Stage, s.AccountID = "done", id })
	if m.onDone != nil {
		go m.onDone(id)
	}
}

// sortAccounts keeps the order the person added them in.
func sortAccounts(list []Account) {
	sort.SliceStable(list, func(i, j int) bool { return list[i].AddedAt < list[j].AddedAt })
}
