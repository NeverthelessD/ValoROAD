package main

// 스킨 정보 (이름 · 무기 · 등급 · 이미지) — valorant-api.com (커뮤니티가 운영하는 공개 자료)
// 게임 버전이 바뀔 때만 다시 받아서 Support/content/content.json 에 줄여서 저장해요.
// 이미지는 media.valorant-api.com 에서 받아 Support/cache/img 에 저장해 두고 보여 줘요 (/api/img).

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	vapiBase           = "https://valorant-api.com/v1/"
	mediaPrefix        = "https://media.valorant-api.com/"
	fallbackClientVers = "release-13.08-shipping-20-5715960"
)

type Skin struct {
	ID       string `json:"id"`    // 스킨 uuid (위시리스트에 저장)
	Level    string `json:"level"` // 1레벨 uuid (상점의 ItemID)
	Name     string `json:"name"`
	Weapon   string `json:"weapon"`
	WeaponEn string `json:"weaponEn"`
	Cat      string `json:"cat"`  // rifle sidearm sniper smg shotgun heavy melee
	Tier     string `json:"tier"` // select deluxe premium exclusive ultra ("" = 기본 스킨)
	Img      string `json:"img"`
}

type BundleInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Img  string `json:"img"`
}

// 미리보기 (게임 안에서 아이템을 누르면 보이는 것): 레벨별 영상 · 변형(크로마)별 이미지
type Chroma struct {
	Name   string `json:"name"`
	Img    string `json:"img"`
	Swatch string `json:"swatch,omitempty"`
	Video  string `json:"video,omitempty"`
}

type Level struct {
	Name  string `json:"name"`
	Item  string `json:"item"` // vfx animation finisher soundeffects … ("" = 기본)
	Video string `json:"video,omitempty"`
	Img   string `json:"img,omitempty"`
}

type SkinDetail struct {
	Chromas []Chroma `json:"chromas"`
	Levels  []Level  `json:"levels"`
}

// contentFormat: 저장 형식이 바뀌면 올려요 (예전 형식이면 같은 게임 버전이어도 다시 받아요)
const contentFormat = 2

type contentData struct {
	Format        int                   `json:"format"`
	Details       map[string]SkinDetail `json:"details"`
	Version       string                `json:"version"`
	ClientVersion string                `json:"clientVersion"`
	Fetched       int64                 `json:"fetched"`
	Skins         []Skin                `json:"skins"`
	Levels        map[string]string     `json:"levels"` // 모든 레벨 uuid → 스킨 uuid
	Bundles       map[string]BundleInfo `json:"bundles"`
}

type Content struct {
	mu      sync.RWMutex
	support string
	data    *contentData
	byID    map[string]*Skin
	err     string
	loading bool
	waiters []chan struct{}
}

func NewContent(support string) *Content {
	c := &Content{support: support}
	if b, err := os.ReadFile(c.path()); err == nil {
		var d contentData
		if json.Unmarshal(b, &d) == nil && len(d.Skins) > 0 {
			c.set(&d)
		}
	}
	return c
}

func (c *Content) path() string { return filepath.Join(c.support, "content", "content.json") }

func (c *Content) set(d *contentData) {
	by := make(map[string]*Skin, len(d.Skins))
	for i := range d.Skins {
		by[d.Skins[i].ID] = &d.Skins[i]
	}
	if d.Levels == nil {
		d.Levels = map[string]string{}
	}
	c.mu.Lock()
	c.data, c.byID = d, by
	c.mu.Unlock()
}

func (c *Content) Ready() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data != nil
}

// WaitReady: 스킨 정보가 준비될 때까지 기다려요 (처음 실행 때 상점을 읽기 전에)
func (c *Content) WaitReady(d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if c.Ready() {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return c.Ready()
}

func (c *Content) Status() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := map[string]any{"ready": c.data != nil, "loading": c.loading, "error": c.err, "version": "", "count": 0}
	if c.data != nil {
		out["version"] = c.data.Version
		out["count"] = len(c.data.Skins)
	}
	return out
}

func (c *Content) ClientVersion() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.data != nil && c.data.ClientVersion != "" {
		return c.data.ClientVersion
	}
	return fallbackClientVers
}

// SkinByItem: 상점의 ItemID(레벨 uuid) 또는 스킨 uuid → 스킨
func (c *Content) SkinByItem(id string) *Skin {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.data == nil {
		return nil
	}
	id = strings.ToLower(id)
	if s, ok := c.byID[id]; ok {
		return s
	}
	if sid, ok := c.data.Levels[id]; ok {
		return c.byID[sid]
	}
	return nil
}

// chromaName: "프라임 밴달 4레벨\n(변형 1 주황색)" → "변형 1 주황색", 첫 번째는 "기본"
func chromaName(s string, i int) string {
	if a := strings.LastIndex(s, "("); a >= 0 {
		if b := strings.LastIndex(s, ")"); b > a {
			return strings.TrimSpace(s[a+1 : b])
		}
	}
	if i == 0 {
		return "기본"
	}
	return fmt.Sprintf("변형 %d", i)
}

func (c *Content) Detail(id string) (SkinDetail, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.data == nil || c.data.Details == nil {
		return SkinDetail{}, false
	}
	d, ok := c.data.Details[id]
	return d, ok
}

func (c *Content) Bundle(id string) (BundleInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.data == nil {
		return BundleInfo{}, false
	}
	b, ok := c.data.Bundles[strings.ToLower(id)]
	return b, ok
}

func normSearch(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), ""))
}

// Search: 위시리스트에 담을 수 있는 스킨(등급이 있는 것) 찾기
func (c *Content) Search(q, cat string, limit int) []Skin {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := []Skin{}
	if c.data == nil {
		return out
	}
	nq := normSearch(q)
	for _, s := range c.data.Skins {
		if s.Tier == "" || (cat != "" && cat != "all" && s.Cat != cat) {
			continue
		}
		if nq != "" && !strings.Contains(normSearch(s.Name), nq) && !strings.Contains(normSearch(s.WeaponEn), nq) && !strings.Contains(normSearch(s.Weapon), nq) {
			continue
		}
		out = append(out, s)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (c *Content) Random(n int, seed int64, tierOK func(string) bool) []Skin {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := []Skin{}
	if c.data == nil {
		return out
	}
	pool := []Skin{}
	for _, s := range c.data.Skins {
		if s.Tier != "" && s.Img != "" && (tierOK == nil || tierOK(s.Tier)) {
			pool = append(pool, s)
		}
	}
	if len(pool) == 0 {
		return out
	}
	x := uint64(seed)*6364136223846793005 + 1442695040888963407
	used := map[int]bool{}
	for len(out) < n && len(used) < len(pool) {
		x = x*6364136223846793005 + 1442695040888963407
		i := int((x >> 33) % uint64(len(pool)))
		if used[i] {
			continue
		}
		used[i] = true
		out = append(out, pool[i])
	}
	return out
}

// ---------------------------------------------------------------- 받기

func (c *Content) loop() {
	for {
		if err := c.Refresh(false); err != nil {
			log.Printf("content: %v", err)
			time.Sleep(2 * time.Minute)
			continue
		}
		time.Sleep(6 * time.Hour)
	}
}

func getJSON(u string, v any) error {
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "ValoROAD/"+build)
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return errors.New("스킨 정보를 받을 수 없어요. 인터넷 연결을 확인해 주세요")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("스킨 정보를 받지 못했어요 (HTTP %d)", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(v)
}

type vapiWeapon struct {
	UUID        string `json:"uuid"`
	DisplayName string `json:"displayName"`
	Category    string `json:"category"`
	Skins       []struct {
		UUID            string  `json:"uuid"`
		DisplayName     string  `json:"displayName"`
		ContentTierUUID *string `json:"contentTierUuid"`
		DisplayIcon     *string `json:"displayIcon"`
		Chromas         []struct {
			DisplayName   string  `json:"displayName"`
			DisplayIcon   *string `json:"displayIcon"`
			FullRender    *string `json:"fullRender"`
			Swatch        *string `json:"swatch"`
			StreamedVideo *string `json:"streamedVideo"`
		} `json:"chromas"`
		Levels []struct {
			UUID          string  `json:"uuid"`
			DisplayName   string  `json:"displayName"`
			LevelItem     *string `json:"levelItem"`
			DisplayIcon   *string `json:"displayIcon"`
			StreamedVideo *string `json:"streamedVideo"`
		} `json:"levels"`
	} `json:"skins"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func catOf(c string) string {
	c = strings.ToLower(strings.TrimPrefix(c, "EEquippableCategory::"))
	switch c {
	case "rifle", "sidearm", "sniper", "smg", "shotgun", "heavy", "melee":
		return c
	}
	return "other"
}

func (c *Content) Refresh(force bool) error {
	c.mu.Lock()
	if c.loading {
		c.mu.Unlock()
		return nil
	}
	c.loading = true
	cur := c.data
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.loading = false
		c.mu.Unlock()
	}()

	var ver struct {
		Data struct {
			Version           string `json:"version"`
			RiotClientVersion string `json:"riotClientVersion"`
		} `json:"data"`
	}
	if err := getJSON(vapiBase+"version", &ver); err != nil {
		c.setErr(err)
		return err
	}
	if !force && cur != nil && cur.Format == contentFormat && cur.Version == ver.Data.Version && cur.ClientVersion == ver.Data.RiotClientVersion {
		c.setErr(nil)
		return nil
	}
	var ko, en struct {
		Data []vapiWeapon `json:"data"`
	}
	var tiers struct {
		Data []struct {
			UUID    string `json:"uuid"`
			DevName string `json:"devName"`
		} `json:"data"`
	}
	var bundles struct {
		Data []struct {
			UUID        string  `json:"uuid"`
			DisplayName string  `json:"displayName"`
			DisplayIcon *string `json:"displayIcon"`
		} `json:"data"`
	}
	for _, step := range []struct {
		u string
		v any
	}{
		{vapiBase + "weapons?language=ko-KR", &ko},
		{vapiBase + "weapons?language=en-US", &en},
		{vapiBase + "contenttiers", &tiers},
		{vapiBase + "bundles?language=ko-KR", &bundles},
	} {
		if err := getJSON(step.u, step.v); err != nil {
			c.setErr(err)
			return err
		}
	}
	tierName := map[string]string{}
	for _, t := range tiers.Data {
		tierName[strings.ToLower(t.UUID)] = strings.ToLower(t.DevName)
	}
	weaponEn := map[string]string{}
	for _, w := range en.Data {
		weaponEn[w.UUID] = w.DisplayName
	}
	d := &contentData{Format: contentFormat, Version: ver.Data.Version, ClientVersion: ver.Data.RiotClientVersion, Fetched: time.Now().Unix(),
		Levels: map[string]string{}, Bundles: map[string]BundleInfo{}, Details: map[string]SkinDetail{}}
	for _, w := range ko.Data {
		for _, s := range w.Skins {
			sk := Skin{ID: strings.ToLower(s.UUID), Name: s.DisplayName, Weapon: w.DisplayName, WeaponEn: weaponEn[w.UUID],
				Cat: catOf(w.Category), Tier: tierName[strings.ToLower(str(s.ContentTierUUID))]}
			if len(s.Levels) > 0 {
				sk.Level = strings.ToLower(s.Levels[0].UUID)
				sk.Img = str(s.Levels[0].DisplayIcon)
			}
			if sk.Img == "" {
				sk.Img = str(s.DisplayIcon)
			}
			if sk.Img == "" && len(s.Chromas) > 0 {
				sk.Img = str(s.Chromas[0].FullRender)
				if sk.Img == "" {
					sk.Img = str(s.Chromas[0].DisplayIcon)
				}
			}
			det := SkinDetail{Chromas: []Chroma{}, Levels: []Level{}}
			for _, l := range s.Levels {
				d.Levels[strings.ToLower(l.UUID)] = sk.ID
				item := strings.ToLower(strings.TrimPrefix(str(l.LevelItem), "EEquippableSkinLevelItem::"))
				det.Levels = append(det.Levels, Level{Name: l.DisplayName, Item: item, Video: str(l.StreamedVideo), Img: str(l.DisplayIcon)})
			}
			for i, ch := range s.Chromas {
				img := str(ch.FullRender)
				if img == "" {
					img = str(ch.DisplayIcon)
				}
				det.Chromas = append(det.Chromas, Chroma{Name: chromaName(ch.DisplayName, i), Img: img, Swatch: str(ch.Swatch), Video: str(ch.StreamedVideo)})
			}
			d.Details[sk.ID] = det
			d.Skins = append(d.Skins, sk)
		}
	}
	sort.SliceStable(d.Skins, func(i, j int) bool { return d.Skins[i].Name < d.Skins[j].Name })
	for _, b := range bundles.Data {
		d.Bundles[strings.ToLower(b.UUID)] = BundleInfo{ID: strings.ToLower(b.UUID), Name: b.DisplayName, Img: str(b.DisplayIcon)}
	}
	if len(d.Skins) == 0 {
		err := errors.New("스킨 정보가 비어 있어요")
		c.setErr(err)
		return err
	}
	b, _ := json.Marshal(d)
	os.MkdirAll(filepath.Dir(c.path()), 0o755)
	writeFileAtomic(c.path(), b, 0o644)
	c.set(d)
	c.setErr(nil)
	log.Printf("content: v%s · 스킨 %d개 · 번들 %d개", d.Version, len(d.Skins), len(d.Bundles))
	if app != nil {
		app.bump()
	}
	return nil
}

func (c *Content) setErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		c.err = ""
	} else {
		c.err = err.Error()
	}
}

// ---------------------------------------------------------------- 이미지 캐시

var imgMu sync.Mutex

func (a *App) handleImg(w http.ResponseWriter, r *http.Request) {
	u := r.URL.Query().Get("u")
	if !strings.HasPrefix(u, mediaPrefix) || strings.Contains(u, "..") || len(u) > 300 {
		http.Error(w, "not allowed", http.StatusBadRequest)
		return
	}
	h := sha1.Sum([]byte(u))
	ext := strings.ToLower(filepath.Ext(u))
	if ext != ".png" && ext != ".jpg" && ext != ".webp" {
		ext = ".png"
	}
	dir := filepath.Join(a.support, "cache", "img")
	p := filepath.Join(dir, hex.EncodeToString(h[:])+ext)
	if !fileExists(p) {
		imgMu.Lock()
		if !fileExists(p) {
			if err := downloadImage(u, p); err != nil {
				imgMu.Unlock()
				http.Error(w, "image unavailable", http.StatusBadGateway)
				return
			}
		}
		imgMu.Unlock()
	}
	w.Header().Set("Cache-Control", "public, max-age=2592000")
	http.ServeFile(w, r, p)
}

func downloadImage(u, dest string) error {
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "ValoROAD/"+build)
	resp, err := (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	os.MkdirAll(filepath.Dir(dest), 0o755)
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, io.LimitReader(resp.Body, 12<<20)); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, dest)
}
