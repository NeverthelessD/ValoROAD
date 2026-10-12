package main

// 계정별 상점(오늘의 상점 · 야시장 · 번들) · 잔액 · 보유 스킨을 모아 화면용으로 정리하고,
// 위시리스트 스킨이 뜨면 알려 줍니다.

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type ItemView struct {
	Kind     string `json:"kind"` // skin | buddy | card | spray | title | flex | other
	ID       string `json:"id"`
	SkinID   string `json:"skinId,omitempty"`
	Name     string `json:"name"`
	Weapon   string `json:"weapon,omitempty"`
	Category string `json:"category,omitempty"`
	Icon     string `json:"icon,omitempty"`
	Tier     *Tier  `json:"tier,omitempty"`
	Price    int    `json:"price"`
	Base     int    `json:"base,omitempty"`
	Discount int    `json:"discount,omitempty"`
	Amount   int    `json:"amount,omitempty"`
	Owned    bool   `json:"owned,omitempty"`
	Wish     bool   `json:"wish,omitempty"`
	Video    bool   `json:"video,omitempty"`
	Seen     bool   `json:"seen,omitempty"`
}

type BundleView struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Sub      string     `json:"sub,omitempty"`
	Image    string     `json:"image,omitempty"`
	Vertical string     `json:"vertical,omitempty"`
	Price    int        `json:"price"`
	Base     int        `json:"base"`
	Discount int        `json:"discount,omitempty"`
	EndsAt   int64      `json:"endsAt"`
	Items    []ItemView `json:"items"`
}

type Wallet struct {
	VP int `json:"vp"`
	RP int `json:"rp"`
	KC int `json:"kc"`
}

type StoreView struct {
	AccountID   string       `json:"accountId"`
	FetchedAt   int64        `json:"fetchedAt"`
	Daily       []ItemView   `json:"daily"`
	DailyEndsAt int64        `json:"dailyEndsAt"`
	Night       []ItemView   `json:"night"`
	NightEndsAt int64        `json:"nightEndsAt"`
	Bundles     []BundleView `json:"bundles"`
	Wallet      Wallet       `json:"wallet"`
	Level       int          `json:"level"`
	Card        string       `json:"card,omitempty"`
	OwnedCount  int          `json:"ownedCount"`
}

type refreshState struct {
	Busy  bool   `json:"busy"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

type Stores struct {
	support string
	acc     *Accounts
	cat     *Catalog
	wish    *Wishlist

	mu    sync.Mutex
	views map[string]*StoreView
	state map[string]*refreshState
	locks map[string]*sync.Mutex
	seq   int64 // 바뀔 때마다 1씩 (화면이 새로 그릴 때를 알도록)
}

func NewStores(support string, acc *Accounts, cat *Catalog, wish *Wishlist) *Stores {
	return &Stores{support: support, acc: acc, cat: cat, wish: wish,
		views: map[string]*StoreView{}, state: map[string]*refreshState{}, locks: map[string]*sync.Mutex{}}
}

func (s *Stores) cachePath(id string) string {
	return filepath.Join(s.support, "cache", "store-"+id+".json")
}

func (s *Stores) Cached(id string) *StoreView {
	s.mu.Lock()
	v := s.views[id]
	s.mu.Unlock()
	st, statErr := os.Stat(s.cachePath(id))
	if v != nil && (statErr != nil || st.ModTime().UnixMilli() <= v.FetchedAt+2000) {
		return v
	}
	// 매일 9시 확인(다른 프로세스)이 더 새 상점을 저장해 뒀으면 그걸 읽어요
	b, err := os.ReadFile(s.cachePath(id))
	if err != nil {
		return v
	}
	var sv StoreView
	if json.Unmarshal(b, &sv) != nil {
		return v
	}
	if v != nil && sv.FetchedAt <= v.FetchedAt {
		return v
	}
	s.mu.Lock()
	s.views[id] = &sv
	s.mu.Unlock()
	return &sv
}

func (s *Stores) dropCache(id string) {
	s.mu.Lock()
	delete(s.views, id)
	delete(s.state, id)
	s.seq++
	s.mu.Unlock()
}

func (s *Stores) States() (map[string]refreshState, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]refreshState{}
	for k, v := range s.state {
		out[k] = *v
	}
	return out, s.seq
}

func (s *Stores) setState(id string, fn func(*refreshState)) {
	s.mu.Lock()
	st := s.state[id]
	if st == nil {
		st = &refreshState{}
		s.state[id] = st
	}
	fn(st)
	s.seq++
	s.mu.Unlock()
}

func (s *Stores) lock(id string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.locks[id]
	if l == nil {
		l = &sync.Mutex{}
		s.locks[id] = l
	}
	return l
}

func (v *StoreView) Stale() bool {
	if v == nil {
		return true
	}
	now := time.Now().UnixMilli()
	return now >= v.DailyEndsAt || (v.NightEndsAt > 0 && now >= v.NightEndsAt)
}

// Refresh downloads the account's store again.
func (s *Stores) Refresh(id string) (*StoreView, error) {
	l := s.lock(id)
	if !l.TryLock() {
		// 이미 받는 중이면 끝날 때까지 기다렸다가 그 결과를 줘요
		l.Lock()
		l.Unlock()
		if v := s.Cached(id); v != nil {
			return v, nil
		}
		return nil, errors.New("상점을 불러오지 못했어요")
	}
	defer l.Unlock()
	s.setState(id, func(st *refreshState) { st.Busy, st.Error, st.Code = true, "", "" })
	v, err := s.fetch(id)
	s.setState(id, func(st *refreshState) {
		st.Busy = false
		if err != nil {
			st.Error = err.Error()
			if errors.Is(err, errSessionExpired) {
				st.Code = "expired"
				st.Error = errSessionExpired.Error()
			}
		}
	})
	if err != nil {
		log.Printf("refresh %s: %v", id, err)
		return nil, err
	}
	return v, nil
}

func (s *Stores) fetch(id string) (*StoreView, error) {
	if err := s.cat.Ensure(false); err != nil {
		return nil, err
	}
	sess, err := s.acc.Session(id, false)
	if err != nil {
		return nil, err
	}
	var (
		sf    *riotStorefront
		wal   map[string]int
		owned []string
		level int
		card  string
	)
	call := func() error {
		var wg sync.WaitGroup
		var sfErr, walErr error
		wg.Add(5)
		go func() { defer wg.Done(); sf, sfErr = fetchStorefront(sess) }()
		go func() { defer wg.Done(); wal, walErr = fetchWallet(sess) }()
		go func() {
			defer wg.Done()
			var e error
			if owned, e = fetchOwnedSkins(sess); e != nil {
				log.Printf("owned: %v", e)
			}
		}()
		go func() {
			defer wg.Done()
			var e error
			if level, e = fetchAccountLevel(sess); e != nil {
				log.Printf("level: %v", e)
			}
		}()
		go func() {
			defer wg.Done()
			var e error
			if card, e = fetchPlayerCard(sess); e != nil {
				log.Printf("card: %v", e)
			}
		}()
		wg.Wait()
		if sfErr != nil {
			return sfErr
		}
		if walErr != nil {
			log.Printf("wallet: %v", walErr)
		}
		return nil
	}
	err = call()
	if errors.Is(err, errTokenInvalid) {
		if sess, err = s.acc.Session(id, true); err != nil {
			return nil, err
		}
		err = call()
	}
	if err != nil {
		if errors.Is(err, errTokenInvalid) {
			return nil, errors.New("Riot 서버가 요청을 받아 주지 않았어요. 잠시 뒤 다시 시도해 주세요")
		}
		var ae *apiError
		if errors.As(err, &ae) {
			log.Printf("storefront error body: %.300s", ae.Body)
			return nil, fmt.Errorf("상점 정보를 받지 못했어요 (HTTP %d). 잠시 뒤 다시 시도해 주세요", ae.Status)
		}
		return nil, err
	}
	v := s.build(id, sf, wal, owned, level, card)
	b, _ := json.Marshal(v)
	os.MkdirAll(filepath.Dir(s.cachePath(id)), 0o755)
	writeFileAtomic(s.cachePath(id), b, 0o600)
	s.mu.Lock()
	s.views[id] = v
	s.seq++
	s.mu.Unlock()
	s.acc.update(id, func(a *Account) {
		if level > 0 {
			a.Level = level
		}
		if card != "" {
			a.Card = card
		}
		a.LastRefresh = v.FetchedAt
		a.Status, a.LastError = "ok", ""
	})
	return v, nil
}

func (s *Stores) item(typ, id string, owned map[string]bool) ItemView {
	typ, id = strings.ToLower(typ), strings.ToLower(id)
	it := ItemView{ID: id, Kind: "other"}
	switch typ {
	case itemTypeSkin, "":
		it.Kind = "skin"
		if sk := s.cat.SkinByLevel(id); sk != nil {
			it.SkinID, it.Name, it.Weapon, it.Category, it.Icon = sk.UUID, sk.Name, sk.Weapon, sk.Category, sk.Icon
			if t, ok := s.cat.Data().Tiers[sk.Tier]; ok {
				tt := t
				it.Tier = &tt
			}
			for _, l := range sk.Levels {
				if l.Video != "" {
					it.Video = true
				}
			}
			for _, c := range sk.Chromas {
				if c.Video != "" {
					it.Video = true
				}
			}
		} else {
			it.Name = "알 수 없는 스킨"
			it.Icon = "https://media.valorant-api.com/weaponskinlevels/" + id + "/displayicon.png"
		}
		it.Owned = owned[id]
	case itemTypeBuddy:
		it.Kind, it.Name, it.Icon = "buddy", "총기 장식", "https://media.valorant-api.com/buddylevels/"+id+"/displayicon.png"
	case itemTypeCard:
		it.Kind, it.Name, it.Icon = "card", "플레이어 카드", "https://media.valorant-api.com/playercards/"+id+"/largeart.png"
	case itemTypeSpray:
		it.Kind, it.Name, it.Icon = "spray", "스프레이", "https://media.valorant-api.com/sprays/"+id+"/displayicon.png"
	case itemTypeTitle:
		it.Kind, it.Name = "title", "플레이어 칭호"
	case itemTypeFlex:
		it.Kind, it.Name, it.Icon = "flex", "플렉스", "https://media.valorant-api.com/flex/"+id+"/displayicon.png"
	default:
		it.Name = "아이템"
	}
	return it
}

func (s *Stores) build(id string, sf *riotStorefront, wal map[string]int, owned []string, level int, card string) *StoreView {
	now := time.Now().UnixMilli()
	ownedSet := map[string]bool{}
	for _, o := range owned {
		ownedSet[o] = true
	}
	v := &StoreView{AccountID: id, FetchedAt: now, Daily: []ItemView{}, Night: []ItemView{}, Bundles: []BundleView{},
		Level: level, Card: card, OwnedCount: len(owned)}
	v.Wallet = Wallet{VP: wal[currencyVP], RP: wal[currencyRP], KC: wal[currencyKC]}

	sp := sf.SkinsPanelLayout
	v.DailyEndsAt = now + int64(sp.SingleItemOffersRemainingDurationInSeconds)*1000
	if len(sp.SingleItemStoreOffers) > 0 {
		for _, o := range sp.SingleItemStoreOffers {
			typ, iid := itemTypeSkin, o.OfferID
			if len(o.Rewards) > 0 {
				typ, iid = o.Rewards[0].ItemTypeID, o.Rewards[0].ItemID
			}
			it := s.item(typ, iid, ownedSet)
			it.Price = o.Cost[currencyVP]
			v.Daily = append(v.Daily, it)
		}
	} else {
		for _, oid := range sp.SingleItemOffers {
			v.Daily = append(v.Daily, s.item(itemTypeSkin, oid, ownedSet))
		}
	}
	if sf.BonusStore != nil && len(sf.BonusStore.BonusStoreOffers) > 0 {
		v.NightEndsAt = now + int64(sf.BonusStore.BonusStoreRemainingDurationInSeconds)*1000
		for _, b := range sf.BonusStore.BonusStoreOffers {
			typ, iid := itemTypeSkin, b.Offer.OfferID
			if len(b.Offer.Rewards) > 0 {
				typ, iid = b.Offer.Rewards[0].ItemTypeID, b.Offer.Rewards[0].ItemID
			}
			it := s.item(typ, iid, ownedSet)
			it.Base = b.Offer.Cost[currencyVP]
			it.Price = b.DiscountCosts[currencyVP]
			it.Discount = pct(b.DiscountPercent)
			it.Seen = b.IsSeen
			v.Night = append(v.Night, it)
		}
	}
	for _, b := range sf.FeaturedBundle.Bundles {
		bv := BundleView{ID: strings.ToLower(b.DataAssetID), Items: []ItemView{},
			EndsAt: now + int64(b.DurationRemainingInSeconds)*1000, Discount: pct(b.TotalDiscountPercent)}
		if info, ok := s.cat.Data().Bundles[bv.ID]; ok {
			bv.Name, bv.Sub, bv.Image, bv.Vertical = info.Name, info.Sub, info.Icon, info.Vertical
		} else {
			bv.Name = "번들"
		}
		cur := b.CurrencyID
		if cur == "" {
			cur = currencyVP
		}
		bv.Price, bv.Base = b.TotalDiscountedCost[cur], b.TotalBaseCost[cur]
		sumP, sumB := 0, 0
		if len(b.Items) == 0 {
			for _, x := range b.ItemOffers {
				typ, iid := itemTypeSkin, x.Offer.OfferID
				amount := 1
				if len(x.Offer.Rewards) > 0 {
					typ, iid, amount = x.Offer.Rewards[0].ItemTypeID, x.Offer.Rewards[0].ItemID, x.Offer.Rewards[0].Quantity
				}
				it := s.item(typ, iid, ownedSet)
				it.Base, it.Price, it.Amount = x.Offer.Cost[cur], x.DiscountedCost[cur], amount
				if it.Price == 0 && x.DiscountedCost == nil {
					it.Price = it.Base
				}
				it.Discount = pct(x.DiscountPercent)
				sumP += it.Price
				sumB += it.Base
				bv.Items = append(bv.Items, it)
			}
		}
		for _, x := range b.Items {
			it := s.item(x.Item.ItemTypeID, x.Item.ItemID, ownedSet)
			it.Price, it.Base, it.Amount = x.DiscountedPrice, x.BasePrice, x.Item.Amount
			if x.BasePrice > 0 && x.DiscountedPrice < x.BasePrice {
				it.Discount = int((1-float64(x.DiscountedPrice)/float64(x.BasePrice))*100 + 0.5)
			}
			sumP += x.DiscountedPrice
			sumB += x.BasePrice
			bv.Items = append(bv.Items, it)
		}
		if bv.Price == 0 {
			bv.Price = sumP
		}
		if bv.Base == 0 {
			bv.Base = sumB
		}
		if bv.Discount == 0 && bv.Base > bv.Price && bv.Base > 0 {
			bv.Discount = int((1-float64(bv.Price)/float64(bv.Base))*100 + 0.5)
		}
		// 스킨이 먼저, 비싼 순
		sort.SliceStable(bv.Items, func(i, j int) bool {
			if (bv.Items[i].Kind == "skin") != (bv.Items[j].Kind == "skin") {
				return bv.Items[i].Kind == "skin"
			}
			return bv.Items[i].Base > bv.Items[j].Base
		})
		v.Bundles = append(v.Bundles, bv)
	}
	return v
}

// pct accepts both 0.25 and 25 style percentages.
func pct(p float64) int {
	if p <= 0 {
		return 0
	}
	if p <= 1 {
		p *= 100
	}
	return int(p + 0.5)
}

// WithWish returns a copy of v with wishlist flags filled in.
func (s *Stores) WithWish(v *StoreView) *StoreView {
	if v == nil {
		return nil
	}
	w := s.wish.Set()
	c := *v
	mark := func(items []ItemView) []ItemView {
		out := make([]ItemView, len(items))
		for i, it := range items {
			it.Wish = it.SkinID != "" && w[it.SkinID]
			out[i] = it
		}
		return out
	}
	c.Daily, c.Night = mark(v.Daily), mark(v.Night)
	c.Bundles = make([]BundleView, len(v.Bundles))
	for i, b := range v.Bundles {
		b.Items = mark(b.Items)
		c.Bundles[i] = b
	}
	return &c
}

// ---------------------------------------------------------------- 위시리스트

type WishItem struct {
	Skin    string `json:"skin"`
	AddedAt int64  `json:"addedAt"`
}

type Wishlist struct {
	mu    sync.Mutex
	path  string
	items []WishItem
}

func NewWishlist(support string) *Wishlist {
	w := &Wishlist{path: filepath.Join(support, "wishlist.json")}
	w.load()
	return w
}

func (w *Wishlist) load() {
	w.items = []WishItem{}
	if b, err := os.ReadFile(w.path); err == nil {
		json.Unmarshal(b, &w.items)
	}
}

func (w *Wishlist) Items() []WishItem {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.load()
	return append([]WishItem(nil), w.items...)
}

func (w *Wishlist) Set() map[string]bool {
	out := map[string]bool{}
	for _, it := range w.Items() {
		out[it.Skin] = true
	}
	return out
}

func (w *Wishlist) Toggle(skin string, on bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.load()
	skin = strings.ToLower(skin)
	keep := []WishItem{}
	found := false
	for _, it := range w.items {
		if it.Skin == skin {
			found = true
			if !on {
				continue
			}
		}
		keep = append(keep, it)
	}
	if on && !found {
		keep = append(keep, WishItem{Skin: skin, AddedAt: time.Now().UnixMilli()})
	}
	w.items = keep
	b, _ := json.MarshalIndent(w.items, "", "  ")
	writeFileAtomic(w.path, b, 0o644)
}

// ---------------------------------------------------------------- 알림

type wishHit struct {
	Where string
	Name  string
	Skin  string
	Until int64
}

func (s *Stores) Hits(v *StoreView) []wishHit {
	w := s.wish.Set()
	hits := []wishHit{}
	seen := map[string]bool{}
	add := func(where string, until int64, items []ItemView) {
		for _, it := range items {
			if it.SkinID != "" && w[it.SkinID] && !it.Owned && !seen[where+it.SkinID] {
				seen[where+it.SkinID] = true
				hits = append(hits, wishHit{Where: where, Name: it.Name, Skin: it.SkinID, Until: until})
			}
		}
	}
	add("오늘의 상점", v.DailyEndsAt, v.Daily)
	add("야시장", v.NightEndsAt, v.Night)
	for _, b := range v.Bundles {
		add("번들", b.EndsAt, b.Items)
	}
	return hits
}

type notifyLog struct {
	mu   sync.Mutex
	path string
}

// firstTime reports whether key has not been notified yet, and records it until `until`.
func (n *notifyLog) firstTime(key string, until int64) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	m := map[string]int64{}
	if b, err := os.ReadFile(n.path); err == nil {
		json.Unmarshal(b, &m)
	}
	now := time.Now().UnixMilli()
	for k, exp := range m {
		if exp < now-48*3600*1000 {
			delete(m, k)
		}
	}
	if _, ok := m[key]; ok {
		return false
	}
	if until < now {
		until = now + 24*3600*1000
	}
	m[key] = until
	b, _ := json.Marshal(m)
	writeFileAtomic(n.path, b, 0o644)
	return true
}

// NotifyWishlist sends one macOS notification per account for wishlist skins that just appeared.
func (s *Stores) NotifyWishlist(acc *Account, v *StoreView, nl *notifyLog, enabled bool) []wishHit {
	hits := s.Hits(v)
	if !enabled || len(hits) == 0 {
		return hits
	}
	fresh := []wishHit{}
	for _, h := range hits {
		if nl.firstTime(fmt.Sprintf("wish:%s:%s:%s:%d", acc.ID, h.Where, h.Skin, (h.Until+300000)/600000), h.Until) {
			fresh = append(fresh, h)
		}
	}
	if len(fresh) == 0 {
		return hits
	}
	names := []string{}
	for _, h := range fresh {
		names = append(names, fmt.Sprintf("%s (%s)", h.Name, h.Where))
	}
	who := acc.Name
	if acc.Tag != "" {
		who += "#" + acc.Tag
	}
	notifyOS("🎯 위시리스트 스킨이 떴어요!", who+" · "+strings.Join(names, ", "))
	return hits
}
