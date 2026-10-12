package main

// 스킨 이름·이미지·영상은 커뮤니티 API(valorant-api.com, 한국어)에서 받아 하루 한 번 새로 고칩니다.

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

var valAPI = func() string {
	if v := os.Getenv("VALOROAD_VALAPI"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://valorant-api.com"
}()

const catalogFormat = 3

type SkinLevel struct {
	UUID  string `json:"uuid"`
	Name  string `json:"name"`
	Icon  string `json:"icon,omitempty"`
	Video string `json:"video,omitempty"`
	Kind  string `json:"kind,omitempty"`
}

type SkinChroma struct {
	UUID   string `json:"uuid"`
	Name   string `json:"name"`
	Icon   string `json:"icon,omitempty"`
	Render string `json:"render,omitempty"`
	Swatch string `json:"swatch,omitempty"`
	Video  string `json:"video,omitempty"`
}

type Skin struct {
	UUID      string       `json:"uuid"`
	Name      string       `json:"name"`
	Weapon    string       `json:"weapon"`
	WeaponID  string       `json:"weaponId"`
	Category  string       `json:"category"`
	Tier      string       `json:"tier,omitempty"`
	Icon      string       `json:"icon,omitempty"`
	Wallpaper string       `json:"wallpaper,omitempty"`
	Default   bool         `json:"default,omitempty"`
	Levels    []SkinLevel  `json:"levels"`
	Chromas   []SkinChroma `json:"chromas"`
}

type Tier struct {
	UUID  string `json:"uuid"`
	Key   string `json:"key"`
	Name  string `json:"name"`
	Rank  int    `json:"rank"`
	Color string `json:"color"`
	Icon  string `json:"icon"`
}

type BundleInfo struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Sub      string `json:"sub,omitempty"`
	Icon     string `json:"icon,omitempty"`
	Icon2    string `json:"icon2,omitempty"`
	Vertical string `json:"vertical,omitempty"`
}

type Weapon struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Icon     string `json:"icon,omitempty"`
}

type CatalogData struct {
	Format     int                   `json:"format"`
	Version    string                `json:"version"`
	FetchedAt  int64                 `json:"fetchedAt"`
	Skins      []*Skin               `json:"skins"`
	Weapons    []Weapon              `json:"weapons"`
	Tiers      map[string]Tier       `json:"tiers"`
	Bundles    map[string]BundleInfo `json:"bundles"`
	Currencies map[string]string     `json:"currencies"` // uuid → icon
}

type Catalog struct {
	mu      sync.RWMutex
	path    string
	data    *CatalogData
	byLevel map[string]*Skin
	bySkin  map[string]*Skin
	byChrom map[string]*Skin
	loading sync.Mutex
	err     string
}

func NewCatalog(support string) *Catalog {
	c := &Catalog{path: filepath.Join(support, "cache", "catalog.json")}
	if b, err := os.ReadFile(c.path); err == nil {
		var d CatalogData
		if json.Unmarshal(b, &d) == nil && d.Format == catalogFormat && len(d.Skins) > 0 {
			c.setData(&d)
		}
	}
	return c
}

func (c *Catalog) setData(d *CatalogData) {
	byLevel, bySkin, byChrom := map[string]*Skin{}, map[string]*Skin{}, map[string]*Skin{}
	for _, s := range d.Skins {
		bySkin[s.UUID] = s
		for _, l := range s.Levels {
			byLevel[l.UUID] = s
		}
		for _, ch := range s.Chromas {
			byChrom[ch.UUID] = s
		}
	}
	c.mu.Lock()
	c.data, c.byLevel, c.bySkin, c.byChrom = d, byLevel, bySkin, byChrom
	c.mu.Unlock()
}

func (c *Catalog) Ready() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data != nil
}

func (c *Catalog) Data() *CatalogData {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data
}

func (c *Catalog) SkinByLevel(id string) *Skin {
	c.mu.RLock()
	defer c.mu.RUnlock()
	id = strings.ToLower(id)
	if s := c.byLevel[id]; s != nil {
		return s
	}
	if s := c.bySkin[id]; s != nil {
		return s
	}
	return c.byChrom[id]
}

func (c *Catalog) Skin(id string) *Skin {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bySkin[strings.ToLower(id)]
}

// Ensure loads the catalog, downloading it when missing or older than a day.
func (c *Catalog) Ensure(force bool) error {
	c.loading.Lock()
	defer c.loading.Unlock()
	d := c.Data()
	if d != nil && !force && time.Since(time.UnixMilli(d.FetchedAt)) < 24*time.Hour {
		return nil
	}
	if d != nil && !force {
		// 버전이 그대로면 날짜만 갱신
		if v, err := fetchAPIVersion(); err == nil && v == d.Version {
			d.FetchedAt = time.Now().UnixMilli()
			c.save(d)
			return nil
		}
	}
	nd, err := downloadCatalog()
	if err != nil {
		log.Printf("catalog: %v", err)
		c.mu.Lock()
		c.err = err.Error()
		c.mu.Unlock()
		if d != nil {
			return nil // 예전 자료로 계속
		}
		return err
	}
	c.save(nd)
	c.setData(nd)
	c.mu.Lock()
	c.err = ""
	c.mu.Unlock()
	return nil
}

func (c *Catalog) save(d *CatalogData) {
	os.MkdirAll(filepath.Dir(c.path), 0o755)
	b, _ := json.Marshal(d)
	writeFileAtomic(c.path, b, 0o644)
}

func getValAPI(path string, out any) error {
	cl := &http.Client{Timeout: 60 * time.Second}
	resp, err := cl.Get(valAPI + path)
	if err != nil {
		return errors.New("스킨 정보를 받지 못했어요. 인터넷 연결을 확인해 주세요")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("스킨 정보를 받지 못했어요 (HTTP %d)", resp.StatusCode)
	}
	var wrap struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrap); err != nil {
		return errors.New("스킨 정보를 읽지 못했어요")
	}
	return json.Unmarshal(wrap.Data, out)
}

func fetchAPIVersion() (string, error) {
	var v struct {
		Version string `json:"version"`
	}
	if err := getValAPI("/v1/version", &v); err != nil {
		return "", err
	}
	return v.Version, nil
}

var tierKo = map[string]string{
	"Select": "셀렉트", "Deluxe": "디럭스", "Premium": "프리미엄", "Exclusive": "익스클루시브", "Ultra": "얼티밋",
}

func categoryKo(c string) string {
	c = strings.TrimPrefix(c, "EEquippableCategory::")
	switch c {
	case "Sidearm":
		return "권총"
	case "SMG":
		return "기관단총"
	case "Shotgun":
		return "산탄총"
	case "Rifle":
		return "소총"
	case "Sniper":
		return "저격총"
	case "Heavy":
		return "기관총"
	case "Melee":
		return "근접 무기"
	}
	return "기타"
}

func levelKind(item string) string {
	switch strings.TrimPrefix(item, "EEquippableSkinLevelItem::") {
	case "VFX":
		return "효과"
	case "Animation":
		return "애니메이션"
	case "Finisher":
		return "마무리 효과"
	case "KillCounter":
		return "처치 카운터"
	case "SoundEffects":
		return "사운드"
	case "InspectAndKill":
		return "점검 · 처치"
	case "TopFrag":
		return "탑 프래그"
	case "AttackerDefenderSwap":
		return "공수 전환"
	case "KillBanner":
		return "처치 배너"
	case "Randomizer":
		return "랜덤"
	case "Transformation":
		return "변신"
	case "Voiceover":
		return "보이스"
	case "KillEffect":
		return "처치 효과"
	case "Fishing":
		return "낚시"
	}
	return ""
}

func sp(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func downloadCatalog() (*CatalogData, error) {
	ver, err := fetchAPIVersion()
	if err != nil {
		return nil, err
	}
	var weapons []struct {
		UUID            string  `json:"uuid"`
		DisplayName     string  `json:"displayName"`
		Category        string  `json:"category"`
		DefaultSkinUUID string  `json:"defaultSkinUuid"`
		DisplayIcon     *string `json:"displayIcon"`
		Skins           []struct {
			UUID            string  `json:"uuid"`
			DisplayName     string  `json:"displayName"`
			ContentTierUUID *string `json:"contentTierUuid"`
			DisplayIcon     *string `json:"displayIcon"`
			Wallpaper       *string `json:"wallpaper"`
			Chromas         []struct {
				UUID          string  `json:"uuid"`
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
	if err := getValAPI("/v1/weapons?language=ko-KR", &weapons); err != nil {
		return nil, err
	}
	var tiers []struct {
		UUID           string `json:"uuid"`
		DevName        string `json:"devName"`
		Rank           int    `json:"rank"`
		HighlightColor string `json:"highlightColor"`
		DisplayIcon    string `json:"displayIcon"`
	}
	if err := getValAPI("/v1/contenttiers", &tiers); err != nil {
		return nil, err
	}
	var bundles []struct {
		UUID               string  `json:"uuid"`
		DisplayName        string  `json:"displayName"`
		DisplayNameSubText *string `json:"displayNameSubText"`
		DisplayIcon        *string `json:"displayIcon"`
		DisplayIcon2       *string `json:"displayIcon2"`
		VerticalPromoImage *string `json:"verticalPromoImage"`
	}
	if err := getValAPI("/v1/bundles?language=ko-KR", &bundles); err != nil {
		return nil, err
	}
	var currencies []struct {
		UUID        string `json:"uuid"`
		DisplayIcon string `json:"displayIcon"`
	}
	getValAPI("/v1/currencies", &currencies)

	d := &CatalogData{Format: catalogFormat, Version: ver, FetchedAt: time.Now().UnixMilli(),
		Tiers: map[string]Tier{}, Bundles: map[string]BundleInfo{}, Currencies: map[string]string{}}
	for _, t := range tiers {
		col := t.HighlightColor
		if len(col) == 8 {
			col = col[:6]
		}
		name := tierKo[t.DevName]
		if name == "" {
			name = t.DevName
		}
		d.Tiers[strings.ToLower(t.UUID)] = Tier{UUID: strings.ToLower(t.UUID), Key: strings.ToLower(t.DevName), Name: name, Rank: t.Rank, Color: "#" + col, Icon: t.DisplayIcon}
	}
	for _, b := range bundles {
		d.Bundles[strings.ToLower(b.UUID)] = BundleInfo{UUID: strings.ToLower(b.UUID), Name: b.DisplayName, Sub: sp(b.DisplayNameSubText),
			Icon: sp(b.DisplayIcon), Icon2: sp(b.DisplayIcon2), Vertical: sp(b.VerticalPromoImage)}
	}
	for _, cur := range currencies {
		d.Currencies[strings.ToLower(cur.UUID)] = cur.DisplayIcon
	}
	for _, w := range weapons {
		cat := categoryKo(w.Category)
		d.Weapons = append(d.Weapons, Weapon{UUID: strings.ToLower(w.UUID), Name: w.DisplayName, Category: cat, Icon: sp(w.DisplayIcon)})
		for _, sk := range w.Skins {
			skin := &Skin{UUID: strings.ToLower(sk.UUID), Name: strings.TrimSpace(sk.DisplayName), Weapon: w.DisplayName,
				WeaponID: strings.ToLower(w.UUID), Category: cat, Tier: strings.ToLower(sp(sk.ContentTierUUID)), Wallpaper: sp(sk.Wallpaper),
				Levels: []SkinLevel{}, Chromas: []SkinChroma{}}
			skin.Default = strings.EqualFold(sk.UUID, w.DefaultSkinUUID) || strings.Contains(sk.DisplayName, "랜덤 즐겨찾기") ||
				strings.Contains(strings.ToLower(sk.DisplayName), "random favorite")
			for i, l := range sk.Levels {
				name := fmt.Sprintf("레벨 %d", i+1)
				skin.Levels = append(skin.Levels, SkinLevel{UUID: strings.ToLower(l.UUID), Name: name, Icon: sp(l.DisplayIcon), Video: sp(l.StreamedVideo), Kind: levelKind(sp(l.LevelItem))})
			}
			for i, ch := range sk.Chromas {
				name := strings.TrimSpace(strings.ReplaceAll(ch.DisplayName, "\n", " "))
				if i == 0 {
					name = "기본"
				} else if j := strings.Index(name, "("); j >= 0 {
					name = strings.Trim(name[j:], "() ")
				}
				skin.Chromas = append(skin.Chromas, SkinChroma{UUID: strings.ToLower(ch.UUID), Name: name, Icon: sp(ch.DisplayIcon),
					Render: sp(ch.FullRender), Swatch: sp(ch.Swatch), Video: sp(ch.StreamedVideo)})
			}
			switch {
			case len(sk.Levels) > 0 && sk.Levels[0].DisplayIcon != nil:
				skin.Icon = *sk.Levels[0].DisplayIcon
			case sk.DisplayIcon != nil:
				skin.Icon = *sk.DisplayIcon
			case len(sk.Chromas) > 0 && sk.Chromas[0].FullRender != nil:
				skin.Icon = *sk.Chromas[0].FullRender
			}
			d.Skins = append(d.Skins, skin)
		}
	}
	sort.SliceStable(d.Skins, func(i, j int) bool { return d.Skins[i].Name < d.Skins[j].Name })
	if len(d.Skins) == 0 {
		return nil, errors.New("스킨 정보가 비어 있어요")
	}
	return d, nil
}
