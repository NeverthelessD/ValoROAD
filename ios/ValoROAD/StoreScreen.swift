import SwiftUI

// 상점 화면 (시안 ① 다크 · ② 라이트)
// 위: 계정 3칸 전환 → 지갑 줄 → 위시 적중 안내 → DAILY OFFERS(2열 카드) → 추천 번들 → 밤시장 → 만든 이

struct StoreScreen: View {
    @EnvironmentObject private var model: AppModel
    @EnvironmentObject private var router: Router
    @Environment(\.palette) private var p

    var body: some View {
        ScrollViewReader { proxy in
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                HStack {
                    BrandMark()
                    Spacer()
                    RefreshButton()
                }
                AccountSlots()
                if let acc = model.activeAccount {
                    WalletRow(wallet: model.stores[acc.id]?.wallet)
                    AccountProblem(acc: acc)
                    HitAlert()
                    StoreBody(acc: acc)
                } else {
                    Welcome()
                }
                Footer()
            }
            .padding(.horizontal, 16)
            .padding(.top, 8)
            .padding(.bottom, 20)
        }
        .scrollIndicators(.hidden)
        .refreshable {
            if let id = model.activeAccount?.id { await model.refresh(id, force: true) }
        }
        .task {
            // 시험 화면: 번들 · 밤시장까지 내려서 찍을 때 (-scroll bundle|night)
            guard Demo.on, let target = Demo.arg("-scroll") else { return }
            try? await Task.sleep(nanoseconds: 7_000_000_000)
            withAnimation { proxy.scrollTo(target, anchor: .top) }
        }
        }
    }
}

// MARK: - 새로고침

private struct RefreshButton: View {
    @EnvironmentObject private var model: AppModel
    @State private var spin = false

    var body: some View {
        let busy = model.activeAccount.map { model.busy.contains($0.id) } ?? false
        Button {
            guard let id = model.activeAccount?.id else { return }
            Task { await model.refresh(id, force: true) }
        } label: {
            Image(systemName: "arrow.clockwise")
                .font(.system(size: 18, weight: .bold))
                .rotationEffect(.degrees(busy ? 360 : 0))
                .animation(busy ? .linear(duration: 0.9).repeatForever(autoreverses: false) : .default, value: busy)
                .foregroundStyle(Palette.navy)
                .frame(width: 44, height: 44)
                .background(Palette.crimson, in: Notch(cut: 10))
        }
        .buttonStyle(.plain)
        .disabled(model.activeAccount == nil)
        .accessibilityLabel(busy ? "상점 받는 중" : "상점 새로고침")
    }
}

// MARK: - 계정 3칸

private struct AccountSlots: View {
    @EnvironmentObject private var model: AppModel
    @EnvironmentObject private var router: Router
    @Environment(\.palette) private var p

    var body: some View {
        HStack(spacing: 8) {
            ForEach(1...AppModel.maxAccounts, id: \.self) { slot in
                if let acc = model.accounts.first(where: { $0.slot == slot }) {
                    slotButton(acc)
                } else {
                    emptySlot(slot)
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel("저장된 계정")
    }

    private func slotButton(_ acc: Account) -> some View {
        let on = acc.id == model.activeAccount?.id
        let hits = model.hits(acc).count
        let fg = on ? Palette.navy : p.text
        return Button {
            model.select(acc.id)
        } label: {
            VStack(alignment: .leading, spacing: 4) {
                HStack(alignment: .top) {
                    Text(acc.slotText)
                        .font(VF.anton(24))
                        .foregroundStyle(on ? Palette.navy : p.muted.opacity(0.7))
                    Spacer(minLength: 2)
                    if acc.status == "expired" {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .font(.system(size: 12))
                            .foregroundStyle(on ? Palette.navy : p.red)
                    } else if hits > 0 {
                        Text("위시 \(hits)")
                            .font(VF.plex(11, .bold))
                            .padding(.horizontal, 6).padding(.vertical, 1)
                            .foregroundStyle(on ? Palette.cream : p.onHit)
                            .background(on ? Palette.navy : p.hit)
                    }
                }
                Text(acc.name)
                    .font(VF.plex(14, .bold))
                    .lineLimit(1)
                    .minimumScaleFactor(0.7)
                Text("#\(acc.tag)")
                    .font(VF.barlow(13, .semibold))
                    .tracking(0.8)
                    .opacity(0.85)
                    .lineLimit(1)
            }
            .foregroundStyle(fg)
            .padding(.horizontal, 10)
            .padding(.top, 10)
            .padding(.bottom, 12)
            .frame(maxWidth: .infinity, minHeight: 76, alignment: .topLeading)
            .background(on ? Palette.crimson : p.panel, in: Notch(cut: 12))
            .overlay {
                if model.busy.contains(acc.id) {
                    ProgressView().tint(fg).scaleEffect(0.7).frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .bottomTrailing).padding(8)
                }
            }
        }
        .buttonStyle(.plain)
        .accessibilityLabel("\(acc.slotText)번 \(acc.label)\(acc.status == "expired" ? ", 로그인이 풀림" : "")\(hits > 0 ? ", 위시 \(hits)개" : "")")
        .accessibilityAddTraits(on ? .isSelected : [])
    }

    private func emptySlot(_ slot: Int) -> some View {
        Button {
            router.openLogin(slot: slot)
        } label: {
            VStack(alignment: .leading, spacing: 4) {
                Text(String(format: "%02d", slot)).font(VF.anton(24)).foregroundStyle(p.muted.opacity(0.6))
                Spacer(minLength: 0)
                HStack(spacing: 4) {
                    Image(systemName: "plus").font(.system(size: 11, weight: .heavy))
                    Text("계정 추가").font(VF.plex(13, .bold))
                }
                .foregroundStyle(p.muted)
            }
            .padding(10)
            .frame(maxWidth: .infinity, minHeight: 76, alignment: .topLeading)
            .overlay(Notch(cut: 12).stroke(p.line, style: StrokeStyle(lineWidth: 1.5, dash: [5, 4])))
        }
        .buttonStyle(.plain)
        .accessibilityLabel("\(slot)번 자리에 계정 추가")
    }
}

// MARK: - 지갑

private struct WalletRow: View {
    @Environment(\.palette) private var p
    let wallet: Wallet?

    var body: some View {
        HStack(spacing: 1) {
            cell(icon: AnyView(VPIcon(size: 14, color: p.text)), value: wallet?.vp, unit: "VP", label: "발로란트 포인트")
            cell(icon: AnyView(Image(systemName: "hexagon").font(.system(size: 13, weight: .semibold))), value: wallet?.rp, unit: "RP", label: "레디언트 포인트")
            cell(icon: AnyView(Image(systemName: "crown").font(.system(size: 12, weight: .semibold))), value: wallet?.kc, unit: "KC", label: "킹덤 크레딧")
        }
        .background(p.line)
    }

    private func cell(icon: AnyView, value: Int?, unit: String, label: String) -> some View {
        HStack(spacing: 6) {
            icon
            Text(value.map(Fmt.n) ?? "–").font(VF.barlow(18))
            Text(unit).font(VF.barlow(12, .semibold)).tracking(1.2).foregroundStyle(p.muted)
        }
        .foregroundStyle(p.text)
        .frame(maxWidth: .infinity, minHeight: 44)
        .background(p.panel)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(label) \(value.map(Fmt.n) ?? "모름")")
    }
}

// MARK: - 로그인 풀림 · 오류

private struct AccountProblem: View {
    @EnvironmentObject private var model: AppModel
    @EnvironmentObject private var router: Router
    @Environment(\.palette) private var p
    let acc: Account

    var body: some View {
        if acc.status == "expired" {
            VStack(alignment: .leading, spacing: 10) {
                Label("로그인이 풀렸어요", systemImage: "lock.fill")
                    .font(VF.plex(15, .bold))
                Text("이 계정만 다시 로그인하면 돼요. 다른 계정은 그대로예요.")
                    .font(VF.plex(13))
                Button { router.openLogin(slot: acc.slot) } label: {
                    Text("다시 로그인")
                        .font(VF.plex(14, .bold))
                        .foregroundStyle(Palette.crimson)
                        .padding(.horizontal, 14).frame(minHeight: 40)
                        .background(Palette.navy)
                }
                .buttonStyle(.plain)
            }
            .foregroundStyle(Palette.navy)
            .padding(14)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Palette.crimson, in: Notch(cut: 12))
        } else if acc.status == "error", !acc.error.isEmpty {
            HStack(alignment: .top, spacing: 10) {
                Image(systemName: "wifi.exclamationmark").foregroundStyle(p.red)
                VStack(alignment: .leading, spacing: 8) {
                    Text(acc.error).font(VF.plex(13, .medium)).foregroundStyle(p.text)
                    Button("다시 시도") { Task { await model.refresh(acc.id, force: true) } }
                        .font(VF.plex(13, .bold))
                        .foregroundStyle(p.redText)
                }
            }
            .padding(14)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(p.panel, in: Notch(cut: 12))
        }
    }
}

// MARK: - 위시 적중 안내

private struct HitAlert: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.palette) private var p

    var body: some View {
        let hits = model.allHits
        let active = model.activeAccount?.id
        if let first = hits.first(where: { $0.accountID != active }) ?? hits.first {
            let other = first.accountID != active
            VStack(alignment: .leading, spacing: 8) {
                Label("WISHLIST HIT", systemImage: "bell")
                    .font(VF.barlow(16))
                    .tracking(1.3)
                Text(other
                     ? "\(first.label) \(first.night ? "밤시장" : "상점")에 ‘\(first.skin.name)’\(Fmt.josa(first.skin.name)) 떴어요.\(hits.count > 1 ? " (모두 \(hits.count)개)" : "")"
                     : "지금 보고 있는 \(first.night ? "밤시장" : "상점")에 ‘\(first.skin.name)’\(Fmt.josa(first.skin.name)) 있어요.")
                    .font(VF.plex(14, .medium))
                    .fixedSize(horizontal: false, vertical: true)
                if other {
                    Button { model.select(first.accountID) } label: {
                        Text("그 계정 상점 보기")
                            .font(VF.plex(13, .bold))
                            .foregroundStyle(p.hit)
                            .padding(.horizontal, 14).frame(minHeight: 40)
                            .background(p.onHit)
                    }
                    .buttonStyle(.plain)
                }
            }
            .foregroundStyle(p.onHit)
            .padding(.horizontal, 14).padding(.vertical, 12)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(p.hit, in: Notch(cut: 12))
            .accessibilityElement(children: .combine)
        }
    }
}

// MARK: - 상점 본문

private struct StoreBody: View {
    @EnvironmentObject private var model: AppModel
    @EnvironmentObject private var router: Router
    @Environment(\.palette) private var p
    let acc: Account

    var body: some View {
        let st = model.stores[acc.id]
        VStack(alignment: .leading, spacing: 18) {
            titleRow(st)
            if let st {
                let daily = model.offers(st.daily)
                LazyVGrid(columns: [GridItem(.flexible(), spacing: 12), GridItem(.flexible(), spacing: 12)], spacing: 12) {
                    ForEach(daily) { o in
                        ItemCard(offer: o) {
                            router.preview = PreviewTarget(skin: o.skin, price: o.price, context: "\(acc.label) 오늘의 상점", ends: st.dailyEnds)
                        }
                    }
                }
                Text("카드를 누르면 미리보기 · 하트를 누르면 위시리스트에 담겨요")
                    .font(VF.plex(12))
                    .foregroundStyle(p.muted)
                    .padding(.top, -8)
                let now = Int64(Date().timeIntervalSince1970)
                ForEach(Array(st.bundles.filter { $0.ends == 0 || $0.ends > now }.enumerated()), id: \.offset) { _, b in
                    BundleCard(bundle: b, info: model.content.bundle(b.id))
                }
                .id("bundle")
                if st.nightEnds > now && !st.night.isEmpty {
                    NightMarket(acc: acc, offers: model.offers(st.night), ends: st.nightEnds)
                        .id("night")
                }
            } else if model.busy.contains(acc.id) {
                loadingBox("\(acc.label) 상점을 받는 중이에요…")
            } else if acc.status == "ok" {
                loadingBox("아직 받은 상점이 없어요. 위의 새로고침을 눌러 주세요")
            }
        }
        .task(id: st?.dailyEnds ?? 0) {
            // 상점이 바뀌는 시각(한국 오전 9시)이 지나면 저절로 다시 받아요
            guard let ends = st?.dailyEnds, ends > 0 else { return }
            let wait = Double(ends) - Date().timeIntervalSince1970 + 15
            if wait > 0 { try? await Task.sleep(nanoseconds: UInt64(wait * 1_000_000_000)) }
            if !Task.isCancelled { await model.refresh(acc.id, force: true) }
        }
    }

    private func titleRow(_ st: StoreData?) -> some View {
        HStack(alignment: .bottom, spacing: 10) {
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 8) {
                    Rectangle().fill(p.red).frame(width: 18, height: 3)
                    Text(acc.label).font(VF.barlow(14)).tracking(1.8)
                }
                .foregroundStyle(p.redText)
                Text("DAILY OFFERS").font(VF.anton(40)).foregroundStyle(p.text)
                Text("오늘의 상점").font(VF.plex(13, .semibold)).foregroundStyle(p.muted)
            }
            Spacer(minLength: 4)
            VStack(alignment: .trailing, spacing: 0) {
                Text("다음 갱신").font(VF.barlow(12)).tracking(1.4).foregroundStyle(p.muted)
                Countdown(ends: st?.dailyEnds ?? Int64(nextDailyReset().timeIntervalSince1970))
                    .font(VF.anton(24))
                    .foregroundStyle(p.text)
            }
            .padding(.horizontal, 10).padding(.vertical, 8)
            .background(p.panel)
            .overlay(alignment: .top) { Rectangle().fill(p.red).frame(height: 3) }
        }
        .background(alignment: .topTrailing) {
            Watermark(text: "STORE", size: 108).offset(x: 14, y: -34)
        }
    }

    private func loadingBox(_ text: String) -> some View {
        HStack(spacing: 10) {
            ProgressView().tint(p.text)
            Text(text).font(VF.plex(14, .medium)).foregroundStyle(p.text)
        }
        .padding(18)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(p.panel, in: Notch(cut: 14))
    }
}

/// 남은 시간 "14:22:08" (1초마다)
struct Countdown: View {
    let ends: Int64
    var days = false
    var body: some View {
        TimelineView(.periodic(from: .now, by: 1)) { ctx in
            let left = Double(ends) - ctx.date.timeIntervalSince1970
            Text(days ? Fmt.days(left) : Fmt.clock(left)).monospacedDigit()
        }
    }
}

// MARK: - 스킨 카드

struct ItemCard: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.palette) private var p
    let offer: Offer
    let open: () -> Void

    var body: some View {
        let s = offer.skin
        let hit = offer.wished
        ZStack(alignment: .topTrailing) {
            Button(action: open) {
                VStack(alignment: .leading, spacing: 0) {
                    if hit {
                        Text("WISHLIST")
                            .font(VF.barlow(13)).tracking(1.6)
                            .foregroundStyle(p.onHit)
                            .padding(.horizontal, 10).padding(.vertical, 3)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(p.hit)
                    }
                    ZStack(alignment: .topLeading) {
                        tierTint(s.tier, p)
                        // 위쪽 등급 글자 · 오른쪽 하트와 겹치지 않게 여백을 두고 가운데에
                        RemoteImage(url: s.img)
                            .padding(.top, 30).padding(.bottom, 12).padding(.horizontal, 14)
                            .frame(maxWidth: .infinity, maxHeight: .infinity)
                            .foregroundStyle(p.text)
                        HStack(spacing: 6) {
                            Diamond().fill(Tier.color(s.tier)).frame(width: 9, height: 9)
                            Text(Tier.name(s.tier)).font(VF.plex(11, .bold)).foregroundStyle(p.text)
                        }
                        .padding(8)
                    }
                    .frame(height: 124)
                    Rectangle().fill(Tier.color(s.tier)).frame(height: 3)
                    VStack(alignment: .leading, spacing: 3) {
                        Text(s.weaponEn.isEmpty ? " " : s.weaponEn)
                            .font(VF.barlow(13)).tracking(1.5).foregroundStyle(p.muted)
                        Text(s.name)
                            .font(VF.plex(14, .bold)).foregroundStyle(p.text)
                            .lineLimit(2).multilineTextAlignment(.leading)
                            .frame(minHeight: 36, alignment: .topLeading)
                        HStack(spacing: 6) {
                            VPIcon(size: 13, color: p.text)
                            Text(offer.price > 0 ? Fmt.n(offer.price) : "–").font(VF.anton(21)).foregroundStyle(p.text)
                        }
                        .padding(.top, 4)
                    }
                    .padding(.horizontal, 12).padding(.top, 10).padding(.bottom, 14)
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
                .background(p.panel)
                .clipShape(Notch(cut: 17))
                .contentShape(Notch(cut: 17))
            }
            .buttonStyle(.plain)
            .accessibilityLabel("\(s.name), \(Tier.name(s.tier)), \(offer.price) VP\(hit ? ", 위시리스트" : ""). 미리보기 열기")

            Button {
                model.setWish(s.id, !hit)
            } label: {
                Image(systemName: hit ? "heart.fill" : "heart")
                    .font(.system(size: 18, weight: .semibold))
                    .foregroundStyle(hit ? p.hit : p.muted)
                    .frame(width: 44, height: 44)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .padding(.top, hit ? 22 : 0)
            .disabled(!offer.known)
            .accessibilityLabel(hit ? "\(s.name) 위시리스트에서 빼기" : "\(s.name) 위시리스트에 담기")
        }
        .padding(hit ? 3 : 1)
        .background(hit ? p.hit : p.line, in: Notch(cut: 18))
    }
}

// MARK: - 번들

private struct BundleCard: View {
    let bundle: RawBundle
    let info: BundleInfo?

    var body: some View {
        ZStack(alignment: .topLeading) {
            Palette.navy
            if let img = info?.img, !img.isEmpty {
                RemoteImage(url: img, mode: .fill)
                    .opacity(0.55)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .clipped()
                LinearGradient(colors: [Palette.navy, Palette.navy.opacity(0.75), .clear], startPoint: .leading, endPoint: .trailing)
            }
            HStack { Spacer(); Slant().fill(Palette.crimson.opacity(0.9)).frame(width: 90) }
            VStack(alignment: .leading, spacing: 8) {
                Text("FEATURED BUNDLE")
                    .font(VF.barlow(13)).tracking(1.8)
                    .foregroundStyle(Palette.navy)
                    .padding(.horizontal, 9).padding(.vertical, 3)
                    .background(Palette.crimson)
                Text(info?.name.isEmpty == false ? info!.name : "추천 번들")
                    .font(VF.plex(22, .bold))
                    .lineLimit(2)
                    .padding(.trailing, 70)
                HStack(spacing: 4) {
                    if bundle.ends > 0 {
                        Text("남은 기간").font(VF.plex(12)).foregroundStyle(Color(hex: 0xB8C1C9))
                        Countdown(ends: bundle.ends, days: true).font(VF.barlow(16))
                    }
                    if bundle.items > 0 {
                        Text("· \(bundle.items)종").font(VF.plex(12)).foregroundStyle(Color(hex: 0xB8C1C9))
                    }
                }
                Spacer(minLength: 8)
                HStack(alignment: .lastTextBaseline, spacing: 7) {
                    VPIcon(size: 15, color: Palette.cream)
                    Text(bundle.price > 0 ? Fmt.n(bundle.price) : "–").font(VF.anton(26))
                    if bundle.base > bundle.price && bundle.price > 0 {
                        Text(Fmt.n(bundle.base)).font(VF.barlow(15, .semibold)).strikethrough().foregroundStyle(Color(hex: 0xB8C1C9))
                    }
                }
            }
            .foregroundStyle(Palette.cream)
            .padding(18)
        }
        .frame(minHeight: 196)
        .clipShape(Notch(cut: 20))
        .accessibilityElement(children: .combine)
    }
}

// MARK: - 밤시장

private struct NightMarket: View {
    @EnvironmentObject private var router: Router
    @Environment(\.palette) private var p
    let acc: Account
    let offers: [Offer]
    let ends: Int64

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .firstTextBaseline) {
                Text("NIGHT MARKET").font(VF.anton(22)).tracking(0.4)
                Spacer()
                let d = max(0, Int((Double(ends) - Date().timeIntervalSince1970) / 86400))
                Text(d > 0 ? "D-\(d)" : "오늘 끝").font(VF.barlow(16)).foregroundStyle(p.redText)
            }
            .foregroundStyle(p.text)
            Text("밤시장 · 할인 품목").font(VF.plex(12)).foregroundStyle(p.muted).padding(.top, -4)
            ForEach(offers) { o in
                Button {
                    router.preview = PreviewTarget(skin: o.skin, price: o.price, base: o.base, context: "\(acc.label) 밤시장", ends: ends)
                } label: {
                    HStack(spacing: 10) {
                        Text("-\(o.pct)%")
                            .font(VF.anton(15))
                            .foregroundStyle(Palette.navy)
                            .frame(minWidth: 42)
                            .padding(.horizontal, 5).padding(.vertical, 3)
                            .background(Palette.crimson)
                        VStack(alignment: .leading, spacing: 1) {
                            HStack(spacing: 5) {
                                if o.wished { Image(systemName: "heart.fill").font(.system(size: 11)).foregroundStyle(p.hit) }
                                Text(o.skin.name).font(VF.plex(14, .semibold)).lineLimit(1)
                            }
                            Text(o.skin.weaponEn).font(VF.barlow(12, .semibold)).tracking(1.2).foregroundStyle(p.muted)
                        }
                        Spacer(minLength: 4)
                        VStack(alignment: .trailing, spacing: 0) {
                            Text("\(Fmt.n(o.price)) VP").font(VF.barlow(17))
                            if o.base > 0 { Text(Fmt.n(o.base)).font(VF.barlow(13, .medium)).strikethrough().foregroundStyle(p.muted) }
                        }
                    }
                    .foregroundStyle(p.text)
                    .padding(.top, 8)
                    .frame(minHeight: 52)
                    .overlay(alignment: .top) { Rectangle().fill(p.line).frame(height: 1) }
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel("\(o.skin.name) \(o.pct)퍼센트 할인 \(o.price) VP. 미리보기 열기")
            }
        }
        .padding(16)
        .background(p.panel, in: Notch(cut: 20))
    }
}

// MARK: - 계정이 없을 때

private struct Welcome: View {
    @EnvironmentObject private var router: Router
    @Environment(\.palette) private var p

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text("WELCOME").font(VF.anton(40)).foregroundStyle(p.text)
            Text("라이엇 계정을 추가하면 오늘의 상점 · 번들 · 밤시장을 바로 볼 수 있어요. 계정은 3개까지 저장되고, 다음부터는 자동으로 로그인돼요.")
                .font(VF.plex(14))
                .foregroundStyle(p.text)
                .fixedSize(horizontal: false, vertical: true)
            Button { router.openLogin(slot: 1) } label: {
                HStack(spacing: 8) {
                    Image(systemName: "plus").font(.system(size: 14, weight: .heavy))
                    Text("첫 계정 추가하기").font(VF.plex(15, .bold))
                }
                .foregroundStyle(Palette.navy)
                .frame(maxWidth: .infinity, minHeight: 52)
                .background(Palette.crimson, in: Notch(cut: 14))
            }
            .buttonStyle(.plain)
            Label("비밀번호는 앱이 보지도, 저장하지도 않아요", systemImage: "checkmark")
                .font(VF.plex(12)).foregroundStyle(p.muted)
        }
        .padding(18)
        .background(p.panel, in: Notch(cut: 18))
    }
}
