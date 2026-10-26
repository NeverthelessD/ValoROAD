import Foundation

// 스킨 정보 (이름 · 무기 · 등급 · 이미지 · 미리보기) — valorant-api.com (커뮤니티가 운영하는 공개 자료)
// 게임 버전이 바뀔 때만 다시 받아서 content.json 에 줄여서 저장해요. (engine/content.go 와 같은 방식)

let fallbackClientVersion = "release-13.08-shipping-20-5715960"

enum ContentLoader {
    private static let base = "https://valorant-api.com/v1/"

    private static func get(_ path: String) async throws -> [String: Any] {
        guard let url = URL(string: base + path) else { throw RiotError.message("주소가 잘못됐어요") }
        var req = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: 60)
        req.setValue("ValoROAD-iPhone/\(appVersion)", forHTTPHeaderField: "User-Agent")
        let data: Data, resp: URLResponse
        do {
            (data, resp) = try await URLSession.shared.data(for: req)
        } catch {
            throw RiotError.message("스킨 정보를 받을 수 없어요. 인터넷 연결을 확인해 주세요")
        }
        let code = (resp as? HTTPURLResponse)?.statusCode ?? 0
        guard code == 200, let v = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any] else {
            throw RiotError.message("스킨 정보를 받지 못했어요 (HTTP \(code))")
        }
        return v
    }

    static func version() async throws -> (String, String) {
        let d = try await get("version")["data"] as? [String: Any] ?? [:]
        return (d["version"] as? String ?? "", d["riotClientVersion"] as? String ?? "")
    }

    private static func s(_ v: Any?) -> String { v as? String ?? "" }

    private static func cat(_ c: String) -> String {
        let x = c.replacingOccurrences(of: "EEquippableCategory::", with: "").lowercased()
        return ["rifle", "sidearm", "sniper", "smg", "shotgun", "heavy", "melee"].contains(x) ? x : "other"
    }

    /// 스킨 이름 끝의 "(변형 1 주황색)" → "변형 1 주황색", 첫 번째는 "기본"
    private static func chromaName(_ s: String, _ i: Int) -> String {
        if let a = s.range(of: "(", options: .backwards), let b = s.range(of: ")", options: .backwards), a.upperBound <= b.lowerBound {
            return String(s[a.upperBound..<b.lowerBound]).trimmingCharacters(in: .whitespacesAndNewlines)
        }
        return i == 0 ? "기본" : "변형 \(i)"
    }

    static func fetch(version: String, clientVersion: String) async throws -> ContentData {
        async let koR = get("weapons?language=ko-KR")
        async let enR = get("weapons?language=en-US")
        async let tiersR = get("contenttiers")
        async let bundlesR = get("bundles?language=ko-KR")
        let (ko, en, tiers, bundles) = try await (koR, enR, tiersR, bundlesR)

        var tierName: [String: String] = [:]
        for t in tiers["data"] as? [[String: Any]] ?? [] {
            tierName[s(t["uuid"]).lowercased()] = s(t["devName"]).lowercased()
        }
        var weaponEn: [String: String] = [:]
        for w in en["data"] as? [[String: Any]] ?? [] { weaponEn[s(w["uuid"])] = s(w["displayName"]) }

        var d = ContentData(format: ContentData.currentFormat, version: version, clientVersion: clientVersion,
                            fetched: Int64(Date().timeIntervalSince1970), skins: [], levels: [:], details: [:], bundles: [:])
        for w in ko["data"] as? [[String: Any]] ?? [] {
            let wname = s(w["displayName"]), wen = weaponEn[s(w["uuid"])] ?? "", wcat = cat(s(w["category"]))
            for sk in w["skins"] as? [[String: Any]] ?? [] {
                let id = s(sk["uuid"]).lowercased()
                let levels = sk["levels"] as? [[String: Any]] ?? []
                let chromas = sk["chromas"] as? [[String: Any]] ?? []
                var skin = Skin(id: id, level: "", name: s(sk["displayName"]), weapon: wname, weaponEn: wen.uppercased(), cat: wcat,
                                tier: tierName[s(sk["contentTierUuid"]).lowercased()] ?? "", img: "")
                if let l0 = levels.first {
                    skin.level = s(l0["uuid"]).lowercased()
                    skin.img = s(l0["displayIcon"])
                }
                if skin.img.isEmpty { skin.img = s(sk["displayIcon"]) }
                if skin.img.isEmpty, let c0 = chromas.first {
                    skin.img = s(c0["fullRender"]).isEmpty ? s(c0["displayIcon"]) : s(c0["fullRender"])
                }
                var det = SkinDetail(chromas: [], levels: [])
                for l in levels {
                    d.levels[s(l["uuid"]).lowercased()] = id
                    let item = s(l["levelItem"]).replacingOccurrences(of: "EEquippableSkinLevelItem::", with: "").lowercased()
                    det.levels.append(Level(name: s(l["displayName"]), item: item, video: s(l["streamedVideo"]), img: s(l["displayIcon"])))
                }
                for (i, c) in chromas.enumerated() {
                    let img = s(c["fullRender"]).isEmpty ? s(c["displayIcon"]) : s(c["fullRender"])
                    det.chromas.append(Chroma(name: chromaName(s(c["displayName"]), i), img: img, swatch: s(c["swatch"]), video: s(c["streamedVideo"])))
                }
                d.details[id] = det
                d.skins.append(skin)
            }
        }
        d.skins.sort { $0.name < $1.name }
        for b in bundles["data"] as? [[String: Any]] ?? [] {
            let id = s(b["uuid"]).lowercased()
            d.bundles[id] = BundleInfo(id: id, name: s(b["displayName"]), img: s(b["displayIcon"]))
        }
        if d.skins.isEmpty { throw RiotError.message("스킨 정보가 비어 있어요") }
        return d
    }

    static func loadSaved() -> ContentData? {
        guard let data = try? Data(contentsOf: Paths.file("content.json")),
              let d = try? JSONDecoder().decode(ContentData.self, from: data), !d.skins.isEmpty else { return nil }
        return d
    }

    static func save(_ d: ContentData) {
        Disk.save(d, "content.json")
    }
}

/// 빠르게 찾기 위한 색인 (메모리)
struct ContentIndex {
    var data: ContentData?
    private var byID: [String: Int] = [:]

    init(_ d: ContentData?) {
        data = d
        if let d { for (i, s) in d.skins.enumerated() { byID[s.id] = i } }
    }

    var ready: Bool { data != nil }
    var clientVersion: String {
        if let v = data?.clientVersion, !v.isEmpty { return v }
        return fallbackClientVersion
    }

    /// 상점의 ItemID(레벨 uuid) 또는 스킨 uuid → 스킨
    func skin(_ id: String) -> Skin? {
        guard let d = data else { return nil }
        let k = id.lowercased()
        if let i = byID[k] { return d.skins[i] }
        if let sid = d.levels[k], let i = byID[sid] { return d.skins[i] }
        return nil
    }

    func detail(_ id: String) -> SkinDetail? { data?.details[id] }

    func bundle(_ id: String) -> BundleInfo? { data?.bundles[id.lowercased()] }

    static func norm(_ s: String) -> String {
        s.lowercased().components(separatedBy: .whitespacesAndNewlines).joined()
    }

    /// 위시리스트에 담을 수 있는 스킨 (등급이 있는 것)
    func search(_ q: String, cat: String, limit: Int = 80) -> [Skin] {
        guard let d = data else { return [] }
        let nq = Self.norm(q)
        var out: [Skin] = []
        for s in d.skins where !s.tier.isEmpty && (cat == "all" || s.cat == cat) {
            if !nq.isEmpty && !Self.norm(s.name).contains(nq) && !Self.norm(s.weaponEn).contains(nq) && !Self.norm(s.weapon).contains(nq) {
                continue
            }
            out.append(s)
            if out.count >= limit { break }
        }
        return out
    }

    /// 시험 모드용: 정해진 씨앗으로 고른 스킨들
    func random(_ n: Int, seed: Int64, tierOK: (String) -> Bool = { _ in true }) -> [Skin] {
        guard let d = data else { return [] }
        let pool = d.skins.filter { !$0.tier.isEmpty && !$0.img.isEmpty && tierOK($0.tier) }
        guard !pool.isEmpty else { return [] }
        var x = UInt64(bitPattern: seed) &* 6364136223846793005 &+ 1442695040888963407
        var used = Set<Int>()
        var out: [Skin] = []
        while out.count < n && used.count < pool.count {
            x = x &* 6364136223846793005 &+ 1442695040888963407
            let i = Int((x >> 33) % UInt64(pool.count))
            if used.contains(i) { continue }
            used.insert(i)
            out.append(pool[i])
        }
        return out
    }
}

/// 등급별 보통 가격 (상점에서 본 적 없는 스킨은 '약'으로 보여 줘요)
func tierPrice(_ s: Skin) -> Int {
    if s.cat == "melee" {
        return ["select": 1750, "deluxe": 2550, "premium": 3550, "exclusive": 4350, "ultra": 4950][s.tier] ?? 0
    }
    return ["select": 875, "deluxe": 1275, "premium": 1775, "exclusive": 2175, "ultra": 2475][s.tier] ?? 0
}

