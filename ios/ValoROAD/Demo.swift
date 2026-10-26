import Foundation

// 시험 모드 (데모): 라이엇에 로그인하지 않고, 실제 스킨 정보로 가짜 계정 3개의 상점을 만들어 보여 줘요.
// 맥 빌드 서버가 아이폰 시뮬레이터에서 화면을 찍을 때만 써요. 켜는 법: 실행 인자 -demo
// 화면 고르기: -tab store|wishlist|settings · -theme dark|light · -active 1|2|3 · -preview · -login

enum Demo {
    static let on: Bool = ProcessInfo.processInfo.arguments.contains("-demo")
        || ProcessInfo.processInfo.environment["VALOROAD_DEMO"] == "1"

    static func arg(_ name: String) -> String? {
        let a = ProcessInfo.processInfo.arguments
        guard let i = a.firstIndex(of: name), i + 1 < a.count, !a[i + 1].hasPrefix("-") else { return nil }
        return a[i + 1]
    }

    static func flag(_ name: String) -> Bool { ProcessInfo.processInfo.arguments.contains(name) }

    static let wallets: [Int: Wallet] = [
        1: Wallet(vp: 2350, rp: 40, kc: 1850),
        2: Wallet(vp: 480, rp: 0, kc: 3120),
        3: Wallet(vp: 10200, rp: 120, kc: 640)
    ]

    static func accounts() -> [Account] {
        let now = Int64(Date().timeIntervalSince1970)
        return [
            Account(id: "demoslot1", slot: 1, puuid: "demo1", name: "본캐", tag: "KR1", region: "kr", shard: "kr", added: now),
            Account(id: "demoslot2", slot: 2, puuid: "demo2", name: "부캐", tag: "KR2", region: "kr", shard: "kr", added: now),
            Account(id: "demoslot3", slot: 3, puuid: "demo3", name: "듀오용", tag: "7777", region: "kr", shard: "kr", added: now,
                    status: "expired", error: RiotError.expired.localizedDescription)
        ]
    }

    private static func day() -> Int64 { Int64(Date().timeIntervalSince1970) / 86400 }

    static func store(_ acc: Account, _ c: ContentIndex) -> StoreData {
        let now = Date()
        let t = Int64(now.timeIntervalSince1970)
        var st = StoreData(fetched: t)
        st.dailyEnds = Int64(nextDailyReset(now).timeIntervalSince1970)
        st.nightEnds = t + 5 * 86400 + 3600
        for s in c.random(4, seed: day() * 10 + Int64(acc.slot)) {
            st.daily.append(RawOffer(item: s.level, price: tierPrice(s)))
        }
        let pcts = [32, 25, 41, 18, 37, 29]
        for (i, s) in c.random(6, seed: day() * 10 + Int64(acc.slot) + 500, tierOK: { $0 != "select" }).enumerated() {
            let base = tierPrice(s)
            let pct = pcts[i % pcts.count]
            st.night.append(RawOffer(item: s.level, price: base * (100 - pct) / 100, base: base, pct: pct))
        }
        if let d = c.data {
            let ids = d.bundles.values
                .filter { !$0.img.isEmpty && !$0.name.isEmpty && !$0.name.contains("VCT") && !$0.name.contains(" x ") }
                .map(\.id).sorted()
            if !ids.isEmpty {
                st.bundles.append(RawBundle(id: ids[Int(day()) % ids.count], price: 7100, base: 8875, ends: t + 3 * 86400 + 7 * 3600, items: 4))
            }
        }
        st.wallet = wallets[acc.slot]
        return st
    }

    /// 시험 화면을 시안처럼: 계정 3개, 계정 2의 상점 스킨 하나가 위시리스트에 있음
    @MainActor
    static func setup(_ m: AppModel) {
        m.accounts = accounts()
        for a in m.accounts { m.stores[a.id] = store(a, m.content) }
        let active = Int(arg("-active") ?? "1") ?? 1
        m.settings.active = "demoslot\(min(3, max(1, active)))"
        var wish: [String] = []
        if let st2 = m.stores["demoslot2"], st2.daily.count > 1, let s = m.content.skin(st2.daily[1].item) { wish.append(s.id) }
        for s in m.content.random(4, seed: 4242, tierOK: { $0 == "premium" || $0 == "exclusive" || $0 == "ultra" }) where !wish.contains(s.id) {
            wish.append(s.id)
        }
        m.settings.wish = wish
    }
}
