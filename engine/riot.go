package main

// 발로란트 게임 클라이언트가 쓰는 (비공식) Riot API 호출.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const chromeUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"

// 발로란트 클라이언트가 보내는 플랫폼 정보 (base64 JSON, 커뮤니티에서 쓰는 값)
const clientPlatform = "ew0KCSJwbGF0Zm9ybVR5cGUiOiAiUEMiLA0KCSJwbGF0Zm9ybU9TIjogIldpbmRvd3MiLA0KCSJwbGF0Zm9ybU9TVmVyc2lvbiI6ICIxMC4wLjE5MDQyLjEuMjU2LjY0Yml0IiwNCgkicGxhdGZvcm1DaGlwc2V0IjogIlVua25vd24iDQp9"

const (
	currencyVP = "85ad13f7-3d1b-5128-9eb2-7cd8ee0b5741"
	currencyRP = "e59aa87c-4cbf-517a-5983-6e81511be9b7"
	currencyKC = "85ca954a-41f2-ce94-9b45-8ca3dd39a00d"

	itemTypeSkin  = "e7c63390-eda7-46e0-bb7a-a6abdacd2433"
	itemTypeBuddy = "dd3bf334-87f3-40bd-b043-682a57a8dc3a"
	itemTypeCard  = "3f296c07-64c3-494c-923b-fe692a4fa1bd"
	itemTypeSpray = "d5f120f8-ff8c-4aac-92ea-f2b5acbe9475"
	itemTypeTitle = "de7caa6b-adf7-4588-bbd1-143831e786c6"
	itemTypeFlex  = "03a572de-4234-31ed-d344-ababa488f981"
	itemTypeCharm = "77258665-71d1-4623-bc72-44db9bd5b3b3"
)

var errSessionExpired = errors.New("로그인이 만료됐어요. 다시 로그인해 주세요")
var errTokenInvalid = errors.New("token invalid")
var errBlocked = errors.New("blocked")

type riotConfig struct {
	authBase     string
	entURL       string
	geoURL       string
	pdTmpl       string
	redirectURI  string
	loginMarker  string
	cookieDomain string
}

var riot = func() riotConfig {
	if m := strings.TrimRight(os.Getenv("VALOROAD_MOCK"), "/"); m != "" {
		u, _ := url.Parse(m)
		return riotConfig{
			authBase:     m,
			entURL:       m + "/ent/api/token/v1",
			geoURL:       m + "/geo/pas/v1/product/valorant",
			pdTmpl:       m + "/pd/%s",
			redirectURI:  m + "/opt_in",
			loginMarker:  "/login",
			cookieDomain: u.Hostname(),
		}
	}
	return riotConfig{
		authBase:     "https://auth.riotgames.com",
		entURL:       "https://entitlements.auth.riotgames.com/api/token/v1",
		geoURL:       "https://riot-geo.pas.si.riotgames.com/pas/v1/product/valorant",
		pdTmpl:       "https://pd.%s.a.pvp.net",
		redirectURI:  "https://playvalorant.com/opt_in",
		loginMarker:  "authenticate.riotgames.com",
		cookieDomain: "riotgames.com",
	}
}()

func (r riotConfig) authorizeURL() string {
	return r.authBase + "/authorize?redirect_uri=" + url.QueryEscape(r.redirectURI) +
		"&client_id=play-valorant-web-prod&response_type=token%20id_token&nonce=1&scope=account%20openid"
}

func (r riotConfig) pd(shard string) string { return fmt.Sprintf(r.pdTmpl, shard) }

func cookieForRiot(domain string) bool {
	d := strings.TrimPrefix(strings.ToLower(domain), ".")
	return d == riot.cookieDomain || strings.HasSuffix(d, "."+riot.cookieDomain)
}

// cookieForHost reports whether a stored cookie should be sent to host.
func cookieForHost(c Cookie, host string) bool {
	d := strings.TrimPrefix(strings.ToLower(c.Domain), ".")
	host = strings.ToLower(host)
	if d == "" || host == d || strings.HasSuffix(host, "."+d) {
		return c.Expires <= 0 || int64(c.Expires) > time.Now().Unix()
	}
	return false
}

var httpc = &http.Client{
	Timeout: 20 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// cookieReauth asks Riot for a fresh token with the saved login cookies (no password involved).
func cookieReauth(cookies []Cookie) (*authResult, []Cookie, error) {
	u := riot.authorizeURL()
	pu, _ := url.Parse(u)
	req, _ := http.NewRequest("GET", u, nil)
	parts := []string{}
	for _, c := range cookies {
		if cookieForHost(c, pu.Hostname()) {
			parts = append(parts, c.Name+"="+c.Value)
		}
	}
	if len(parts) == 0 {
		return nil, cookies, errSessionExpired
	}
	req.Header.Set("Cookie", strings.Join(parts, "; "))
	req.Header.Set("User-Agent", chromeUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ko-KR,ko;q=0.9,en-US;q=0.8,en;q=0.7")
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, cookies, fmt.Errorf("Riot 서버에 연결하지 못했어요. 인터넷 연결을 확인해 주세요")
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	cookies = mergeCookies(cookies, resp.Cookies(), pu.Hostname())
	loc := resp.Header.Get("Location")
	if resp.StatusCode >= 300 && resp.StatusCode < 400 && loc != "" {
		if res, ok := parseTokenURL(loc); ok {
			res.Cookies = cookies
			return res, cookies, nil
		}
		if isLoginPage(loc) {
			return nil, cookies, errSessionExpired
		}
	}
	log.Printf("cookie reauth: unexpected %d %q", resp.StatusCode, loc)
	return nil, cookies, errBlocked
}

func mergeCookies(old []Cookie, fresh []*http.Cookie, host string) []Cookie {
	out := append([]Cookie(nil), old...)
	for _, f := range fresh {
		dom := f.Domain
		if dom == "" {
			dom = host
		}
		var exp float64
		if !f.Expires.IsZero() {
			exp = float64(f.Expires.Unix())
		}
		if f.MaxAge > 0 {
			exp = float64(time.Now().Unix() + int64(f.MaxAge))
		}
		nc := Cookie{Name: f.Name, Value: f.Value, Domain: dom, Path: f.Path, Expires: exp, Secure: f.Secure, HTTPOnly: f.HttpOnly}
		replaced := false
		for i, c := range out {
			if c.Name == nc.Name && strings.TrimPrefix(c.Domain, ".") == strings.TrimPrefix(nc.Domain, ".") {
				if f.MaxAge < 0 {
					out = append(out[:i], out[i+1:]...)
				} else {
					out[i] = nc
				}
				replaced = true
				break
			}
		}
		if !replaced && f.MaxAge >= 0 {
			out = append(out, nc)
		}
	}
	return out
}

// ---------------------------------------------------------------- authenticated calls

type riotSession struct {
	Access  string    `json:"access"`
	IDToken string    `json:"idToken"`
	Ent     string    `json:"ent"`
	Expires time.Time `json:"expires"`
	PUUID   string    `json:"puuid"`
	Shard   string    `json:"shard"`
	Region  string    `json:"region"`
}

func (s *riotSession) valid() bool {
	return s != nil && s.Access != "" && s.Ent != "" && time.Until(s.Expires) > 3*time.Minute
}

type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string { return fmt.Sprintf("HTTP %d", e.Status) }

func riotDo(method, u string, headers map[string]string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, u, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "ShooterGame/13 Windows/10.0.19042.1.256.64bit")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return fmt.Errorf("Riot 서버에 연결하지 못했어요. 인터넷 연결을 확인해 주세요")
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode == 401 || (resp.StatusCode == 400 && bytes.Contains(b, []byte("BAD_CLAIMS"))) {
		return errTokenInvalid
	}
	if resp.StatusCode >= 300 {
		return &apiError{resp.StatusCode, string(b)}
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("Riot 응답을 읽지 못했어요")
		}
	}
	return nil
}

func bearer(tok string) map[string]string { return map[string]string{"Authorization": "Bearer " + tok} }

func fetchEntitlement(access string) (string, error) {
	var out struct {
		Token string `json:"entitlements_token"`
	}
	if err := riotDo("POST", riot.entURL, bearer(access), map[string]any{}, &out); err != nil {
		return "", err
	}
	return out.Token, nil
}

type userInfo struct {
	Sub  string `json:"sub"`
	Acct struct {
		GameName string `json:"game_name"`
		TagLine  string `json:"tag_line"`
	} `json:"acct"`
}

func fetchUserInfo(access string) (*userInfo, error) {
	var out userInfo
	if err := riotDo("GET", riot.authBase+"/userinfo", bearer(access), nil, &out); err != nil {
		return nil, err
	}
	if out.Sub == "" {
		return nil, errors.New("계정 정보를 받지 못했어요")
	}
	return &out, nil
}

func regionToShard(region string) string {
	switch region {
	case "latam", "br", "na":
		return "na"
	case "kr", "ap", "eu":
		return region
	case "pbe":
		return "pbe"
	}
	return "ap"
}

func fetchRegion(access, idToken string) (string, error) {
	var out struct {
		Affinities struct {
			Live string `json:"live"`
		} `json:"affinities"`
	}
	if err := riotDo("PUT", riot.geoURL, bearer(access), map[string]any{"id_token": idToken}, &out); err != nil {
		return "", err
	}
	if out.Affinities.Live == "" {
		return "", errors.New("계정 지역을 알 수 없어요")
	}
	return out.Affinities.Live, nil
}

func (s *riotSession) headers() map[string]string {
	return map[string]string{
		"Authorization":           "Bearer " + s.Access,
		"X-Riot-Entitlements-JWT": s.Ent,
		"X-Riot-ClientPlatform":   clientPlatform,
		"X-Riot-ClientVersion":    clientVersion(),
	}
}

// ---------------------------------------------------------------- store

type riotOffer struct {
	OfferID string         `json:"OfferID"`
	Cost    map[string]int `json:"Cost"`
	Rewards []struct {
		ItemTypeID string `json:"ItemTypeID"`
		ItemID     string `json:"ItemID"`
		Quantity   int    `json:"Quantity"`
	} `json:"Rewards"`
}

type riotStorefront struct {
	FeaturedBundle struct {
		Bundles []struct {
			ID          string `json:"ID"`
			DataAssetID string `json:"DataAssetID"`
			CurrencyID  string `json:"CurrencyID"`
			Items       []struct {
				Item struct {
					ItemTypeID string `json:"ItemTypeID"`
					ItemID     string `json:"ItemID"`
					Amount     int    `json:"Amount"`
				} `json:"Item"`
				BasePrice       int     `json:"BasePrice"`
				DiscountPercent float64 `json:"DiscountPercent"`
				DiscountedPrice int     `json:"DiscountedPrice"`
			} `json:"Items"`
			ItemOffers []struct {
				Offer           riotOffer      `json:"Offer"`
				DiscountPercent float64        `json:"DiscountPercent"`
				DiscountedCost  map[string]int `json:"DiscountedCost"`
			} `json:"ItemOffers"`
			TotalBaseCost              map[string]int `json:"TotalBaseCost"`
			TotalDiscountedCost        map[string]int `json:"TotalDiscountedCost"`
			TotalDiscountPercent       float64        `json:"TotalDiscountPercent"`
			DurationRemainingInSeconds int            `json:"DurationRemainingInSeconds"`
		} `json:"Bundles"`
	} `json:"FeaturedBundle"`
	SkinsPanelLayout struct {
		SingleItemOffers                           []string    `json:"SingleItemOffers"`
		SingleItemStoreOffers                      []riotOffer `json:"SingleItemStoreOffers"`
		SingleItemOffersRemainingDurationInSeconds int         `json:"SingleItemOffersRemainingDurationInSeconds"`
	} `json:"SkinsPanelLayout"`
	BonusStore *struct {
		BonusStoreOffers []struct {
			BonusOfferID    string         `json:"BonusOfferID"`
			Offer           riotOffer      `json:"Offer"`
			DiscountPercent float64        `json:"DiscountPercent"`
			DiscountCosts   map[string]int `json:"DiscountCosts"`
			IsSeen          bool           `json:"IsSeen"`
		} `json:"BonusStoreOffers"`
		BonusStoreRemainingDurationInSeconds int `json:"BonusStoreRemainingDurationInSeconds"`
	} `json:"BonusStore"`
}

func fetchStorefront(s *riotSession) (*riotStorefront, error) {
	var out riotStorefront
	err := riotDo("POST", riot.pd(s.Shard)+"/store/v3/storefront/"+s.PUUID, s.headers(), map[string]any{}, &out)
	if ae, ok := err.(*apiError); ok && (ae.Status == 404 || ae.Status == 405) {
		// 예전 방식 (v2, GET)
		err = riotDo("GET", riot.pd(s.Shard)+"/store/v2/storefront/"+s.PUUID, s.headers(), nil, &out)
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func fetchWallet(s *riotSession) (map[string]int, error) {
	var out struct {
		Balances map[string]int `json:"Balances"`
	}
	if err := riotDo("GET", riot.pd(s.Shard)+"/store/v1/wallet/"+s.PUUID, s.headers(), nil, &out); err != nil {
		return nil, err
	}
	return out.Balances, nil
}

func fetchOwnedSkins(s *riotSession) ([]string, error) {
	var out struct {
		Entitlements []struct {
			ItemID string `json:"ItemID"`
		} `json:"Entitlements"`
	}
	if err := riotDo("GET", riot.pd(s.Shard)+"/store/v1/entitlements/"+s.PUUID+"/"+itemTypeSkin, s.headers(), nil, &out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Entitlements))
	for _, e := range out.Entitlements {
		ids = append(ids, strings.ToLower(e.ItemID))
	}
	return ids, nil
}

func fetchAccountLevel(s *riotSession) (int, error) {
	var out struct {
		Progress struct {
			Level int `json:"Level"`
		} `json:"Progress"`
	}
	if err := riotDo("GET", riot.pd(s.Shard)+"/account-xp/v1/players/"+s.PUUID, s.headers(), nil, &out); err != nil {
		return 0, err
	}
	return out.Progress.Level, nil
}

func fetchPlayerCard(s *riotSession) (string, error) {
	var out struct {
		Identity struct {
			PlayerCardID string `json:"PlayerCardID"`
		} `json:"Identity"`
	}
	if err := riotDo("GET", riot.pd(s.Shard)+"/personalization/v2/players/"+s.PUUID+"/playerloadout", s.headers(), nil, &out); err != nil {
		return "", err
	}
	return strings.ToLower(out.Identity.PlayerCardID), nil
}

// ---------------------------------------------------------------- client version

var cv struct {
	sync.Mutex
	v  string
	at time.Time
}

func clientVersion() string {
	cv.Lock()
	defer cv.Unlock()
	if cv.v != "" && time.Since(cv.at) < 6*time.Hour {
		return cv.v
	}
	var out struct {
		Data struct {
			RiotClientVersion string `json:"riotClientVersion"`
		} `json:"data"`
	}
	c := &http.Client{Timeout: 10 * time.Second}
	if resp, err := c.Get(valAPI + "/v1/version"); err == nil {
		json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
	}
	if out.Data.RiotClientVersion != "" {
		cv.v, cv.at = out.Data.RiotClientVersion, time.Now()
	}
	if cv.v == "" {
		return "release-13.06-shipping-18-5590001"
	}
	return cv.v
}
