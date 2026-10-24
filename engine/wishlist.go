package main

// 위시리스트 · 알림
//
// 상점을 새로 받을 때마다(매일 오전 9시 · 앱을 켤 때) 위시리스트 스킨이 있는지 보고, 아직 알리지 않았으면 알림을 만들어요.
// 알림은 앱 껍데기(Swift)가 15초마다 /api/notify/pending 으로 가져가서 맥 알림으로 띄워요.
// 같은 계정 · 같은 스킨 · 같은 상점 기간에는 한 번만 알려요 (Support/notified.json).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type Note struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Sound bool   `json:"sound"`
}

type Notifier struct {
	mu      sync.Mutex
	support string
	sent    map[string]int64
	queue   []Note
}

func NewNotifier(support string) *Notifier {
	n := &Notifier{support: support, sent: map[string]int64{}}
	if b, err := os.ReadFile(n.path()); err == nil {
		json.Unmarshal(b, &n.sent)
	}
	return n
}

func (n *Notifier) path() string { return filepath.Join(n.support, "notified.json") }

// josa: 받침이 있으면 "이", 없으면 "가" (한글이 아니면 "이(가)")
func josa(word string) string {
	r, _ := utf8.DecodeLastRuneInString(strings.TrimSpace(word))
	if r >= 0xAC00 && r <= 0xD7A3 {
		if (r-0xAC00)%28 != 0 {
			return "이"
		}
		return "가"
	}
	return "이(가)"
}

// Check: 이 계정의 상점에 위시리스트 스킨이 있으면 알림을 만들어요
func (n *Notifier) Check(acc *Account, st *StoreData) {
	if app == nil || st == nil {
		return
	}
	s := app.Settings()
	if !s.WishAlert || len(s.Wish) == 0 {
		return
	}
	wish := app.wishSet()
	now := time.Now().Unix()
	who := acc.Name
	if acc.Tag != "" {
		who += "#" + acc.Tag
	}
	type hit struct{ name, key string }
	collect := func(list []RawOffer, ends int64, kind string) []hit {
		out := []hit{}
		if ends <= now {
			return out
		}
		for _, o := range list {
			sk := app.content.SkinByItem(o.Item)
			if sk == nil || !wish[sk.ID] {
				continue
			}
			out = append(out, hit{sk.Name, fmt.Sprintf("%s|%s|%s|%d", acc.ID, sk.ID, kind, ends/600)})
		}
		return out
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, k := range []struct {
		kind, title, where string
		list               []RawOffer
		ends               int64
		on                 bool
	}{
		{"daily", "위시리스트 스킨 등장!", "상점", st.Daily, st.DailyEnds, true},
		{"night", "밤시장에 위시리스트 스킨!", "밤시장", st.Night, st.NightEnds, s.NightAlert},
	} {
		if !k.on {
			continue
		}
		fresh := []hit{}
		for _, h := range collect(k.list, k.ends, k.kind) {
			if _, done := n.sent[h.key]; !done {
				fresh = append(fresh, h)
				n.sent[h.key] = now
			}
		}
		if len(fresh) == 0 {
			continue
		}
		body := fmt.Sprintf("%s %s에 ‘%s’%s 떴어요.", who, k.where, fresh[0].name, josa(fresh[0].name))
		if len(fresh) > 1 {
			body = fmt.Sprintf("%s %s에 ‘%s’ 외 %d개가 떴어요.", who, k.where, fresh[0].name, len(fresh)-1)
		}
		n.queue = append(n.queue, Note{ID: randID(6), Title: k.title, Body: body, Sound: s.Sound})
	}
	// 2주 지난 기록은 지워요
	for key, t := range n.sent {
		if now-t > 14*24*3600 {
			delete(n.sent, key)
		}
	}
	b, _ := json.Marshal(n.sent)
	writeFileAtomic(n.path(), b, 0o644)
	if len(n.queue) > 20 {
		n.queue = n.queue[len(n.queue)-20:]
	}
}

func (n *Notifier) Take() []Note {
	n.mu.Lock()
	defer n.mu.Unlock()
	q := n.queue
	n.queue = nil
	if q == nil {
		q = []Note{}
	}
	return q
}

func (n *Notifier) Push(note Note) {
	n.mu.Lock()
	n.queue = append(n.queue, note)
	n.mu.Unlock()
}

// ---------------------------------------------------------------- HTTP

func (a *App) handleNotifyPending(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"notes": a.notes.Take()})
}

func (a *App) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	a.notes.Push(Note{ID: randID(6), Title: "ValoROAD 알림 시험", Body: "위시리스트 스킨이 뜨면 이렇게 알려 드려요.", Sound: a.Settings().Sound})
	writeJSON(w, map[string]any{"ok": true})
}

type SkinView struct {
	Skin
	Wished     bool   `json:"wished"`
	Price      int    `json:"price"`
	PriceState string `json:"priceState"`
}

func (a *App) handleSkins(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 300 {
		limit = 120
	}
	wish := a.wishSet()
	all := a.content.Search(q.Get("q"), q.Get("cat"), 0)
	out := []SkinView{}
	for i, s := range all {
		if i >= limit {
			break
		}
		price, ps := a.priceOf(s)
		out = append(out, SkinView{Skin: s, Wished: wish[s.ID], Price: price, PriceState: ps})
	}
	writeJSON(w, map[string]any{"skins": out, "total": len(all), "ready": a.content.Ready()})
}

func (a *App) handleWish(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
		On bool   `json:"on"`
	}
	if err := readJSON(r, &in); err != nil || in.ID == "" {
		jsonError(w, 400, "bad request")
		return
	}
	id := strings.ToLower(in.ID)
	if sk := a.content.SkinByItem(id); sk != nil {
		id = sk.ID // 상점의 레벨 uuid 로 와도 스킨 uuid 로 저장
	}
	a.mu.Lock()
	list := []string{}
	for _, x := range a.settings.Wish {
		if x != id {
			list = append(list, x)
		}
	}
	if in.On {
		list = append([]string{id}, list...)
	}
	if len(list) > 200 {
		list = list[:200]
	}
	a.settings.Wish = list
	a.saveSettings()
	a.mu.Unlock()
	a.bump()
	writeJSON(w, map[string]any{"ok": true, "wish": list})
}

type WhereView struct {
	Account string `json:"account"`
	Slot    int    `json:"slot"`
	Kind    string `json:"kind"` // daily | night
	Price   int    `json:"price"`
	Base    int    `json:"base,omitempty"`
}

type WishView struct {
	Skin
	Known      bool        `json:"known"`
	Where      []WhereView `json:"where"`
	Price      int         `json:"price"`
	PriceState string      `json:"priceState"`
}

func (a *App) handleWishlist(w http.ResponseWriter, r *http.Request) {
	s := a.Settings()
	if a.prices.Stale() && !demoMode() {
		go func() { defer recoverLog("prices"); a.accounts.ensurePrices(true) }()
	}
	where := map[string][]WhereView{}
	for _, v := range a.accounts.Views() {
		label := v.Name
		if v.Tag != "" {
			label += "#" + v.Tag
		}
		for _, h := range v.Hits {
			where[h.Skin] = append(where[h.Skin], WhereView{Account: label, Slot: v.Slot, Kind: h.Kind, Price: h.Price, Base: h.Base})
		}
	}
	out := []WishView{}
	total := 0
	for _, id := range s.Wish {
		wv := WishView{Where: where[id]}
		if wv.Where == nil {
			wv.Where = []WhereView{}
		}
		if sk := a.content.SkinByItem(id); sk != nil {
			wv.Skin, wv.Known = *sk, true
			wv.Price, wv.PriceState = a.priceOf(*sk)
		} else {
			wv.Skin = Skin{ID: id, Name: "스킨 정보를 불러오는 중"}
			wv.PriceState = "unknown"
		}
		total += wv.Price
		out = append(out, wv)
	}
	writeJSON(w, map[string]any{"items": out, "total": total, "pricesReady": a.prices.Ready() || demoMode(), "priceError": a.prices.Err()})
}
