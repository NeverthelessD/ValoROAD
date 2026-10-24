package main

// 시험 모드 (데모): 라이엇에 로그인하지 않고, 실제 스킨 정보로 가짜 계정 3개의 상점을 만들어 보여 줘요.
// 리눅스 화면 시험 · 맥 빌드 서버의 시험(스크린샷)에서만 써요. 켜는 법: VALOROAD_DEMO=1 또는 Support/demo-mode 파일.

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	demoOnce sync.Once
	demoOn   bool
)

func demoMode() bool {
	demoOnce.Do(func() {
		demoOn = env("DEMO") == "1"
		if !demoOn && app != nil {
			demoOn = fileExists(filepath.Join(app.support, "demo-mode"))
		}
	})
	return demoOn
}

func demoAccounts() []*Account {
	now := time.Now().Unix()
	return []*Account{
		{ID: "demoslot1", Slot: 1, Name: "본캐", Tag: "KR1", Region: "kr", Shard: "kr", Added: now, Status: "ok"},
		{ID: "demoslot2", Slot: 2, Name: "부캐", Tag: "KR2", Region: "kr", Shard: "kr", Added: now, Status: "ok"},
		{ID: "demoslot3", Slot: 3, Name: "듀오용", Tag: "7777", Region: "kr", Shard: "kr", Added: now, Status: "ok"},
	}
}

func demoPrice(s Skin) int {
	p := map[string]int{"select": 875, "deluxe": 1275, "premium": 1775, "exclusive": 2175, "ultra": 2475}[s.Tier]
	if s.Cat == "melee" {
		p = map[string]int{"select": 1750, "deluxe": 2550, "premium": 3550, "exclusive": 4350, "ultra": 4950}[s.Tier]
	}
	return p
}

func demoStore(acc *Account) *StoreData {
	now := time.Now()
	day := now.UTC().Unix() / 86400
	c := app.content
	st := &StoreData{Fetched: now.Unix(), Daily: []RawOffer{}, Night: []RawOffer{}, Bundles: []RawBundle{},
		DailyEnds: nextDailyReset(now).Unix(), NightEnds: now.Unix() + 5*86400 + 3600}
	for _, s := range c.Random(4, day*10+int64(acc.Slot), nil) {
		st.Daily = append(st.Daily, RawOffer{Item: s.Level, Price: demoPrice(s)})
	}
	pcts := []int{32, 25, 41, 18, 37, 29}
	for i, s := range c.Random(6, day*10+int64(acc.Slot)+500, func(t string) bool { return t != "select" }) {
		base := demoPrice(s)
		st.Night = append(st.Night, RawOffer{Item: s.Level, Base: base, Pct: pcts[i%len(pcts)], Price: base * (100 - pcts[i%len(pcts)]) / 100})
	}
	c.mu.RLock()
	if c.data != nil {
		ids := []string{}
		for _, b := range c.data.Bundles {
			if b.Img != "" && b.Name != "" && !strings.Contains(b.Name, "VCT") && !strings.Contains(b.Name, " x ") {
				ids = append(ids, b.ID)
			}
		}
		sort.Strings(ids)
		if len(ids) > 0 {
			st.Bundles = append(st.Bundles, RawBundle{ID: ids[int(day)%len(ids)], Price: 7100, Base: 8875, Ends: now.Unix() + 3*86400 + 7*3600, Items: 4})
		}
	}
	c.mu.RUnlock()
	st.Wallet = map[int]map[string]int{
		1: {"vp": 2350, "rp": 40, "kc": 1850},
		2: {"vp": 480, "rp": 0, "kc": 3120},
		3: {"vp": 10200, "rp": 120, "kc": 640},
	}[acc.Slot]
	return st
}
