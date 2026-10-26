import Foundation

// 라이엇 연결 (공식 API 가 아니라서 바뀔 수 있어요 — PROJECT_NOTES.md 「라이엇 연결」)
// 맥 엔진(engine/riot.go)과 같은 방식을 아이폰의 URLSession 으로 그대로 옮겼어요.
//
// - 로그인: 앱 안의 라이엇 공식 로그인 화면(LoginView) → playvalorant.com/opt_in#access_token=… 를 가로채요. 비밀번호는 앱이 못 봐요.
// - 자동 로그인: 저장한 쿠키로 같은 authorize 주소를 부르면(리디렉트 따라가지 않음) Location 의 # 뒤에 새 토큰이 와요.
// - 상점: pd.{shard}.a.pvp.net/store/v3/storefront/{puuid} (POST {}), 지갑: store/v1/wallet/{puuid}

let riotAuthorizeURL = "https://auth.riotgames.com/authorize?redirect_uri=https%3A%2F%2Fplayvalorant.com%2Fopt_in&client_id=play-valorant-web-prod&response_type=token%20id_token&nonce=1&scope=account%20openid"

/// 로그인 화면과 자동 로그인에 같은 브라우저 이름을 써요 (아이폰 사파리)
let iPhoneSafariUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.6 Mobile/15E148 Safari/604.1"

/// 라이엇 클라이언트가 보내는 플랫폼 정보(base64). 널리 쓰이는 값 그대로예요.
let riotClientPlatform = Data("{\r\n\t\"platformType\": \"PC\",\r\n\t\"platformOS\": \"Windows\",\r\n\t\"platformOSVersion\": \"10.0.19042.1.256.64bit\",\r\n\t\"platformChipset\": \"Unknown\"\r\n}".utf8).base64EncodedString()

enum Currency {
    static let vp = "85ad13f7-3d1b-5128-9eb2-7cd8ee0b5741"
    static let rp = "e59aa87c-4cbf-517a-5983-6e81511be9b7"
    static let kc = "85ca954a-41f2-ce94-9b45-8ca3dd39a00d"
}

enum RiotError: LocalizedError, Equatable {
    case expired
    case busy(Int)
    case rejected(Int)
    case network
    case message(String)

    var errorDescription: String? {
        switch self {
        case .expired: return "로그인이 풀렸어요. 이 계정만 다시 로그인해 주세요"
        case .busy(let c): return "라이엇 서버가 요청을 받아 주지 않았어요 (HTTP \(c)). 잠시 뒤 다시 시도해 주세요"
        case .rejected(let c): return "라이엇 서버가 상점 요청을 받아 주지 않았어요 (HTTP \(c)). 잠시 뒤 다시 시도해 주세요"
        case .network: return "라이엇 서버에 연결할 수 없어요. 인터넷 연결을 확인해 주세요"
        case .message(let m): return m
        }
    }
}

// MARK: - 쿠키

struct RiotCookie: Codable, Equatable {
    var name: String
    var value: String
    var domain: String
    var path: String
    var expires: Int64 = 0 // 유닉스 초, 0 = 세션 쿠키
    var secure: Bool = false
    var httpOnly: Bool = false

    init(name: String, value: String, domain: String, path: String, expires: Int64 = 0, secure: Bool = false, httpOnly: Bool = false) {
        self.name = name; self.value = value; self.domain = domain; self.path = path
        self.expires = expires; self.secure = secure; self.httpOnly = httpOnly
    }

    init(_ c: HTTPCookie) {
        self.init(name: c.name, value: c.value, domain: c.domain, path: c.path,
                  expires: c.expiresDate.map { Int64($0.timeIntervalSince1970) } ?? 0,
                  secure: c.isSecure, httpOnly: c.isHTTPOnly)
    }

    var isRiot: Bool {
        let d = domain.lowercased().trimmingCharacters(in: CharacterSet(charactersIn: "."))
        return d == "riotgames.com" || d.hasSuffix(".riotgames.com")
    }
}

enum Cookies {
    static func domainMatch(_ domain: String, _ host: String) -> Bool {
        let d = domain.lowercased().trimmingCharacters(in: CharacterSet(charactersIn: "."))
        let h = host.lowercased()
        return h == d || h.hasSuffix("." + d)
    }

    static func header(_ cookies: [RiotCookie], host: String) -> String {
        let now = Int64(Date().timeIntervalSince1970)
        return cookies
            .filter { !$0.value.isEmpty && !($0.expires > 0 && $0.expires < now) && domainMatch($0.domain, host) }
            .map { "\($0.name)=\($0.value)" }
            .joined(separator: "; ")
    }

    /// 새로 받은 쿠키로 바꾸거나 더하고, 만료된 것은 빼요
    static func merge(_ old: [RiotCookie], _ fresh: [RiotCookie]) -> [RiotCookie] {
        func key(_ c: RiotCookie) -> String {
            let p = c.path.isEmpty ? "/" : c.path
            return c.name + "|" + c.domain.lowercased().trimmingCharacters(in: CharacterSet(charactersIn: ".")) + "|" + p
        }
        let now = Int64(Date().timeIntervalSince1970)
        var out: [RiotCookie] = []
        var idx: [String: Int] = [:]
        for c in old where !(c.expires > 0 && c.expires < now) {
            idx[key(c)] = out.count
            out.append(c)
        }
        for c in fresh {
            let k = key(c)
            let dead = c.value.isEmpty || (c.expires > 0 && c.expires < now)
            if let i = idx[k] {
                if dead { out[i].value = "" } else { out[i] = c }
                continue
            }
            if !dead {
                idx[k] = out.count
                out.append(c)
            }
        }
        return out.filter { !$0.value.isEmpty }
    }
}

// MARK: - 토큰 · 세션

struct Tokens {
    var access: String
    var id: String
    var expires: Date
}

struct Session {
    var access: String
    var id: String
    var ent: String
    var expires: Date
    var valid: Bool { !access.isEmpty && !ent.isEmpty && expires.timeIntervalSinceNow > 5 * 60 }
}

/// https://playvalorant.com/opt_in#access_token=…&id_token=…&expires_in=3600
func parseTokens(_ s: String) -> Tokens? {
    guard let u = URL(string: s) else { return nil }
    let frag = u.fragment ?? u.query ?? ""
    var q: [String: String] = [:]
    for part in frag.split(separator: "&") {
        let kv = part.split(separator: "=", maxSplits: 1).map(String.init)
        if kv.count == 2 { q[kv[0]] = kv[1].removingPercentEncoding ?? kv[1] }
    }
    guard let access = q["access_token"], !access.isEmpty else { return nil }
    var exp = Int(q["expires_in"] ?? "") ?? 3600
    if exp <= 0 { exp = 3600 }
    return Tokens(access: access, id: q["id_token"] ?? "", expires: Date().addingTimeInterval(TimeInterval(exp)))
}

// MARK: - 연결

struct RiotResponse {
    var status: Int
    var location: String
    var cookies: [RiotCookie]
    var body: Data
}

private final class NoRedirect: NSObject, URLSessionTaskDelegate {
    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) {
        completionHandler(nil)
    }
}

enum Riot {
    private static let session: URLSession = {
        let cfg = URLSessionConfiguration.ephemeral
        cfg.httpCookieAcceptPolicy = .never
        cfg.httpShouldSetCookies = false
        cfg.httpCookieStorage = nil
        cfg.urlCache = nil
        cfg.timeoutIntervalForRequest = 25
        cfg.waitsForConnectivity = false
        return URLSession(configuration: cfg)
    }()

    static func send(_ method: String, _ urlString: String, headers: [String: String] = [:], body: Data? = nil,
                     noRedirect: Bool = false) async throws -> RiotResponse {
        guard let url = URL(string: urlString) else { throw RiotError.message("주소가 잘못됐어요") }
        var req = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: 25)
        req.httpMethod = method
        req.httpShouldHandleCookies = false
        for (k, v) in headers { req.setValue(v, forHTTPHeaderField: k) }
        req.httpBody = body
        let data: Data
        let resp: URLResponse
        do {
            (data, resp) = try await session.data(for: req, delegate: noRedirect ? NoRedirect() : nil)
        } catch {
            Log.write("⚠️ 연결 실패 \(url.host ?? "") : \(error.localizedDescription)")
            throw RiotError.network
        }
        guard let h = resp as? HTTPURLResponse else { throw RiotError.network }
        var fields: [String: String] = [:]
        for (k, v) in h.allHeaderFields {
            if let ks = k as? String, let vs = v as? String { fields[ks] = vs }
        }
        let cookies = HTTPCookie.cookies(withResponseHeaderFields: fields, for: url).map(RiotCookie.init)
        return RiotResponse(status: h.statusCode, location: h.value(forHTTPHeaderField: "Location") ?? "", cookies: cookies, body: data)
    }

    private static func json(_ d: Data) -> [String: Any]? {
        (try? JSONSerialization.jsonObject(with: d)) as? [String: Any]
    }

    /// 저장한 쿠키로 새 토큰을 받아요. 쿠키가 만료됐으면 .expired
    static func reauth(_ cookies: [RiotCookie]) async throws -> (Tokens, [RiotCookie]) {
        let r = try await send("GET", riotAuthorizeURL, headers: [
            "Cookie": Cookies.header(cookies, host: "auth.riotgames.com"),
            "User-Agent": iPhoneSafariUA,
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
            "Accept-Language": "ko-KR,ko;q=0.9"
        ], noRedirect: true)
        let merged = Cookies.merge(cookies, r.cookies)
        if let t = parseTokens(r.location) { return (t, merged) }
        if r.status == 403 || r.status == 429 { throw ReauthFailure(error: .busy(r.status), cookies: merged) }
        throw ReauthFailure(error: .expired, cookies: merged)
    }

    struct ReauthFailure: Error {
        let error: RiotError
        let cookies: [RiotCookie]
    }

    struct UserInfo {
        var puuid: String
        var name: String
        var tag: String
    }

    static func userInfo(_ access: String) async throws -> UserInfo {
        let r = try await send("GET", "https://auth.riotgames.com/userinfo",
                               headers: ["Authorization": "Bearer " + access, "User-Agent": iPhoneSafariUA])
        if r.status == 401 || r.status == 403 { throw RiotError.expired }
        guard r.status == 200, let v = json(r.body), let sub = v["sub"] as? String, !sub.isEmpty else {
            throw RiotError.message("계정 정보를 받지 못했어요 (HTTP \(r.status))")
        }
        let acct = v["acct"] as? [String: Any]
        return UserInfo(puuid: sub, name: acct?["game_name"] as? String ?? "", tag: acct?["tag_line"] as? String ?? "")
    }

    static func entitlements(_ access: String) async throws -> String {
        let r = try await send("POST", "https://entitlements.auth.riotgames.com/api/token/v1",
                               headers: ["Authorization": "Bearer " + access, "Content-Type": "application/json", "User-Agent": iPhoneSafariUA],
                               body: Data("{}".utf8))
        if r.status == 401 || r.status == 403 { throw RiotError.expired }
        guard r.status == 200, let tok = json(r.body)?["entitlements_token"] as? String, !tok.isEmpty else {
            throw RiotError.message("게임 이용 정보를 받지 못했어요 (HTTP \(r.status))")
        }
        return tok
    }

    /// 계정의 지역 (kr · ap · eu · na · latam · br)
    static func region(_ access: String, _ idToken: String) async throws -> String {
        let body = try JSONSerialization.data(withJSONObject: ["id_token": idToken])
        let r = try await send("PUT", "https://riot-geo.pas.si.riotgames.com/pas/v1/product/valorant",
                               headers: ["Authorization": "Bearer " + access, "Content-Type": "application/json", "User-Agent": iPhoneSafariUA],
                               body: body)
        guard r.status == 200, let aff = json(r.body)?["affinities"] as? [String: Any],
              let live = aff["live"] as? String, !live.isEmpty else {
            throw RiotError.message("계정의 지역을 알지 못했어요 (HTTP \(r.status))")
        }
        return live.lowercased()
    }

    static func shard(for region: String) -> String {
        switch region.lowercased() {
        case "latam", "br", "na": return "na"
        case "pbe": return "pbe"
        case "eu": return "eu"
        case "kr": return "kr"
        default: return "ap"
        }
    }

    private static func pd(_ s: Session, shard: String, clientVersion: String, method: String, path: String, body: Data?) async throws -> Data {
        let r = try await send(method, "https://pd.\(shard).a.pvp.net" + path, headers: [
            "Authorization": "Bearer " + s.access,
            "X-Riot-Entitlements-JWT": s.ent,
            "X-Riot-ClientPlatform": riotClientPlatform,
            "X-Riot-ClientVersion": clientVersion,
            "Content-Type": "application/json"
        ], body: body)
        switch r.status {
        case 200: return r.body
        case 400, 401, 403: throw RiotError.rejected(r.status)
        case 429: throw RiotError.message("라이엇 서버가 잠시 쉬어 달래요. 1분 뒤 다시 시도해 주세요")
        default: throw RiotError.message("상점 정보를 받지 못했어요 (HTTP \(r.status))")
        }
    }

    static func storefront(_ s: Session, shard: String, puuid: String, clientVersion: String) async throws -> [String: Any] {
        let d = try await pd(s, shard: shard, clientVersion: clientVersion, method: "POST", path: "/store/v3/storefront/" + puuid, body: Data("{}".utf8))
        guard let v = json(d) else { throw RiotError.message("상점 정보를 읽지 못했어요") }
        return v
    }

    static func wallet(_ s: Session, shard: String, puuid: String, clientVersion: String) async throws -> Wallet {
        let d = try await pd(s, shard: shard, clientVersion: clientVersion, method: "GET", path: "/store/v1/wallet/" + puuid, body: nil)
        guard let b = json(d)?["Balances"] as? [String: Any] else { throw RiotError.message("지갑 정보를 읽지 못했어요") }
        func n(_ k: String) -> Int { (b[k] as? NSNumber)?.intValue ?? 0 }
        return Wallet(vp: n(Currency.vp), rp: n(Currency.rp), kc: n(Currency.kc))
    }
}

// MARK: - 상점 읽기 (engine/accounts.go buildStore 와 같음)

/// 상점은 매일 00:00 UTC (한국 오전 9시)에 바뀌어요
func nextDailyReset(_ now: Date = Date()) -> Date {
    var cal = Calendar(identifier: .gregorian)
    cal.timeZone = TimeZone(identifier: "UTC")!
    let start = cal.startOfDay(for: now)
    let r = start > now ? start : cal.date(byAdding: .day, value: 1, to: start)!
    return r
}

func buildStore(_ sf: [String: Any], now: Date = Date()) -> StoreData {
    let t = Int64(now.timeIntervalSince1970)
    func dict(_ v: Any?) -> [String: Any] { v as? [String: Any] ?? [:] }
    func arr(_ v: Any?) -> [[String: Any]] { v as? [[String: Any]] ?? [] }
    func int(_ v: Any?) -> Int64 { (v as? NSNumber)?.int64Value ?? 0 }
    func vp(_ v: Any?) -> Int { (dict(v)[Currency.vp] as? NSNumber)?.intValue ?? 0 }

    var st = StoreData(fetched: t)
    let panel = dict(sf["SkinsPanelLayout"])
    let offers = arr(panel["SingleItemStoreOffers"])
    if !offers.isEmpty {
        for o in offers {
            var item = (arr(o["Rewards"]).first?["ItemID"] as? String) ?? ""
            if item.isEmpty { item = o["OfferID"] as? String ?? "" }
            st.daily.append(RawOffer(item: item.lowercased(), price: vp(o["Cost"])))
        }
    } else {
        for id in panel["SingleItemOffers"] as? [String] ?? [] {
            st.daily.append(RawOffer(item: id.lowercased(), price: 0))
        }
    }
    let rem = int(panel["SingleItemOffersRemainingDurationInSeconds"])
    st.dailyEnds = rem > 0 ? t + rem : Int64(nextDailyReset(now).timeIntervalSince1970)

    let bonus = dict(sf["BonusStore"])
    let nrem = int(bonus["BonusStoreRemainingDurationInSeconds"])
    if nrem > 0 {
        st.nightEnds = t + nrem
        for o in arr(bonus["BonusStoreOffers"]) {
            let offer = dict(o["Offer"])
            let item = (arr(offer["Rewards"]).first?["ItemID"] as? String) ?? ""
            st.night.append(RawOffer(item: item.lowercased(), price: vp(o["DiscountCosts"]), base: vp(offer["Cost"]),
                                     pct: (o["DiscountPercent"] as? NSNumber)?.intValue ?? 0))
        }
    }

    let fb = dict(sf["FeaturedBundle"])
    for b in arr(fb["Bundles"]) {
        let items = arr(b["Items"])
        var rb = RawBundle(id: (b["DataAssetID"] as? String ?? "").lowercased(), price: vp(b["TotalDiscountedCost"]),
                           base: vp(b["TotalBaseCost"]), ends: 0, items: items.count)
        var p = 0, bs = 0
        for it in items {
            let base = (it["BasePrice"] as? NSNumber)?.intValue ?? 0
            let disc = (it["DiscountedPrice"] as? NSNumber)?.intValue ?? 0
            p += disc; bs += base
            if base > 0, let id = dict(it["Item"])["ItemID"] as? String, !id.isEmpty {
                st.seen[id.lowercased()] = base
            }
        }
        if rb.price == 0 { rb.price = p }
        if rb.base == 0 { rb.base = bs }
        var r = int(b["DurationRemainingInSeconds"])
        if r == 0 { r = int(fb["BundleRemainingDurationInSeconds"]) }
        if r > 0 { rb.ends = t + r }
        st.bundles.append(rb)
    }
    return st
}
