package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseTokenLocation(t *testing.T) {
	tk, ok := parseTokenLocation("https://playvalorant.com/opt_in#access_token=AAA.BBB&scope=account+openid&iss=x&id_token=III&token_type=Bearer&session_state=s&expires_in=3600")
	if !ok || tk.Access != "AAA.BBB" || tk.ID != "III" {
		t.Fatalf("got %+v %v", tk, ok)
	}
	if d := time.Until(tk.Expires); d < 59*time.Minute || d > 61*time.Minute {
		t.Fatalf("expires %v", d)
	}
	if _, ok := parseTokenLocation("https://authenticate.riotgames.com/login?client_id=play-valorant-web-prod"); ok {
		t.Fatal("login page must not parse as token")
	}
	if _, ok := parseTokenLocation(""); ok {
		t.Fatal("empty")
	}
}

func TestCookies(t *testing.T) {
	old := []Cookie{
		{Name: "ssid", Value: "old", Domain: "auth.riotgames.com", Path: "/"},
		{Name: "clid", Value: "c1", Domain: ".riotgames.com", Path: "/"},
		{Name: "tdid", Value: "t1", Domain: ".riotgames.com", Path: "/", Expires: 1},
		{Name: "other", Value: "o", Domain: "example.com", Path: "/"},
	}
	fresh := []Cookie{
		{Name: "ssid", Value: "new", Domain: "auth.riotgames.com", Path: "/"},
		{Name: "clid", Value: "", Domain: "riotgames.com", Path: "/"},
		{Name: "asid", Value: "a", Domain: "auth.riotgames.com"},
	}
	m := mergeCookies(old, fresh)
	got := map[string]string{}
	for _, c := range m {
		got[c.Name] = c.Value
	}
	if got["ssid"] != "new" || got["clid"] != "" || got["asid"] != "a" || got["tdid"] != "" {
		t.Fatalf("merge: %+v", got)
	}
	h := cookieHeader(onlyRiotCookies(m), "auth.riotgames.com")
	if h != "ssid=new; asid=a" {
		t.Fatalf("header %q", h)
	}
	if cookieDomainMatch("example.com", "auth.riotgames.com") || !cookieDomainMatch(".riotgames.com", "auth.riotgames.com") {
		t.Fatal("domain match")
	}
}

func TestBuildStore(t *testing.T) {
	raw := `{
	 "FeaturedBundle": {"Bundles": [{"ID":"b1","DataAssetID":"D1A","TotalBaseCost":{"85ad13f7-3d1b-5128-9eb2-7cd8ee0b5741":8700},
	   "TotalDiscountedCost":{"85ad13f7-3d1b-5128-9eb2-7cd8ee0b5741":7100},"DurationRemainingInSeconds":3600,
	   "Items":[{"Item":{"ItemTypeID":"e7c63390","ItemID":"x"},"BasePrice":1775,"DiscountedPrice":1500}]}],
	   "BundleRemainingDurationInSeconds": 3600},
	 "SkinsPanelLayout": {"SingleItemOffers":["L1","L2"],
	   "SingleItemStoreOffers":[{"OfferID":"L1","Cost":{"85ad13f7-3d1b-5128-9eb2-7cd8ee0b5741":1775},"Rewards":[{"ItemTypeID":"e7","ItemID":"L1"}]},
	                            {"OfferID":"L2","Cost":{"85ad13f7-3d1b-5128-9eb2-7cd8ee0b5741":875},"Rewards":[{"ItemTypeID":"e7","ItemID":"L2"}]}],
	   "SingleItemOffersRemainingDurationInSeconds": 1000},
	 "BonusStore": {"BonusStoreOffers":[{"Offer":{"Cost":{"85ad13f7-3d1b-5128-9eb2-7cd8ee0b5741":1775},"Rewards":[{"ItemID":"N1"}]},
	   "DiscountPercent":32,"DiscountCosts":{"85ad13f7-3d1b-5128-9eb2-7cd8ee0b5741":1207},"IsSeen":true}],
	   "BonusStoreRemainingDurationInSeconds": 50000}
	}`
	var sf rawStorefront
	if err := json.Unmarshal([]byte(raw), &sf); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	st := buildStore(&sf, now)
	if len(st.Daily) != 2 || st.Daily[0].Item != "l1" || st.Daily[0].Price != 1775 || st.DailyEnds != now.Unix()+1000 {
		t.Fatalf("daily %+v", st)
	}
	if len(st.Night) != 1 || st.Night[0].Price != 1207 || st.Night[0].Base != 1775 || st.Night[0].Pct != 32 || st.NightEnds != now.Unix()+50000 {
		t.Fatalf("night %+v", st.Night)
	}
	if len(st.Bundles) != 1 || st.Bundles[0].ID != "d1a" || st.Bundles[0].Price != 7100 || st.Bundles[0].Base != 8700 {
		t.Fatalf("bundles %+v", st.Bundles)
	}
	seen := seenPrices(st)
	if seen["x"] != 1775 || seen["l1"] != 1775 || seen["l2"] != 875 || seen["n1"] != 1775 {
		t.Fatalf("seen %+v", seen)
	}
	p := &Prices{path: t.TempDir() + "/p.json", VP: map[string]int{}, Seen: map[string]int{}}
	p.Merge(seen)
	if p.Get("L2") != 875 || p.Ready() {
		t.Fatal("prices merge")
	}
}

func TestReset(t *testing.T) {
	kst := time.FixedZone("KST", 9*3600)
	n := nextDailyReset(time.Date(2026, 10, 24, 8, 59, 0, 0, kst))
	if !n.Equal(time.Date(2026, 10, 24, 9, 0, 0, 0, kst)) {
		t.Fatalf("got %v", n.In(kst))
	}
	n = nextDailyReset(time.Date(2026, 10, 24, 9, 0, 0, 0, kst))
	if !n.Equal(time.Date(2026, 10, 25, 9, 0, 0, 0, kst)) {
		t.Fatalf("got %v", n.In(kst))
	}
}

func TestJosaShard(t *testing.T) {
	if josa("네온 회로 팬텀") != "이" || josa("크로마 오퍼레이터") != "가" || josa("VCT") != "이(가)" {
		t.Fatal("josa")
	}
	if shardFor("latam") != "na" || shardFor("KR") != "kr" || shardFor("ap") != "ap" || shardFor("eu") != "eu" {
		t.Fatal("shard")
	}
}

func TestParseOffers(t *testing.T) {
	m, err := parseOffers([]byte(`{"Offers":[
	  {"OfferID":"AA","Cost":{"85ad13f7-3d1b-5128-9eb2-7cd8ee0b5741":1775},"Rewards":[{"ItemTypeID":"e7","ItemID":"LVL1","Quantity":1}]},
	  {"OfferID":"BB","Cost":{"e59aa87c-4cbf-517a-5983-6e81511be9b7":10},"Rewards":[{"ItemID":"RPONLY"}]}]}`))
	if err != nil || m["lvl1"] != 1775 || len(m) != 1 {
		t.Fatalf("%v %v", m, err)
	}
}
