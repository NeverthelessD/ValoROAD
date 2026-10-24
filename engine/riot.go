package main

// 라이엇 연결 (공식 API 가 아니라서 바뀔 수 있어요 — PROJECT_NOTES.md 「라이엇 연결」)
//
// 맥에서는 riotgames.com 으로 가는 요청을 앱 껍데기의 도우미(`ValoROAD --riot-fetch`, 애플 URLSession)로 보내요.
// 라이엇이 브라우저가 아닌 프로그램의 연결을 막는 경우가 있어서, 맥의 사파리와 같은 연결 방식을 쓰려는 거예요.
// 상점(pvp.net)과 스킨 정보(valorant-api.com)는 엔진이 직접 받아요.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const riotAuthorizeURL = "https://auth.riotgames.com/authorize?redirect_uri=https%3A%2F%2Fplayvalorant.com%2Fopt_in&client_id=play-valorant-web-prod&response_type=token%20id_token&nonce=1&scope=account%20openid"

// X-Riot-ClientPlatform: 라이엇 클라이언트가 보내는 플랫폼 정보(base64). 널리 쓰이는 값 그대로예요.
var riotClientPlatform = base64.StdEncoding.EncodeToString([]byte("{\r\n\t\"platformType\": \"PC\",\r\n\t\"platformOS\": \"Windows\",\r\n\t\"platformOSVersion\": \"10.0.19042.1.256.64bit\",\r\n\t\"platformChipset\": \"Unknown\"\r\n}"))

const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15"

var (
	errExpired  = errors.New("로그인이 만료됐어요. 이 계정만 다시 로그인해 주세요")
	errNoAccess = errors.New("라이엇 서버가 요청을 받아 주지 않았어요")
)

type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Expires  int64  `json:"expires,omitempty"` // 유닉스 초, 0 = 세션 쿠키
	Secure   bool   `json:"secure,omitempty"`
	HTTPOnly bool   `json:"httpOnly,omitempty"`
}

type riotReq struct {
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body,omitempty"`
	NoRedirect bool              `json:"noRedirect"`
}

type riotResp struct {
	Status   int      `json:"status"`
	Location string   `json:"location"`
	Cookies  []Cookie `json:"cookies"`
	Body     []byte   `json:"body"`
	Error    string   `json:"error"`
}

// ---------------------------------------------------------------- 연결 방식

func shellHelper() string {
	if runtime.GOOS != "darwin" || env("RIOT_DIRECT") == "1" {
		return ""
	}
	p := env("APP")
	if p == "" {
		return ""
	}
	h := filepath.Join(p, "Contents", "MacOS", "ValoROAD")
	if fileExists(h) {
		return h
	}
	return ""
}

func riotTransportName() string {
	if shellHelper() != "" {
		return "앱 도우미(URLSession) + 엔진"
	}
	return "엔진 직접"
}

func useHelper(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return strings.HasSuffix(u.Hostname(), ".riotgames.com") && shellHelper() != ""
}

func riotDo(req riotReq) (*riotResp, error) {
	if req.Headers == nil {
		req.Headers = map[string]string{}
	}
	if useHelper(req.URL) {
		return helperDo(req)
	}
	return goDo(req)
}

func helperDo(req riotReq) (*riotResp, error) {
	in, _ := json.Marshal(req)
	cmd := exec.Command(shellHelper(), "--riot-fetch")
	cmd.Stdin = bytes.NewReader(in)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return goDo(req)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		cmd.Process.Kill()
		return nil, errors.New("라이엇 서버가 응답하지 않아요")
	}
	var resp riotResp
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("라이엇 연결 도우미가 이상해요: %s", lastLine(errb.String()))
	}
	if resp.Error != "" {
		return nil, errors.New("라이엇 서버에 연결할 수 없어요. 인터넷 연결을 확인해 주세요")
	}
	return &resp, nil
}

var httpClient = &http.Client{Timeout: 25 * time.Second}

func goDo(req riotReq) (*riotResp, error) {
	var body io.Reader
	if req.Body != nil {
		body = bytes.NewReader(req.Body)
	}
	hr, err := http.NewRequest(req.Method, req.URL, body)
	if err != nil {
		return nil, err
	}
	for k, v := range req.Headers {
		hr.Header.Set(k, v)
	}
	client := httpClient
	if req.NoRedirect {
		c := *httpClient
		c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		client = &c
	}
	resp, err := client.Do(hr)
	if err != nil {
		return nil, errors.New("라이엇 서버에 연결할 수 없어요. 인터넷 연결을 확인해 주세요")
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	out := &riotResp{Status: resp.StatusCode, Location: resp.Header.Get("Location"), Body: b}
	for _, c := range resp.Cookies() {
		d := c.Domain
		if d == "" {
			d = hr.URL.Hostname()
		}
		ck := Cookie{Name: c.Name, Value: c.Value, Domain: d, Path: c.Path, Secure: c.Secure, HTTPOnly: c.HttpOnly}
		if !c.Expires.IsZero() {
			ck.Expires = c.Expires.Unix()
		}
		if c.MaxAge < 0 {
			ck.Expires = 1
		}
		out.Cookies = append(out.Cookies, ck)
	}
	return out, nil
}

// ---------------------------------------------------------------- 쿠키

func cookieDomainMatch(domain, host string) bool {
	d := strings.TrimPrefix(strings.ToLower(domain), ".")
	h := strings.ToLower(host)
	return h == d || strings.HasSuffix(h, "."+d)
}

func cookieHeader(cookies []Cookie, host string) string {
	now := time.Now().Unix()
	var parts []string
	for _, c := range cookies {
		if c.Value == "" || (c.Expires > 0 && c.Expires < now) || !cookieDomainMatch(c.Domain, host) {
			continue
		}
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

// mergeCookies 는 새로 받은 쿠키로 바꾸거나 더하고, 만료된 것은 빼요.
func mergeCookies(old, fresh []Cookie) []Cookie {
	key := func(c Cookie) string {
		p := c.Path
		if p == "" {
			p = "/"
		}
		return c.Name + "|" + strings.TrimPrefix(strings.ToLower(c.Domain), ".") + "|" + p
	}
	now := time.Now().Unix()
	idx := map[string]int{}
	out := []Cookie{}
	for _, c := range old {
		if c.Expires > 0 && c.Expires < now {
			continue
		}
		idx[key(c)] = len(out)
		out = append(out, c)
	}
	for _, c := range fresh {
		k := key(c)
		dead := c.Value == "" || (c.Expires > 0 && c.Expires < now)
		if i, ok := idx[k]; ok {
			if dead {
				out[i].Value = ""
			} else {
				out[i] = c
			}
			continue
		}
		if !dead {
			idx[k] = len(out)
			out = append(out, c)
		}
	}
	kept := out[:0]
	for _, c := range out {
		if c.Value != "" {
			kept = append(kept, c)
		}
	}
	return kept
}

// onlyRiotCookies 는 riotgames.com 쿠키만 남겨요 (로그인 창의 다른 사이트 쿠키는 버려요).
func onlyRiotCookies(in []Cookie) []Cookie {
	out := []Cookie{}
	for _, c := range in {
		d := strings.TrimPrefix(strings.ToLower(c.Domain), ".")
		if d == "riotgames.com" || strings.HasSuffix(d, ".riotgames.com") {
			out = append(out, c)
		}
	}
	return out
}

// ---------------------------------------------------------------- 토큰

type tokens struct {
	Access  string
	ID      string
	Expires time.Time
}

// parseTokenLocation: https://playvalorant.com/opt_in#access_token=…&id_token=…&expires_in=3600
func parseTokenLocation(loc string) (tokens, bool) {
	u, err := url.Parse(loc)
	if err != nil {
		return tokens{}, false
	}
	frag := u.Fragment
	if frag == "" {
		frag = u.RawQuery
	}
	q, err := url.ParseQuery(frag)
	if err != nil || q.Get("access_token") == "" {
		return tokens{}, false
	}
	exp := 3600
	fmt.Sscanf(q.Get("expires_in"), "%d", &exp)
	if exp <= 0 {
		exp = 3600
	}
	return tokens{Access: q.Get("access_token"), ID: q.Get("id_token"), Expires: time.Now().Add(time.Duration(exp) * time.Second)}, true
}

// reauth: 저장한 쿠키로 새 토큰을 받아요. 쿠키가 만료됐으면 errExpired.
func reauth(cookies []Cookie) (tokens, []Cookie, error) {
	resp, err := riotDo(riotReq{
		Method:     "GET",
		URL:        riotAuthorizeURL,
		NoRedirect: true,
		Headers: map[string]string{
			"Cookie":          cookieHeader(cookies, "auth.riotgames.com"),
			"User-Agent":      browserUA,
			"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			"Accept-Language": "ko-KR,ko;q=0.9",
		},
	})
	if err != nil {
		return tokens{}, cookies, err
	}
	merged := mergeCookies(cookies, resp.Cookies)
	if t, ok := parseTokenLocation(resp.Location); ok {
		return t, merged, nil
	}
	if resp.Status >= 300 && resp.Status < 400 {
		return tokens{}, merged, errExpired
	}
	if resp.Status == 403 || resp.Status == 429 {
		return tokens{}, merged, fmt.Errorf("%w (HTTP %d) — 잠시 뒤 다시 시도해 주세요", errNoAccess, resp.Status)
	}
	return tokens{}, merged, errExpired
}

type userInfo struct {
	PUUID string
	Name  string
	Tag   string
}

func fetchUserInfo(access string) (userInfo, error) {
	resp, err := riotDo(riotReq{Method: "GET", URL: "https://auth.riotgames.com/userinfo", Headers: map[string]string{
		"Authorization": "Bearer " + access, "User-Agent": browserUA,
	}})
	if err != nil {
		return userInfo{}, err
	}
	if resp.Status == 401 || resp.Status == 403 {
		return userInfo{}, errExpired
	}
	if resp.Status != 200 {
		return userInfo{}, fmt.Errorf("계정 정보를 받지 못했어요 (HTTP %d)", resp.Status)
	}
	var v struct {
		Sub  string `json:"sub"`
		Acct struct {
			GameName string `json:"game_name"`
			TagLine  string `json:"tag_line"`
		} `json:"acct"`
	}
	if json.Unmarshal(resp.Body, &v) != nil || v.Sub == "" {
		return userInfo{}, errors.New("계정 정보를 읽지 못했어요")
	}
	return userInfo{PUUID: v.Sub, Name: v.Acct.GameName, Tag: v.Acct.TagLine}, nil
}

func fetchEntitlements(access string) (string, error) {
	resp, err := riotDo(riotReq{Method: "POST", URL: "https://entitlements.auth.riotgames.com/api/token/v1", Body: []byte("{}"), Headers: map[string]string{
		"Authorization": "Bearer " + access, "Content-Type": "application/json", "User-Agent": browserUA,
	}})
	if err != nil {
		return "", err
	}
	if resp.Status == 401 || resp.Status == 403 {
		return "", errExpired
	}
	var v struct {
		Token string `json:"entitlements_token"`
	}
	if resp.Status != 200 || json.Unmarshal(resp.Body, &v) != nil || v.Token == "" {
		return "", fmt.Errorf("게임 이용 정보를 받지 못했어요 (HTTP %d)", resp.Status)
	}
	return v.Token, nil
}

// fetchRegion: 계정의 지역(kr · ap · eu · na · latam · br)
func fetchRegion(access, idToken string) (string, error) {
	body, _ := json.Marshal(map[string]string{"id_token": idToken})
	resp, err := riotDo(riotReq{Method: "PUT", URL: "https://riot-geo.pas.si.riotgames.com/pas/v1/product/valorant", Body: body, Headers: map[string]string{
		"Authorization": "Bearer " + access, "Content-Type": "application/json", "User-Agent": browserUA,
	}})
	if err != nil {
		return "", err
	}
	var v struct {
		Affinities map[string]string `json:"affinities"`
	}
	if resp.Status != 200 || json.Unmarshal(resp.Body, &v) != nil || v.Affinities["live"] == "" {
		return "", fmt.Errorf("계정의 지역을 알지 못했어요 (HTTP %d)", resp.Status)
	}
	return strings.ToLower(v.Affinities["live"]), nil
}

func shardFor(region string) string {
	switch strings.ToLower(region) {
	case "latam", "br", "na":
		return "na"
	case "pbe":
		return "pbe"
	case "eu":
		return "eu"
	case "kr":
		return "kr"
	default:
		return "ap"
	}
}

// ---------------------------------------------------------------- 상점 · 지갑

type session struct {
	Access  string
	ID      string
	Ent     string
	Expires time.Time
}

func (s *session) valid() bool {
	return s != nil && s.Access != "" && s.Ent != "" && time.Until(s.Expires) > 5*time.Minute
}

func pdHeaders(s *session) map[string]string {
	return map[string]string{
		"Authorization":           "Bearer " + s.Access,
		"X-Riot-Entitlements-JWT": s.Ent,
		"X-Riot-ClientPlatform":   riotClientPlatform,
		"X-Riot-ClientVersion":    app.content.ClientVersion(),
		"Content-Type":            "application/json",
	}
}

func pdGet(s *session, shard, method, path string, body []byte) ([]byte, error) {
	resp, err := riotDo(riotReq{Method: method, URL: "https://pd." + shard + ".a.pvp.net" + path, Body: body, Headers: pdHeaders(s)})
	if err != nil {
		return nil, err
	}
	switch {
	case resp.Status == 200:
		return resp.Body, nil
	case resp.Status == 400 || resp.Status == 401 || resp.Status == 403:
		return nil, errTokenRejected
	case resp.Status == 429:
		return nil, errors.New("라이엇 서버가 잠시 쉬어 달래요. 1분 뒤 다시 시도해 주세요")
	default:
		return nil, fmt.Errorf("상점 정보를 받지 못했어요 (HTTP %d)", resp.Status)
	}
}

var errTokenRejected = errors.New("토큰이 거절됐어요")

type rawStorefront struct {
	FeaturedBundle struct {
		Bundles []struct {
			ID                         string         `json:"ID"`
			DataAssetID                string         `json:"DataAssetID"`
			TotalBaseCost              map[string]int `json:"TotalBaseCost"`
			TotalDiscountedCost        map[string]int `json:"TotalDiscountedCost"`
			DurationRemainingInSeconds int64          `json:"DurationRemainingInSeconds"`
			Items                      []struct {
				Item struct {
					ItemTypeID string `json:"ItemTypeID"`
					ItemID     string `json:"ItemID"`
				} `json:"Item"`
				BasePrice       int `json:"BasePrice"`
				DiscountedPrice int `json:"DiscountedPrice"`
			} `json:"Items"`
		} `json:"Bundles"`
		BundleRemainingDurationInSeconds int64 `json:"BundleRemainingDurationInSeconds"`
	} `json:"FeaturedBundle"`
	SkinsPanelLayout struct {
		SingleItemOffers      []string `json:"SingleItemOffers"`
		SingleItemStoreOffers []struct {
			OfferID string         `json:"OfferID"`
			Cost    map[string]int `json:"Cost"`
			Rewards []struct {
				ItemTypeID string `json:"ItemTypeID"`
				ItemID     string `json:"ItemID"`
			} `json:"Rewards"`
		} `json:"SingleItemStoreOffers"`
		Remaining int64 `json:"SingleItemOffersRemainingDurationInSeconds"`
	} `json:"SkinsPanelLayout"`
	BonusStore *struct {
		BonusStoreOffers []struct {
			Offer struct {
				Cost    map[string]int `json:"Cost"`
				Rewards []struct {
					ItemID string `json:"ItemID"`
				} `json:"Rewards"`
			} `json:"Offer"`
			DiscountPercent int            `json:"DiscountPercent"`
			DiscountCosts   map[string]int `json:"DiscountCosts"`
			IsSeen          bool           `json:"IsSeen"`
		} `json:"BonusStoreOffers"`
		Remaining int64 `json:"BonusStoreRemainingDurationInSeconds"`
	} `json:"BonusStore"`
}

const (
	currencyVP = "85ad13f7-3d1b-5128-9eb2-7cd8ee0b5741"
	currencyRP = "e59aa87c-4cbf-517a-5983-6e81511be9b7"
	currencyKC = "85ca954a-41f2-ce94-9b45-8ca3dd39a00d"
)

func fetchStorefront(s *session, shard, puuid string) (*rawStorefront, error) {
	b, err := pdGet(s, shard, "POST", "/store/v3/storefront/"+puuid, []byte("{}"))
	if err != nil {
		return nil, err
	}
	var sf rawStorefront
	if err := json.Unmarshal(b, &sf); err != nil {
		return nil, errors.New("상점 정보를 읽지 못했어요")
	}
	return &sf, nil
}

func fetchWallet(s *session, shard, puuid string) (map[string]int, error) {
	b, err := pdGet(s, shard, "GET", "/store/v1/wallet/"+puuid, nil)
	if err != nil {
		return nil, err
	}
	var v struct {
		Balances map[string]int `json:"Balances"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, errors.New("지갑 정보를 읽지 못했어요")
	}
	return map[string]int{"vp": v.Balances[currencyVP], "rp": v.Balances[currencyRP], "kc": v.Balances[currencyKC]}, nil
}
