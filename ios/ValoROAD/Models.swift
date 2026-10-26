import Foundation

// 저장하는 자료 모양 (맥 엔진의 accounts.go · content.go 와 같은 뜻)

struct Account: Codable, Identifiable, Equatable {
    var id: String
    var slot: Int
    var puuid: String
    var name: String
    var tag: String
    var region: String
    var shard: String
    var added: Int64
    var lastOK: Int64 = 0
    var status: String = "ok" // ok | expired | error
    var error: String = ""

    var label: String { tag.isEmpty ? name : "\(name)#\(tag)" }
    var slotText: String { String(format: "%02d", slot) }
}

struct Wallet: Codable, Equatable {
    var vp: Int
    var rp: Int
    var kc: Int
}

struct RawOffer: Codable, Equatable {
    var item: String // 스킨 레벨 uuid
    var price: Int
    var base: Int = 0
    var pct: Int = 0
}

struct RawBundle: Codable, Equatable {
    var id: String // 번들 정보 uuid (DataAssetID)
    var price: Int
    var base: Int
    var ends: Int64
    var items: Int
}

struct StoreData: Codable, Equatable {
    var fetched: Int64
    var daily: [RawOffer] = []
    var dailyEnds: Int64 = 0
    var night: [RawOffer] = []
    var nightEnds: Int64 = 0
    var bundles: [RawBundle] = []
    var wallet: Wallet?
    var seen: [String: Int] = [:]

    /// 상점 · 밤시장 · 번들에서 본 원래 가격들
    var seenPrices: [String: Int] {
        var m = seen
        for o in daily where o.price > 0 { m[o.item] = o.price }
        for o in night where o.base > 0 { m[o.item] = o.base }
        return m
    }
}

struct Settings: Codable, Equatable {
    var theme: String = "auto" // auto | dark | light
    var wishAlert = true
    var nightAlert = true
    var passAlert = true
    var wish: [String] = []    // 스킨 uuid (새로 담은 것이 앞)
    var active: String = ""    // 마지막으로 본 계정
    var askedNotify = false

    init() {}

    // 나중에 항목이 늘어도 예전 설정 파일을 그대로 읽을 수 있게, 없는 항목은 기본값으로
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        theme = try c.decodeIfPresent(String.self, forKey: .theme) ?? "auto"
        wishAlert = try c.decodeIfPresent(Bool.self, forKey: .wishAlert) ?? true
        nightAlert = try c.decodeIfPresent(Bool.self, forKey: .nightAlert) ?? true
        passAlert = try c.decodeIfPresent(Bool.self, forKey: .passAlert) ?? true
        wish = try c.decodeIfPresent([String].self, forKey: .wish) ?? []
        active = try c.decodeIfPresent(String.self, forKey: .active) ?? ""
        askedNotify = try c.decodeIfPresent(Bool.self, forKey: .askedNotify) ?? false
    }
}

// MARK: - 스킨 정보 (valorant-api.com)

struct Skin: Codable, Identifiable, Hashable {
    var id: String       // 스킨 uuid (위시리스트에 저장)
    var level: String    // 1레벨 uuid (상점의 ItemID)
    var name: String
    var weapon: String
    var weaponEn: String
    var cat: String      // rifle sidearm sniper smg shotgun heavy melee
    var tier: String     // select deluxe premium exclusive ultra ("" = 기본 스킨)
    var img: String

    static func placeholder(_ id: String) -> Skin {
        Skin(id: id, level: id, name: "스킨 정보를 불러오는 중", weapon: "", weaponEn: "", cat: "other", tier: "", img: "")
    }
}

struct Chroma: Codable, Hashable {
    var name: String
    var img: String
    var swatch: String = ""
    var video: String = ""
}

struct Level: Codable, Hashable {
    var name: String
    var item: String // vfx animation finisher … ("" = 기본)
    var video: String = ""
    var img: String = ""
}

struct SkinDetail: Codable, Hashable {
    var chromas: [Chroma]
    var levels: [Level]
}

struct BundleInfo: Codable, Hashable {
    var id: String
    var name: String
    var img: String
}

struct ContentData: Codable {
    static let currentFormat = 2
    var format: Int
    var version: String
    var clientVersion: String
    var fetched: Int64
    var skins: [Skin]
    var levels: [String: String]        // 모든 레벨 uuid → 스킨 uuid
    var details: [String: SkinDetail]
    var bundles: [String: BundleInfo]
}

// MARK: - 화면에 쓰는 모양

struct Offer: Identifiable, Hashable {
    var id: String { item }
    var item: String
    var skin: Skin
    var known: Bool
    var price: Int
    var base: Int
    var pct: Int
    var wished: Bool
}

struct Hit: Hashable {
    var accountID: String
    var label: String
    var slot: Int
    var skin: Skin
    var night: Bool
    var price: Int
    var base: Int
}

enum PriceState {
    case ok, estimate, none, unknown
}
