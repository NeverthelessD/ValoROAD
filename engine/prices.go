package main

// 스킨 가격 (VP)
//
// 스킨 정보(valorant-api.com)에는 가격이 없고, 라이엇의 '전체 판매 목록'(store/v1/offers)은 2025년부터 404 로 사라졌어요
// (v1.0.1 에서 써 봤다가 사용자 맥에서 HTTP 404 확인, 2026-10-24).
// 그래서 계정 3개의 오늘의 상점 · 밤시장 · 번들에서 본 원래 가격을 모아(Seen) 쓰고, 본 적 없는 스킨은 등급별 보통 가격을 '약'으로 보여 줘요.
// VP 맵은 예전 가격표 자리로 남겨 두었어요(지금은 비어 있음).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Prices struct {
	mu      sync.RWMutex
	path    string
	VP      map[string]int `json:"vp"`   // 라이엇 전체 가격표 (store/v1/offers)
	Seen    map[string]int `json:"seen"` // 상점 · 밤시장 · 번들에서 본 원래 가격 (가격표를 못 받아도 쓸 수 있게)
	Fetched int64          `json:"fetched"`
	err     string
}

func NewPrices(support string) *Prices {
	p := &Prices{path: filepath.Join(support, "content", "prices.json"), VP: map[string]int{}, Seen: map[string]int{}}
	if b, err := os.ReadFile(p.path); err == nil {
		json.Unmarshal(b, p)
	}
	if p.VP == nil {
		p.VP = map[string]int{}
	}
	if p.Seen == nil {
		p.Seen = map[string]int{}
	}
	return p
}

// Ready: 라이엇 전체 가격표를 받았는지
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

func (p *Prices) Err() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.err
}

func (p *Prices) SetErr(e string) {
	p.mu.Lock()
	p.err = e
	p.mu.Unlock()
}

func (p *Prices) Get(level string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	level = strings.ToLower(level)
	if v := p.VP[level]; v > 0 {
		return v
	}
	return p.Seen[level]
}

func (p *Prices) save() {
	b, _ := json.Marshal(p)
	os.MkdirAll(filepath.Dir(p.path), 0o755)
	writeFileAtomic(p.path, b, 0o644)
}

func (p *Prices) Set(m map[string]int) {
	p.mu.Lock()
	p.VP, p.Fetched, p.err = m, time.Now().Unix(), ""
	p.save()
	p.mu.Unlock()
}

// Merge: 상점에서 본 가격을 더해요 (바뀐 게 있을 때만 저장)
func (p *Prices) Merge(m map[string]int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	changed := false
	for k, v := range m {
		k = strings.ToLower(k)
		if v > 0 && p.Seen[k] != v {
			p.Seen[k] = v
			changed = true
		}
	}
	if changed {
		p.save()
	}
}

func (p *Prices) Status() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	s := fmt.Sprintf("전체 가격표 %d개 · 상점에서 본 가격 %d개", len(p.VP), len(p.Seen))
	if p.Fetched > 0 {
		s += " · 받은 시각 " + time.Unix(p.Fetched, 0).Format("01-02 15:04")
	}
	if p.err != "" {
		s += " · 마지막 실패: " + p.err
	}
	return s
}

// tierPrice: 등급별 보통 가격 (라이엇 가격표에 없을 때 '약'으로 보여 주는 값)
func tierPrice(s Skin) int {
	if s.Cat == "melee" {
		return map[string]int{"select": 1750, "deluxe": 2550, "premium": 3550, "exclusive": 4350, "ultra": 4950}[s.Tier]
	}
	return map[string]int{"select": 875, "deluxe": 1275, "premium": 1775, "exclusive": 2175, "ultra": 2475}[s.Tier]
}

// priceOf: 스킨의 가격과 상태
//
//	"ok" 라이엇 가격 · "estimate" 등급별 보통 가격(약) · "none" 상점에서 팔지 않음 · "unknown" 모름
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
	if v := tierPrice(s); v > 0 {
		return v, "estimate"
	}
	return 0, "unknown"
}
