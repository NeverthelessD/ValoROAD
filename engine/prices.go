package main

// 스킨 가격 (VP)
//
// 스킨 정보(valorant-api.com)에는 가격이 없어서, 라이엇 상점의 '전체 판매 목록'(store/v1/offers)에서 가격을 받아요.
// 저장된 계정 아무거나로 하루에 한 번 받아 Support/content/prices.json 에 저장해요 (스킨 레벨 uuid → VP).
// 목록에 없는 스킨(배틀패스 · 이벤트 보상 등)은 상점에서 팔지 않는 스킨이에요.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Prices struct {
	mu      sync.RWMutex
	path    string
	VP      map[string]int `json:"vp"`
	Fetched int64          `json:"fetched"`
}

func NewPrices(support string) *Prices {
	p := &Prices{path: filepath.Join(support, "content", "prices.json"), VP: map[string]int{}}
	if b, err := os.ReadFile(p.path); err == nil {
		json.Unmarshal(b, p)
		if p.VP == nil {
			p.VP = map[string]int{}
		}
	}
	return p
}

func (p *Prices) Ready() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.VP) > 0
}

func (p *Prices) Stale() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.VP) == 0 || time.Since(time.Unix(p.Fetched, 0)) > 24*time.Hour
}

func (p *Prices) Get(level string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.VP[strings.ToLower(level)]
}

func (p *Prices) Set(m map[string]int) {
	p.mu.Lock()
	p.VP, p.Fetched = m, time.Now().Unix()
	b, _ := json.Marshal(p)
	p.mu.Unlock()
	os.MkdirAll(filepath.Dir(p.path), 0o755)
	writeFileAtomic(p.path, b, 0o644)
}

// priceOf: 스킨의 가격과 상태 ("ok" 가격 있음 · "none" 상점에서 팔지 않음 · "unknown" 아직 가격표가 없음)
func (a *App) priceOf(s Skin) (int, string) {
	if v := a.prices.Get(s.Level); v > 0 {
		return v, "ok"
	}
	if demoMode() {
		if v := demoPrice(s); v > 0 {
			return v, "ok"
		}
	}
	if a.prices.Ready() {
		return 0, "none"
	}
	return 0, "unknown"
}

// fetchOffers: 상점에서 파는 모든 아이템의 VP 가격
func fetchOffers(s *session, shard string) (map[string]int, error) {
	b, err := pdGet(s, shard, "GET", "/store/v1/offers/", nil)
	if err != nil {
		return nil, err
	}
	return parseOffers(b)
}

func parseOffers(b []byte) (map[string]int, error) {
	var v struct {
		Offers []struct {
			OfferID string         `json:"OfferID"`
			Cost    map[string]int `json:"Cost"`
			Rewards []struct {
				ItemID string `json:"ItemID"`
			} `json:"Rewards"`
		} `json:"Offers"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, o := range v.Offers {
		vp := o.Cost[currencyVP]
		if vp <= 0 {
			continue
		}
		id := o.OfferID
		if len(o.Rewards) > 0 && o.Rewards[0].ItemID != "" {
			id = o.Rewards[0].ItemID
		}
		out[strings.ToLower(id)] = vp
	}
	return out, nil
}
