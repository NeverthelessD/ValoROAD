package main

// 계정 (최대 3개) · 자동 로그인 · 계정별 상점
//
// - 계정 정보(이름 · 태그 · 지역)는 Support/accounts.json, 로그인 열쇠(쿠키)는 키체인, 상점은 Support/stores/<id>.json
// - 토큰은 메모리에만 두고, 1시간마다 쿠키로 새로 받아요(reauth). 쿠키가 만료되면 그 계정만 '다시 로그인'.
// - 상점은 계정을 고를 때 · 매일 상점이 바뀔 때(한국 오전 9시) · 새로고침 버튼으로 다시 받아요.

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxAccounts = 3

type Account struct {
	ID     string `json:"id"`
	Slot   int    `json:"slot"`
	PUUID  string `json:"puuid"`
	Name   string `json:"name"`
	Tag    string `json:"tag"`
	Region string `json:"region"`
	Shard  string `json:"shard"`
	Added  int64  `json:"added"`
	LastOK int64  `json:"lastOk"`
	Status string `json:"status"` // ok | expired | error
	Error  string `json:"error"`
}

type RawOffer struct {
	Item  string `json:"item"` // 스킨 레벨 uuid
	Price int    `json:"price"`
	Base  int    `json:"base,omitempty"`
	Pct   int    `json:"pct,omitempty"`
	Seen  bool   `json:"seen,omitempty"`
}

type RawBundle struct {
	ID    string `json:"id"` // 번들 정보 uuid (DataAssetID)
	Price int    `json:"price"`
	Base  int    `json:"base"`
	Ends  int64  `json:"ends"`
	Items int    `json:"items"`
}

type StoreData struct {
	Fetched   int64          `json:"fetched"`
	Daily     []RawOffer     `json:"daily"`
	DailyEnds int64          `json:"dailyEnds"`
	Night     []RawOffer     `json:"night"`
	NightEnds int64          `json:"nightEnds"`
	Bundles   []RawBundle    `json:"bundles"`
	Wallet    map[string]int `json:"wallet"`
	Seen      map[string]int `json:"seen,omitempty"` // 이 상점에서 본 아이템의 원래 가격 (레벨 uuid → VP)
}

// seenPrices: 상점 · 밤시장 · 번들에서 본 원래 가격들 (라이엇 가격표를 못 받았을 때도 쓸 수 있게)
func seenPrices(st *StoreData) map[string]int {
	m := map[string]int{}
	for k, v := range st.Seen {
		m[k] = v
	}
	for _, o := range st.Daily {
		if o.Price > 0 {
			m[o.Item] = o.Price
		}
	}
	for _, o := range st.Night {
		if o.Base > 0 {
			m[o.Item] = o.Base
		}
	}
	return m
}

type Accounts struct {
	mu       sync.Mutex
	support  string
	content  *Content
	secrets  SecretStore
	list     []*Account
	sessions map[string]*session
	stores   map[string]*StoreData
	busy     map[string]bool
	nextTry  map[string]time.Time
	locks    map[string]*sync.Mutex
}

func NewAccounts(support string, c *Content) *Accounts {
	a := &Accounts{support: support, content: c, secrets: newSecretStore(support),
		sessions: map[string]*session{}, stores: map[string]*StoreData{}, busy: map[string]bool{},
		nextTry: map[string]time.Time{}, locks: map[string]*sync.Mutex{}}
	if demoMode() {
		a.list = demoAccounts()
		return a
	}
	if b, err := os.ReadFile(a.path()); err == nil {
		json.Unmarshal(b, &a.list)
	}
	for _, acc := range a.list {
		if b, err := os.ReadFile(a.storePath(acc.ID)); err == nil {
			var st StoreData
			if json.Unmarshal(b, &st) == nil {
				a.stores[acc.ID] = &st
				if app != nil && app.prices != nil {
					app.prices.Merge(seenPrices(&st))
				}
			}
		}
	}
	a.sortList()
	return a
}

func (a *Accounts) path() string               { return filepath.Join(a.support, "accounts.json") }
func (a *Accounts) storePath(id string) string { return filepath.Join(a.support, "stores", id+".json") }

func (a *Accounts) sortList() {
	sort.Slice(a.list, func(i, j int) bool { return a.list[i].Slot < a.list[j].Slot })
}

// save: a.mu 를 잡은 상태에서 불러요
func (a *Accounts) save() {
	if demoMode() {
		return
	}
	b, _ := json.MarshalIndent(a.list, "", "  ")
	writeFileAtomic(a.path(), b, 0o600)
}

func (a *Accounts) get(id string) *Account {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, acc := range a.list {
		if acc.ID == id {
			c := *acc
			return &c
		}
	}
	return nil
}

func (a *Accounts) update(id string, fn func(*Account)) {
	a.mu.Lock()
	for _, acc := range a.list {
		if acc.ID == id {
			fn(acc)
		}
	}
	a.save()
	a.mu.Unlock()
	if app != nil {
		app.bump()
	}
}

func (a *Accounts) lockFor(id string) *sync.Mutex {
	a.mu.Lock()
	defer a.mu.Unlock()
	l, ok := a.locks[id]
	if !ok {
		l = &sync.Mutex{}
		a.locks[id] = l
	}
	return l
}

func (a *Accounts) Store(id string) *StoreData {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stores[id]
}

func (a *Accounts) IDs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []string{}
	for _, acc := range a.list {
		out = append(out, acc.ID)
	}
	return out
}

// ---------------------------------------------------------------- 화면용

type HitView struct {
	Skin  string `json:"skin"`
	Name  string `json:"name"`
	Kind  string `json:"kind"` // daily | night
	Price int    `json:"price"`
	Base  int    `json:"base,omitempty"`
}

type AccountView struct {
	ID       string         `json:"id"`
	Slot     int            `json:"slot"`
	Name     string         `json:"name"`
	Tag      string         `json:"tag"`
	Region   string         `json:"region"`
	Status   string         `json:"status"`
	Error    string         `json:"error"`
	Loading  bool           `json:"loading"`
	Wallet   map[string]int `json:"wallet"`
	Hits     []HitView      `json:"hits"`
	StoreAge string         `json:"-"`
}

func (a *Accounts) Views() []AccountView {
	wish := map[string]bool{}
	if app != nil {
		wish = app.wishSet()
	}
	nightOK := app == nil || app.Settings().NightAlert
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []AccountView{}
	now := time.Now().Unix()
	for _, acc := range a.list {
		v := AccountView{ID: acc.ID, Slot: acc.Slot, Name: acc.Name, Tag: acc.Tag, Region: strings.ToUpper(acc.Region),
			Status: acc.Status, Error: acc.Error, Loading: a.busy[acc.ID], Hits: []HitView{}, StoreAge: "없음"}
		if st := a.stores[acc.ID]; st != nil {
			v.Wallet = st.Wallet
			v.StoreAge = time.Since(time.Unix(st.Fetched, 0)).Round(time.Second).String() + " 전"
			if st.DailyEnds > now {
				for _, o := range st.Daily {
					if s := a.content.SkinByItem(o.Item); s != nil && wish[s.ID] {
						v.Hits = append(v.Hits, HitView{Skin: s.ID, Name: s.Name, Kind: "daily", Price: o.Price})
					}
				}
			}
			if nightOK && st.NightEnds > now {
				for _, o := range st.Night {
					if s := a.content.SkinByItem(o.Item); s != nil && wish[s.ID] {
						v.Hits = append(v.Hits, HitView{Skin: s.ID, Name: s.Name, Kind: "night", Price: o.Price, Base: o.Base})
					}
				}
			}
		}
		out = append(out, v)
	}
	return out
}

type OfferView struct {
	Skin
	Item   string `json:"item"`
	Price  int    `json:"price"`
	Base   int    `json:"base,omitempty"`
	Pct    int    `json:"pct,omitempty"`
	Wished bool   `json:"wished"`
	Known  bool   `json:"known"`
}

type BundleView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Img   string `json:"img"`
	Price int    `json:"price"`
	Base  int    `json:"base"`
	Ends  int64  `json:"ends"`
	Items int    `json:"items"`
}

func (a *Accounts) offerViews(list []RawOffer, wish map[string]bool) []OfferView {
	out := []OfferView{}
	for _, o := range list {
		v := OfferView{Item: o.Item, Price: o.Price, Base: o.Base, Pct: o.Pct}
		if s := a.content.SkinByItem(o.Item); s != nil {
			v.Skin, v.Known, v.Wished = *s, true, wish[s.ID]
		} else {
			v.Skin = Skin{ID: o.Item, Name: "스킨 정보를 불러오는 중", Cat: "other"}
		}
		out = append(out, v)
	}
	return out
}

func (a *Accounts) StoreView(id string) map[string]any {
	st := a.Store(id)
	if st == nil {
		return nil
	}
	wish := app.wishSet()
	now := time.Now().Unix()
	out := map[string]any{
		"fetched":   st.Fetched,
		"dailyEnds": st.DailyEnds,
		"nightEnds": st.NightEnds,
		"daily":     a.offerViews(st.Daily, wish),
		"night":     []OfferView{},
		"bundles":   []BundleView{},
		"wallet":    st.Wallet,
		"stale":     st.DailyEnds <= now,
	}
	if st.NightEnds > now {
		out["night"] = a.offerViews(st.Night, wish)
	}
	bundles := []BundleView{}
	for _, b := range st.Bundles {
		if b.Ends > 0 && b.Ends <= now {
			continue
		}
		info, _ := a.content.Bundle(b.ID)
		name := info.Name
		if name == "" {
			name = "추천 번들"
		}
		bundles = append(bundles, BundleView{ID: b.ID, Name: name, Img: info.Img, Price: b.Price, Base: b.Base, Ends: b.Ends, Items: b.Items})
	}
	out["bundles"] = bundles
	return out
}

// ---------------------------------------------------------------- 로그인 열쇠

func (a *Accounts) cookies(id string) ([]Cookie, error) {
	s, err := a.secrets.Get(id)
	if err != nil {
		return nil, err
	}
	var c []Cookie
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return nil, errors.New("로그인 정보를 읽지 못했어요")
	}
	return c, nil
}

func (a *Accounts) putCookies(id string, c []Cookie) error {
	b, _ := json.Marshal(c)
	return a.secrets.Set(id, string(b))
}

// ensureSession: 토큰이 1시간보다 오래됐으면 쿠키로 새로 받아요
func (a *Accounts) ensureSession(acc *Account, force bool) (*session, error) {
	a.mu.Lock()
	s := a.sessions[acc.ID]
	a.mu.Unlock()
	if !force && s.valid() {
		return s, nil
	}
	ck, err := a.cookies(acc.ID)
	if err != nil {
		return nil, errExpired
	}
	t, merged, err := reauth(ck)
	if len(merged) > 0 {
		if perr := a.putCookies(acc.ID, merged); perr != nil {
			log.Printf("account %d: 쿠키 저장 실패: %v", acc.Slot, perr)
		}
	}
	if err != nil {
		return nil, err
	}
	ent, err := fetchEntitlements(t.Access)
	if err != nil {
		return nil, err
	}
	s = &session{Access: t.Access, ID: t.ID, Ent: ent, Expires: t.Expires}
	a.mu.Lock()
	a.sessions[acc.ID] = s
	a.mu.Unlock()
	return s, nil
}

// ---------------------------------------------------------------- 로그인 · 지우기

type LoginInput struct {
	Slot        int      `json:"slot"`
	AccessToken string   `json:"accessToken"`
	IDToken     string   `json:"idToken"`
	Cookies     []Cookie `json:"cookies"`
}

func (a *Accounts) Login(in LoginInput) (*Account, string, error) {
	if demoMode() {
		return nil, "", errors.New("시험 모드에서는 로그인할 수 없어요")
	}
	if in.AccessToken == "" {
		return nil, "", errors.New("로그인 정보가 비어 있어요")
	}
	ck := onlyRiotCookies(in.Cookies)
	info, err := fetchUserInfo(in.AccessToken)
	if err != nil {
		return nil, "", err
	}
	region, err := fetchRegion(in.AccessToken, in.IDToken)
	if err != nil {
		log.Printf("login: 지역 확인 실패(%v) → kr 로 가정", err)
		region = "kr"
	}
	ent, err := fetchEntitlements(in.AccessToken)
	if err != nil {
		return nil, "", err
	}
	note := ""
	a.mu.Lock()
	var acc *Account
	var replaced *Account
	for _, x := range a.list {
		if x.PUUID == info.PUUID {
			acc = x
		}
	}
	if acc != nil {
		if in.Slot != 0 && in.Slot != acc.Slot {
			note = fmt.Sprintf("이미 %02d번에 저장된 계정이라 그 자리를 새로 고쳤어요", acc.Slot)
		}
	} else {
		slot := in.Slot
		if slot < 1 || slot > maxAccounts {
			slot = 0
			used := map[int]bool{}
			for _, x := range a.list {
				used[x.Slot] = true
			}
			for i := 1; i <= maxAccounts; i++ {
				if !used[i] {
					slot = i
					break
				}
			}
			if slot == 0 {
				a.mu.Unlock()
				return nil, "", errors.New("계정 자리 3개가 모두 찼어요. 설정에서 한 자리를 비운 뒤 추가해 주세요")
			}
		}
		keep := a.list[:0]
		for _, x := range a.list {
			if x.Slot == slot {
				replaced = x
				continue
			}
			keep = append(keep, x)
		}
		a.list = keep
		acc = &Account{ID: randID(8), Slot: slot, Added: time.Now().Unix()}
		a.list = append(a.list, acc)
		a.sortList()
	}
	acc.PUUID, acc.Name, acc.Tag, acc.Region, acc.Shard = info.PUUID, info.Name, info.Tag, region, shardFor(region)
	acc.Status, acc.Error, acc.LastOK = "ok", "", time.Now().Unix()
	a.sessions[acc.ID] = &session{Access: in.AccessToken, ID: in.IDToken, Ent: ent, Expires: time.Now().Add(55 * time.Minute)}
	if replaced != nil {
		delete(a.stores, replaced.ID)
		delete(a.sessions, replaced.ID)
	}
	a.save()
	cp := *acc
	a.mu.Unlock()

	if replaced != nil {
		a.secrets.Delete(replaced.ID)
		os.Remove(a.storePath(replaced.ID))
	}
	if err := a.putCookies(cp.ID, ck); err != nil {
		log.Printf("login: %v", err)
		note = "자동 로그인 정보를 저장하지 못했어요. 앱을 다시 켜면 다시 로그인해야 할 수 있어요"
	}
	hasSSID := false
	for _, c := range ck {
		if c.Name == "ssid" {
			hasSSID = true
		}
	}
	log.Printf("login: 슬롯 %d · 지역 %s · 쿠키 %d개 (ssid %v)", cp.Slot, region, len(ck), hasSSID)
	if !hasSSID && note == "" {
		note = "‘로그인 상태 유지’가 꺼져 있었던 것 같아요. 다음에 앱을 켜면 다시 로그인해야 할 수 있어요"
	}
	if app != nil {
		app.bump()
	}
	go func() { defer recoverLog("login refresh"); a.Refresh(cp.ID, true) }()
	return &cp, note, nil
}

func (a *Accounts) Remove(id string) bool {
	a.mu.Lock()
	keep := a.list[:0]
	found := false
	for _, x := range a.list {
		if x.ID == id {
			found = true
			continue
		}
		keep = append(keep, x)
	}
	a.list = keep
	delete(a.stores, id)
	delete(a.sessions, id)
	a.save()
	a.mu.Unlock()
	if found && !demoMode() {
		a.secrets.Delete(id)
		os.Remove(a.storePath(id))
	}
	if app != nil {
		app.bump()
	}
	return found
}

// ---------------------------------------------------------------- 상점 받기

func (a *Accounts) Busy(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.busy[id]
}

// Refresh: 계정의 상점 · 지갑을 다시 받아요
func (a *Accounts) Refresh(id string, force bool) error {
	l := a.lockFor(id)
	l.Lock()
	defer l.Unlock()
	acc := a.get(id)
	if acc == nil {
		return errors.New("계정을 찾을 수 없어요")
	}
	if !force {
		if st := a.Store(id); st != nil && st.DailyEnds > time.Now().Unix() && time.Since(time.Unix(st.Fetched, 0)) < 10*time.Minute {
			return nil
		}
	}
	a.mu.Lock()
	a.busy[id] = true
	a.mu.Unlock()
	if app != nil {
		app.bump()
	}
	defer func() {
		a.mu.Lock()
		a.busy[id] = false
		a.mu.Unlock()
		if app != nil {
			app.bump()
		}
	}()

	st, err := a.fetchStore(acc)
	if err != nil {
		status := "error"
		if errors.Is(err, errExpired) {
			status = "expired"
			err = errExpired
		}
		log.Printf("account %d: 상점 받기 실패: %v", acc.Slot, err)
		a.update(id, func(x *Account) { x.Status, x.Error = status, err.Error() })
		a.mu.Lock()
		a.nextTry[id] = time.Now().Add(5 * time.Minute)
		a.mu.Unlock()
		return err
	}
	b, _ := json.Marshal(st)
	if !demoMode() {
		os.MkdirAll(filepath.Dir(a.storePath(id)), 0o755)
		writeFileAtomic(a.storePath(id), b, 0o600)
	}
	a.mu.Lock()
	a.stores[id] = st
	delete(a.nextTry, id)
	a.mu.Unlock()
	if app != nil && app.prices != nil && !demoMode() {
		app.prices.Merge(seenPrices(st))
	}
	a.update(id, func(x *Account) { x.Status, x.Error, x.LastOK = "ok", "", time.Now().Unix() })
	log.Printf("account %d: 상점 받음 (오늘의 상점 %d · 밤시장 %d · 번들 %d)", acc.Slot, len(st.Daily), len(st.Night), len(st.Bundles))
	if app != nil && app.notes != nil {
		app.notes.Check(acc, st)
	}
	return nil
}

func (a *Accounts) fetchStore(acc *Account) (*StoreData, error) {
	if demoMode() {
		return demoStore(acc), nil
	}
	a.content.WaitReady(20 * time.Second)
	var sf *rawStorefront
	var wallet map[string]int
	for attempt := 0; attempt < 2; attempt++ {
		s, err := a.ensureSession(acc, attempt > 0)
		if err != nil {
			return nil, err
		}
		sf, err = fetchStorefront(s, acc.Shard, acc.PUUID)
		if errors.Is(err, errTokenRejected) && attempt == 0 {
			continue
		}
		if err != nil {
			if errors.Is(err, errTokenRejected) {
				return nil, errors.New("라이엇 서버가 상점 요청을 받아 주지 않았어요. 잠시 뒤 다시 시도해 주세요")
			}
			return nil, err
		}
		if w, werr := fetchWallet(s, acc.Shard, acc.PUUID); werr == nil {
			wallet = w
		}
		break
	}
	if sf == nil {
		return nil, errors.New("상점 정보를 받지 못했어요")
	}
	st := buildStore(sf, time.Now())
	if wallet == nil {
		if old := a.Store(acc.ID); old != nil {
			wallet = old.Wallet
		}
	}
	st.Wallet = wallet
	return st, nil
}

func buildStore(sf *rawStorefront, now time.Time) *StoreData {
	t := now.Unix()
	st := &StoreData{Fetched: t, Daily: []RawOffer{}, Night: []RawOffer{}, Bundles: []RawBundle{}, Seen: map[string]int{}}
	sp := sf.SkinsPanelLayout
	if len(sp.SingleItemStoreOffers) > 0 {
		for _, o := range sp.SingleItemStoreOffers {
			item := ""
			if len(o.Rewards) > 0 {
				item = o.Rewards[0].ItemID
			}
			if item == "" {
				item = o.OfferID
			}
			st.Daily = append(st.Daily, RawOffer{Item: strings.ToLower(item), Price: o.Cost[currencyVP]})
		}
	} else {
		for _, id := range sp.SingleItemOffers {
			st.Daily = append(st.Daily, RawOffer{Item: strings.ToLower(id)})
		}
	}
	if sp.Remaining > 0 {
		st.DailyEnds = t + sp.Remaining
	} else {
		st.DailyEnds = nextDailyReset(now).Unix()
	}
	if sf.BonusStore != nil && sf.BonusStore.Remaining > 0 {
		st.NightEnds = t + sf.BonusStore.Remaining
		for _, o := range sf.BonusStore.BonusStoreOffers {
			item := ""
			if len(o.Offer.Rewards) > 0 {
				item = o.Offer.Rewards[0].ItemID
			}
			st.Night = append(st.Night, RawOffer{Item: strings.ToLower(item), Price: o.DiscountCosts[currencyVP], Base: o.Offer.Cost[currencyVP], Pct: o.DiscountPercent, Seen: o.IsSeen})
		}
	}
	for _, b := range sf.FeaturedBundle.Bundles {
		rb := RawBundle{ID: strings.ToLower(b.DataAssetID), Price: b.TotalDiscountedCost[currencyVP], Base: b.TotalBaseCost[currencyVP], Items: len(b.Items)}
		for _, it := range b.Items {
			if it.BasePrice > 0 && it.Item.ItemID != "" {
				st.Seen[strings.ToLower(it.Item.ItemID)] = it.BasePrice
			}
		}
		if rb.Price == 0 || rb.Base == 0 {
			p, bs := 0, 0
			for _, it := range b.Items {
				p += it.DiscountedPrice
				bs += it.BasePrice
			}
			if rb.Price == 0 {
				rb.Price = p
			}
			if rb.Base == 0 {
				rb.Base = bs
			}
		}
		rem := b.DurationRemainingInSeconds
		if rem == 0 {
			rem = sf.FeaturedBundle.BundleRemainingDurationInSeconds
		}
		if rem > 0 {
			rb.Ends = t + rem
		}
		st.Bundles = append(st.Bundles, rb)
	}
	return st
}

// nextDailyReset: 상점은 매일 00:00 UTC (한국 오전 9시)에 바뀌어요
func nextDailyReset(now time.Time) time.Time {
	u := now.UTC()
	r := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	if !r.After(u) {
		r = r.Add(24 * time.Hour)
	}
	return r
}

// loop: 처음 켤 때 모든 계정의 상점을 받고, 상점이 바뀌는 시각이 지나면 다시 받아요
func (a *Accounts) loop() {
	a.content.WaitReady(40 * time.Second)
	for _, id := range a.IDs() {
		if err := a.Refresh(id, false); err == nil {
			if st := a.Store(id); st != nil && app != nil && app.notes != nil {
				if acc := a.get(id); acc != nil {
					app.notes.Check(acc, st)
				}
			}
		}
	}
	for range time.Tick(30 * time.Second) {
		now := time.Now()
		for _, id := range a.IDs() {
			acc := a.get(id)
			if acc == nil {
				continue
			}
			a.mu.Lock()
			next, waiting := a.nextTry[id]
			st := a.stores[id]
			a.mu.Unlock()
			if waiting && now.Before(next) {
				continue
			}
			if acc.Status == "expired" && waiting {
				continue
			}
			due := st == nil || now.Unix() >= st.DailyEnds+15 || (st.NightEnds > 0 && now.Unix() >= st.NightEnds+15 && now.Unix() < st.NightEnds+600)
			if acc.Status == "error" && waiting == false {
				due = true
			}
			if due {
				if acc.Status == "expired" {
					// 만료된 계정은 6시간마다 한 번만 다시 시도해 봐요 (일시적인 문제였을 수도 있어서)
					a.mu.Lock()
					a.nextTry[id] = now.Add(6 * time.Hour)
					a.mu.Unlock()
				}
				a.Refresh(id, true)
			}
		}
	}
}

// ---------------------------------------------------------------- HTTP

func (app *App) handleStore(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	acc := app.accounts.get(id)
	if acc == nil {
		jsonError(w, 404, "계정을 찾을 수 없어요")
		return
	}
	st := app.accounts.Store(id)
	if st == nil || st.DailyEnds <= time.Now().Unix() || time.Since(time.Unix(st.Fetched, 0)) > 15*time.Minute {
		if !app.accounts.Busy(id) && acc.Status != "expired" {
			go func() { defer recoverLog("store refresh"); app.accounts.Refresh(id, false) }()
		}
	}
	app.mu.Lock()
	if app.settings.Active != id {
		app.settings.Active = id
		app.saveSettings()
	}
	app.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "store": app.accounts.StoreView(id), "loading": app.accounts.Busy(id)})
}

func (app *App) handleStoreRefresh(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	readJSON(r, &in)
	if err := app.accounts.Refresh(in.ID, true); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "store": app.accounts.StoreView(in.ID)})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "store": app.accounts.StoreView(in.ID)})
}

func (app *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in LoginInput
	if err := readJSON(r, &in); err != nil {
		jsonError(w, 400, "로그인 정보를 읽지 못했어요")
		return
	}
	acc, note, err := app.accounts.Login(in)
	if err != nil {
		log.Printf("login 실패: %v", err)
		jsonError(w, 400, err.Error())
		return
	}
	app.mu.Lock()
	app.settings.Active = acc.ID
	app.saveSettings()
	app.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "id": acc.ID, "slot": acc.Slot, "name": acc.Name, "tag": acc.Tag, "note": note})
}

func (app *App) handleRemove(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	readJSON(r, &in)
	if !app.accounts.Remove(in.ID) {
		jsonError(w, 404, "계정을 찾을 수 없어요")
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleAccountCheck: 키체인에 로그인 정보가 남아 있는지 (설정 화면 · 시험용)
func (app *App) handleAccountCheck(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"store": app.accounts.secrets.Name()}
	if r.URL.Query().Get("selftest") == "1" {
		id := "selftest" + randID(4)
		v := `[{"name":"ssid","value":"x"}]`
		err := app.accounts.secrets.Set(id, v)
		got, gerr := app.accounts.secrets.Get(id)
		app.accounts.secrets.Delete(id)
		_, after := app.accounts.secrets.Get(id)
		out["ok"] = err == nil && gerr == nil && got == v && after != nil
		if err != nil {
			out["error"] = err.Error()
		}
	}
	writeJSON(w, out)
}
