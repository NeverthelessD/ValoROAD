import BackgroundTasks
import Foundation
import SwiftUI
import UIKit
import UserNotifications

// 앱의 두뇌: 계정 3개 · 자동 로그인 · 계정별 상점 · 위시리스트 · 알림 (맥 엔진 accounts.go · wishlist.go 와 같은 규칙)
//
// - 계정 정보는 accounts.json, 로그인 열쇠(쿠키)는 키체인, 상점은 stores/<id>.json
// - 토큰은 메모리에만 두고, 1시간마다 쿠키로 새로 받아요. 쿠키가 만료되면 그 계정만 '다시 로그인'.
// - 상점은 앱을 열 때 · 계정을 고를 때 · 매일 상점이 바뀔 때(한국 오전 9시) · 새로고침(당겨서/버튼)으로 다시 받아요.
// - 앱이 닫혀 있을 때는 아이폰이 정해 주는 때에 뒤에서 한 번씩 확인해요 (BGAppRefresh).

struct Notice: Identifiable, Equatable {
    let id = UUID()
    var text: String
    var bad = false
}

@MainActor
final class AppModel: ObservableObject {
    static let shared = AppModel()
    static let backgroundID = "com.neverthelessd.valoroad.iphone.refresh"
    static let maxAccounts = 3

    @Published var accounts: [Account] = []
    @Published var stores: [String: StoreData] = [:]
    @Published var settings = Settings()
    @Published var content = ContentIndex(nil)
    @Published var contentError = ""
    @Published var contentLoading = false
    @Published var busy: Set<String> = []
    @Published var notice: Notice?
    @Published var pass = AppPass.read()

    private var sessions: [String: Session] = [:]
    private var seen: [String: Int] = [:]
    private var notified: [String: Int64] = [:]
    private var lastTry: [String: Date] = [:]
    private var lastContentCheck = Date.distantPast
    private var started = false

    init() {
        if Demo.on {
            settings = Settings()
            settings.theme = Demo.arg("-theme") ?? "dark"
            return
        }
        settings = Disk.load(Settings.self, "settings.json") ?? Settings()
        accounts = (Disk.load([Account].self, "accounts.json") ?? []).sorted { $0.slot < $1.slot }
        for a in accounts {
            if let st = Disk.load(StoreData.self, "stores/\(a.id).json") { stores[a.id] = st }
        }
        seen = Disk.load([String: Int].self, "prices.json") ?? [:]
        notified = Disk.load([String: Int64].self, "notified.json") ?? [:]
        if let saved = ContentLoader.loadSaved() { content = ContentIndex(saved) }
    }

    // MARK: 저장

    private func saveAccounts() { if !Demo.on { Disk.save(accounts, "accounts.json") } }
    func saveSettings() { if !Demo.on { Disk.save(settings, "settings.json") } }
    private func saveStore(_ id: String) {
        guard !Demo.on, let st = stores[id] else { return }
        Disk.save(st, "stores/\(id).json")
    }

    private func putCookies(_ id: String, _ c: [RiotCookie]) {
        if let d = try? JSONEncoder().encode(c) { Keychain.set(id, d) }
    }

    private func cookies(_ id: String) -> [RiotCookie]? {
        guard let d = Keychain.get(id) else { return nil }
        return try? JSONDecoder().decode([RiotCookie].self, from: d)
    }

    private func updateAccount(_ id: String, _ fn: (inout Account) -> Void) {
        guard let i = accounts.firstIndex(where: { $0.id == id }) else { return }
        fn(&accounts[i])
        saveAccounts()
    }

    func say(_ text: String, bad: Bool = false) { notice = Notice(text: text, bad: bad) }

    // MARK: 시작 · 앞으로 나올 때

    func start() async {
        guard !started else { return }
        started = true
        Log.write("시작 v\(appVersion) · iOS \(UIDevice.current.systemVersion) · 계정 \(accounts.count)개\(Demo.on ? " · 시험 모드" : "")")
        if !Demo.on { await askNotifications() }
        await loadContent()
        if Demo.on { Demo.setup(self) } else { await refreshDue() }
        schedulePassAlert()
        scheduleBackground()
    }

    func onForeground() async {
        guard started else { return }
        pass = AppPass.read()
        if Demo.on { return }
        if Date().timeIntervalSince(lastContentCheck) > 6 * 3600 { await loadContent() }
        await refreshDue()
        schedulePassAlert()
    }

    func onBackground() {
        scheduleBackground()
    }

    // MARK: 스킨 정보

    func loadContent(force: Bool = false) async {
        if contentLoading { return }
        contentLoading = true
        defer { contentLoading = false }
        lastContentCheck = Date()
        do {
            let (v, cv) = try await ContentLoader.version()
            if !force, let d = content.data, d.format == ContentData.currentFormat, d.version == v, d.clientVersion == cv {
                contentError = ""
                return
            }
            let d = try await ContentLoader.fetch(version: v, clientVersion: cv)
            content = ContentIndex(d)
            contentError = ""
            Log.write("스킨 정보 v\(v) · 스킨 \(d.skins.count)개 · 번들 \(d.bundles.count)개")
            if !Demo.on { Task.detached(priority: .utility) { ContentLoader.save(d) } }
        } catch {
            contentError = error.localizedDescription
            Log.write("⚠️ 스킨 정보: \(error.localizedDescription)")
        }
    }

    // MARK: 계정

    var activeAccount: Account? {
        accounts.first { $0.id == settings.active } ?? accounts.first
    }

    func select(_ id: String) {
        guard settings.active != id else { return }
        settings.active = id
        saveSettings()
        Task { await refresh(id, force: false) }
    }

    func freeSlot() -> Int? {
        let used = Set(accounts.map(\.slot))
        return (1...Self.maxAccounts).first { !used.contains($0) }
    }

    /// 로그인 화면에서 받은 토큰 · 쿠키로 계정을 저장해요 (맥 엔진 Accounts.Login 과 같음)
    func login(slot: Int, access: String, idToken: String, cookies raw: [RiotCookie]) async throws -> String {
        if Demo.on { throw RiotError.message("시험 모드에서는 로그인할 수 없어요") }
        let ck = raw.filter(\.isRiot)
        let info = try await Riot.userInfo(access)
        var region = "kr"
        do { region = try await Riot.region(access, idToken) } catch {
            Log.write("로그인: 지역 확인 실패(\(error.localizedDescription)) → kr 로 가정")
        }
        let ent = try await Riot.entitlements(access)

        var note = ""
        let id: String
        if let existing = accounts.first(where: { $0.puuid == info.puuid }) {
            id = existing.id
            if slot != 0 && slot != existing.slot {
                note = "이미 \(existing.slotText)번에 저장된 계정이라 그 자리를 새로 고쳤어요"
            }
        } else {
            var s = slot
            if s < 1 || s > Self.maxAccounts {
                guard let f = freeSlot() else {
                    throw RiotError.message("계정 자리 3개가 모두 찼어요. 설정에서 한 자리를 비운 뒤 추가해 주세요")
                }
                s = f
            }
            if let old = accounts.first(where: { $0.slot == s }) { remove(old.id, quiet: true) }
            id = Fmt.randomID(8)
            accounts.append(Account(id: id, slot: s, puuid: info.puuid, name: info.name, tag: info.tag, region: region,
                                    shard: Riot.shard(for: region), added: Int64(Date().timeIntervalSince1970)))
            accounts.sort { $0.slot < $1.slot }
        }
        updateAccount(id) {
            $0.puuid = info.puuid; $0.name = info.name; $0.tag = info.tag
            $0.region = region; $0.shard = Riot.shard(for: region)
            $0.status = "ok"; $0.error = ""; $0.lastOK = Int64(Date().timeIntervalSince1970)
        }
        sessions[id] = Session(access: access, id: idToken, ent: ent, expires: Date().addingTimeInterval(55 * 60))
        putCookies(id, ck)
        if Keychain.get(id) == nil {
            note = "자동 로그인 정보를 저장하지 못했어요. 다음에 앱을 열면 다시 로그인해야 할 수 있어요"
        } else if !ck.contains(where: { $0.name == "ssid" }) && note.isEmpty {
            note = "‘로그인 상태 유지’가 꺼져 있었던 것 같아요. 나중에 다시 로그인해야 할 수 있어요"
        }
        Log.write("로그인: 슬롯 \(accounts.first { $0.id == id }?.slot ?? 0) · 지역 \(region) · 쿠키 \(ck.count)개 (ssid \(ck.contains { $0.name == "ssid" }))")
        settings.active = id
        saveSettings()
        Task { await refresh(id, force: true) }
        return note
    }

    func remove(_ id: String, quiet: Bool = false) {
        accounts.removeAll { $0.id == id }
        stores[id] = nil
        sessions[id] = nil
        saveAccounts()
        if !Demo.on {
            Keychain.delete(id)
            Disk.remove("stores/\(id).json")
        }
        if settings.active == id {
            settings.active = accounts.first?.id ?? ""
            saveSettings()
        }
        if !quiet { Log.write("계정 지움") }
    }

    // MARK: 상점 받기

    /// 앱을 열 때: 상점이 바뀌었거나 오래된 계정만 다시 받아요
    func refreshDue() async {
        let now = Int64(Date().timeIntervalSince1970)
        await withTaskGroup(of: Void.self) { group in
            for acc in accounts {
                let st = stores[acc.id]
                var due = st == nil || st!.dailyEnds <= now
                if let st, acc.id == activeAccount?.id, now - st.fetched > 15 * 60 { due = true }
                if let st, st.nightEnds > 0, now >= st.nightEnds, !st.night.isEmpty { due = true }
                if acc.status == "error" { due = true }
                if acc.status == "expired" {
                    // 만료된 계정은 6시간마다 한 번만 다시 시도해 봐요 (일시적인 문제였을 수도 있어서)
                    if let t = lastTry[acc.id], Date().timeIntervalSince(t) < 6 * 3600 { due = false } else { due = true }
                }
                if due {
                    let id = acc.id
                    group.addTask { await self.refresh(id, force: true) }
                }
            }
        }
    }

    func refresh(_ id: String, force: Bool) async {
        guard let acc = accounts.first(where: { $0.id == id }), !busy.contains(id) else { return }
        let now = Int64(Date().timeIntervalSince1970)
        if !force, let st = stores[id], st.dailyEnds > now, now - st.fetched < 10 * 60 { return }
        busy.insert(id)
        defer { busy.remove(id) }
        lastTry[id] = Date()
        do {
            let st = try await fetchStore(acc)
            stores[id] = st
            saveStore(id)
            mergeSeen(st.seenPrices)
            updateAccount(id) { $0.status = "ok"; $0.error = ""; $0.lastOK = Int64(Date().timeIntervalSince1970) }
            Log.write("계정 \(acc.slot): 상점 받음 (오늘 \(st.daily.count) · 밤시장 \(st.night.count) · 번들 \(st.bundles.count))")
            checkHits(acc, st)
        } catch {
            let e = (error as? RiotError) ?? .message(error.localizedDescription)
            let expired = e == .expired
            Log.write("⚠️ 계정 \(acc.slot): 상점 받기 실패: \(e.localizedDescription)")
            updateAccount(id) { $0.status = expired ? "expired" : "error"; $0.error = e.localizedDescription }
        }
    }

    private func fetchStore(_ acc: Account) async throws -> StoreData {
        if Demo.on {
            if !content.ready { await loadContent() }
            try? await Task.sleep(nanoseconds: 300_000_000)
            return Demo.store(acc, content)
        }
        if !content.ready { await loadContent() }
        var sf: [String: Any]?
        var wallet: Wallet?
        for attempt in 0..<2 {
            let s = try await ensureSession(acc, force: attempt > 0)
            do {
                sf = try await Riot.storefront(s, shard: acc.shard, puuid: acc.puuid, clientVersion: content.clientVersion)
            } catch let e as RiotError {
                if case .rejected = e, attempt == 0 { continue }
                throw e
            }
            wallet = try? await Riot.wallet(s, shard: acc.shard, puuid: acc.puuid, clientVersion: content.clientVersion)
            break
        }
        guard let sf else { throw RiotError.message("상점 정보를 받지 못했어요") }
        var st = buildStore(sf)
        st.wallet = wallet ?? stores[acc.id]?.wallet
        return st
    }

    /// 토큰이 오래됐으면 키체인의 쿠키로 새로 받아요
    private func ensureSession(_ acc: Account, force: Bool) async throws -> Session {
        if !force, let s = sessions[acc.id], s.valid { return s }
        guard let ck = cookies(acc.id), !ck.isEmpty else { throw RiotError.expired }
        do {
            let (t, merged) = try await Riot.reauth(ck)
            putCookies(acc.id, merged)
            let ent = try await Riot.entitlements(t.access)
            let s = Session(access: t.access, id: t.id, ent: ent, expires: t.expires)
            sessions[acc.id] = s
            return s
        } catch let f as Riot.ReauthFailure {
            if !f.cookies.isEmpty { putCookies(acc.id, f.cookies) }
            throw f.error
        }
    }

    // MARK: 가격

    private func mergeSeen(_ m: [String: Int]) {
        var changed = false
        for (k, v) in m where v > 0 && seen[k.lowercased()] != v {
            seen[k.lowercased()] = v
            changed = true
        }
        if changed && !Demo.on { Disk.save(seen, "prices.json") }
    }

    /// 스킨 가격: 상점에서 본 가격(ok) · 등급별 보통 가격(약) · 모름
    func price(_ s: Skin) -> (Int, PriceState) {
        if let v = seen[s.level], v > 0 { return (v, .ok) }
        if Demo.on, tierPrice(s) > 0 { return (tierPrice(s), .ok) }
        if tierPrice(s) > 0 { return (tierPrice(s), .estimate) }
        return (0, .unknown)
    }

    // MARK: 화면용

    var wishSet: Set<String> { Set(settings.wish) }

    func offers(_ list: [RawOffer]) -> [Offer] {
        let wish = wishSet
        return list.map { o in
            if let s = content.skin(o.item) {
                return Offer(item: o.item, skin: s, known: true, price: o.price, base: o.base, pct: o.pct, wished: wish.contains(s.id))
            }
            return Offer(item: o.item, skin: .placeholder(o.item), known: false, price: o.price, base: o.base, pct: o.pct, wished: false)
        }
    }

    func hits(_ acc: Account) -> [Hit] {
        guard let st = stores[acc.id] else { return [] }
        let wish = wishSet
        let now = Int64(Date().timeIntervalSince1970)
        var out: [Hit] = []
        if st.dailyEnds > now {
            for o in st.daily {
                if let s = content.skin(o.item), wish.contains(s.id) {
                    out.append(Hit(accountID: acc.id, label: acc.label, slot: acc.slot, skin: s, night: false, price: o.price, base: 0))
                }
            }
        }
        if settings.nightAlert && st.nightEnds > now {
            for o in st.night {
                if let s = content.skin(o.item), wish.contains(s.id) {
                    out.append(Hit(accountID: acc.id, label: acc.label, slot: acc.slot, skin: s, night: true, price: o.price, base: o.base))
                }
            }
        }
        return out
    }

    var allHits: [Hit] { accounts.flatMap { hits($0) } }

    func setWish(_ skinID: String, _ on: Bool) {
        var id = skinID.lowercased()
        if let s = content.skin(id) { id = s.id }
        var list = settings.wish.filter { $0 != id }
        if on { list.insert(id, at: 0) }
        settings.wish = Array(list.prefix(200))
        saveSettings()
    }

    func isWished(_ skinID: String) -> Bool {
        let id = content.skin(skinID)?.id ?? skinID.lowercased()
        return settings.wish.contains(id)
    }

    // MARK: 알림

    func askNotifications() async {
        let c = UNUserNotificationCenter.current()
        let st = await c.notificationSettings()
        if st.authorizationStatus == .notDetermined {
            _ = try? await c.requestAuthorization(options: [.alert, .sound, .badge])
        }
        settings.askedNotify = true
        saveSettings()
    }

    func notificationsAllowed() async -> Bool {
        let st = await UNUserNotificationCenter.current().notificationSettings()
        return st.authorizationStatus == .authorized || st.authorizationStatus == .provisional
    }

    private func post(_ id: String, _ title: String, _ body: String, after: TimeInterval? = nil) {
        let c = UNMutableNotificationContent()
        c.title = title
        c.body = body
        c.sound = .default
        let trigger = after.map { UNTimeIntervalNotificationTrigger(timeInterval: max(1, $0), repeats: false) }
        UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: id, content: c, trigger: trigger))
    }

    func testNotification() async {
        if !(await notificationsAllowed()) {
            say("알림이 꺼져 있어요. 아이폰 설정 › 알림 › ValoROAD 에서 켜 주세요", bad: true)
            return
        }
        post("test-" + Fmt.randomID(4), "ValoROAD 알림 시험", "위시리스트 스킨이 뜨면 이렇게 알려 드려요.", after: 3)
        say("3초 뒤에 시험 알림이 와요")
    }

    /// 이 계정 상점에 위시리스트 스킨이 있으면 (아직 안 알렸으면) 알림
    private func checkHits(_ acc: Account, _ st: StoreData) {
        guard settings.wishAlert, !settings.wish.isEmpty, !Demo.on else { return }
        let wish = wishSet
        let now = Int64(Date().timeIntervalSince1970)
        let kinds: [(kind: String, title: String, place: String, list: [RawOffer], ends: Int64, on: Bool)] = [
            ("daily", "위시리스트 스킨 등장!", "상점", st.daily, st.dailyEnds, true),
            ("night", "밤시장에 위시리스트 스킨!", "밤시장", st.night, st.nightEnds, settings.nightAlert)
        ]
        for k in kinds where k.on && k.ends > now {
            var fresh: [String] = []
            for o in k.list {
                guard let s = content.skin(o.item), wish.contains(s.id) else { continue }
                let key = "\(acc.id)|\(s.id)|\(k.kind)|\(k.ends / 600)"
                if notified[key] == nil {
                    notified[key] = now
                    fresh.append(s.name)
                }
            }
            guard let first = fresh.first else { continue }
            let body = fresh.count > 1
                ? "\(acc.label) \(k.place)에 ‘\(first)’ 외 \(fresh.count - 1)개가 떴어요."
                : "\(acc.label) \(k.place)에 ‘\(first)’\(Fmt.josa(first)) 떴어요."
            post("hit-" + Fmt.randomID(6), k.title, body)
        }
        notified = notified.filter { now - $0.value < 14 * 86400 }
        Disk.save(notified, "notified.json")
    }

    /// 사용 기간이 하루 남으면 알림 (맥에서 다시 설치할 때)
    func schedulePassAlert() {
        let c = UNUserNotificationCenter.current()
        c.removePendingNotificationRequests(withIdentifiers: ["pass-1day", "pass-2h"])
        guard settings.passAlert, !Demo.on, let e = pass.expires else { return }
        let left = e.timeIntervalSinceNow
        if left > 86400 + 60 {
            post("pass-1day", "ValoROAD 사용 기간이 하루 남았어요",
                 "아이폰을 맥에 연결하고 Xcode 에서 ▶ 를 누르면 다시 7일이 시작돼요. 계정 · 위시리스트는 그대로예요.", after: left - 86400)
        }
        if left > 2 * 3600 + 60 {
            post("pass-2h", "ValoROAD 가 2시간 뒤에 멈춰요",
                 "오늘 맥에서 Xcode 의 ▶ 를 한 번 눌러 주세요.", after: left - 2 * 3600)
        }
    }

    // MARK: 뒤에서 확인 (BGAppRefresh)

    func scheduleBackground() {
        guard !Demo.on else { return }
        let req = BGAppRefreshTaskRequest(identifier: Self.backgroundID)
        let now = Int64(Date().timeIntervalSince1970)
        let stale = accounts.contains { a in (stores[a.id]?.dailyEnds ?? 0) <= now && a.status != "expired" }
        req.earliestBeginDate = stale ? Date().addingTimeInterval(15 * 60) : nextDailyReset().addingTimeInterval(90)
        do {
            BGTaskScheduler.shared.cancel(taskRequestWithIdentifier: Self.backgroundID)
            try BGTaskScheduler.shared.submit(req)
        } catch {
            Log.write("뒤에서 확인 예약 실패: \(error.localizedDescription)")
        }
    }

    func backgroundCheck() async {
        Log.write("뒤에서 상점 확인")
        let now = Int64(Date().timeIntervalSince1970)
        for acc in accounts where acc.status != "expired" {
            if (stores[acc.id]?.dailyEnds ?? 0) <= now {
                await refresh(acc.id, force: true)
            }
        }
        scheduleBackground()
    }

    // MARK: 진단

    func diagnostics() -> String {
        var lines: [String] = []
        lines.append("ValoROAD 아이폰 v\(appVersion) · iOS \(UIDevice.current.systemVersion) · \(UIDevice.current.model)")
        if pass.expires != nil {
            lines.append("사용 기간: \(pass.untilText)까지 (남은 \(pass.daysLeft ?? 0)일) · \(pass.free ? "무료 애플 ID" : "개발자 계정") \(pass.teamName)")
        } else {
            lines.append("사용 기간: 정보 없음 (시뮬레이터이거나 허가서가 없어요)")
        }
        let d = content.data
        lines.append("스킨 정보: \(d.map { "v\($0.version) · 스킨 \($0.skins.count)개" } ?? "없음")\(contentError.isEmpty ? "" : " · 오류: \(contentError)")")
        lines.append("가격: 상점에서 본 가격 \(seen.count)개")
        lines.append("위시리스트: \(settings.wish.count)개 · 알림 \(settings.wishAlert ? "켬" : "끔") · 밤시장 \(settings.nightAlert ? "켬" : "끔")")
        lines.append("키체인 시험: \(Keychain.selfTest() ? "통과" : "실패")")
        for a in accounts {
            let st = stores[a.id]
            let age = st.map { "\(Int(Date().timeIntervalSince1970) - Int($0.fetched))초 전" } ?? "없음"
            lines.append("계정 \(a.slotText) \(a.label) [\(a.region.uppercased())] 상태 \(a.status) · 상점 \(age) · 열쇠 \(Keychain.get(a.id) != nil ? "있음" : "없음")\(a.error.isEmpty ? "" : " · \(a.error)")")
        }
        lines.append("")
        lines.append("— 최근 기록 —")
        lines.append(contentsOf: Log.text().split(separator: "\n").suffix(60).map(String.init))
        return lines.joined(separator: "\n")
    }
}
