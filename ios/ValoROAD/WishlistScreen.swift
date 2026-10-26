import SwiftUI

// 위시리스트 화면 (시안 ⑤): 담은 스킨 · 검색해서 담기 · 무기 종류 · 알림 설정
// 위시리스트 스킨이 계정 3개 중 어느 상점(또는 밤시장)에 뜨면 그 줄이 청록으로 바뀌고 알림이 와요.

struct WishlistScreen: View {
    @EnvironmentObject private var model: AppModel
    @EnvironmentObject private var router: Router
    @Environment(\.palette) private var p
    @State private var query = ""
    @State private var cat = "all"
    @FocusState private var searching: Bool

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                BrandMark()
                header
                searchBox
                if query.trimmingCharacters(in: .whitespaces).isEmpty {
                    wishList
                } else {
                    results
                }
                AlertSettings()
                Footer()
            }
            .padding(.horizontal, 16)
            .padding(.top, 8)
            .padding(.bottom, 20)
        }
        .scrollIndicators(.hidden)
        .scrollDismissesKeyboard(.interactively)
    }

    // MARK: 머리

    private var header: some View {
        let items = model.settings.wish.map { model.content.skin($0) ?? .placeholder($0) }
        let total = items.reduce(0) { $0 + model.price($1).0 }
        let approx = items.contains { model.price($0).1 == .estimate }
        return VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 8) {
                Rectangle().fill(p.red).frame(width: 18, height: 3)
                Text(model.accounts.isEmpty ? "계정을 추가하면 확인해요" : "\(model.accounts.count)개 계정 모두 확인")
                    .font(VF.barlow(14)).tracking(1.8)
            }
            .foregroundStyle(p.redText)
            (Text("MY WISHLIST ").foregroundColor(p.text) + Text(String(format: "%02d", items.count)).foregroundColor(p.redText))
                .font(VF.anton(40))
            Text(items.isEmpty ? "하트를 누르거나 아래에서 찾아서 담아요" : "모두 합쳐 \(approx ? "약 " : "")\(Fmt.n(total)) VP")
                .font(VF.plex(13, .semibold))
                .foregroundStyle(p.muted)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(alignment: .topTrailing) {
            Watermark(text: "WISHLIST", size: 96).offset(x: 14, y: -22)
        }
    }

    // MARK: 검색

    private var searchBox: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 10) {
                Image(systemName: "magnifyingglass").foregroundStyle(p.muted)
                TextField("", text: $query, prompt: Text("스킨 이름으로 찾아서 담기 (예: 밴달)").foregroundColor(p.muted))
                    .font(VF.plex(14))
                    .foregroundStyle(p.text)
                    .focused($searching)
                    .submitLabel(.search)
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.never)
                if !query.isEmpty {
                    Button { query = ""; searching = false } label: {
                        Image(systemName: "xmark.circle.fill").foregroundStyle(p.muted)
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("검색어 지우기")
                }
            }
            .padding(.horizontal, 14)
            .frame(minHeight: 46)
            .background(p.panel)
            .overlay(alignment: .bottom) { Rectangle().fill(p.red).frame(height: 2) }

            ScrollView(.horizontal) {
                HStack(spacing: 6) {
                    ForEach(WeaponCat.filters, id: \.key) { f in
                        let on = cat == f.key
                        Button { cat = f.key } label: {
                            Text(f.label)
                                .font(VF.barlow(15)).tracking(1.2)
                                .foregroundStyle(on ? Palette.navy : p.text)
                                .padding(.horizontal, 12)
                                .frame(minHeight: 40)
                                .background(on ? p.text : Color.clear)
                                .overlay(Rectangle().stroke(on ? p.text : p.line, lineWidth: 1.5))
                        }
                        .buttonStyle(.plain)
                        .accessibilityAddTraits(on ? .isSelected : [])
                    }
                }
                .padding(1)
            }
            .scrollIndicators(.hidden)
        }
    }

    // MARK: 담은 스킨

    private var wishList: some View {
        let hits = model.allHits
        let items = model.settings.wish
            .map { model.content.skin($0) ?? .placeholder($0) }
            .filter { cat == "all" || $0.cat == cat }
        return VStack(spacing: 8) {
            if items.isEmpty {
                Text(model.settings.wish.isEmpty
                     ? "아직 담은 스킨이 없어요. 상점 카드의 하트를 누르거나, 위에서 이름으로 찾아 담아 보세요."
                     : "이 종류에는 담은 스킨이 없어요.")
                    .font(VF.plex(14))
                    .foregroundStyle(p.muted)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(16)
                    .background(p.panel, in: Notch(cut: 14))
            }
            ForEach(items) { s in
                WishRow(skin: s, hits: hits.filter { $0.skin.id == s.id })
            }
        }
    }

    private var results: some View {
        let found = model.content.search(query, cat: cat, limit: 60)
        return VStack(spacing: 8) {
            if !model.content.ready {
                Text(model.contentLoading ? "스킨 정보를 받는 중이에요…" : "스킨 정보를 아직 받지 못했어요. \(model.contentError)")
                    .font(VF.plex(13)).foregroundStyle(p.muted)
            } else if found.isEmpty {
                Text("‘\(query)’에 맞는 스킨이 없어요").font(VF.plex(14)).foregroundStyle(p.muted)
                    .frame(maxWidth: .infinity, alignment: .leading).padding(16).background(p.panel, in: Notch(cut: 14))
            }
            ForEach(found) { s in
                WishRow(skin: s, hits: [], searchMode: true)
            }
        }
    }
}

// MARK: - 한 줄

private struct WishRow: View {
    @EnvironmentObject private var model: AppModel
    @EnvironmentObject private var router: Router
    @Environment(\.palette) private var p
    let skin: Skin
    let hits: [Hit]
    var searchMode = false

    var body: some View {
        let hit = !hits.isEmpty
        let wished = model.isWished(skin.id)
        let pr = model.price(skin)
        let price = pr.0
        let state = pr.1
        let fg = hit ? p.onHit : p.text
        HStack(spacing: 10) {
            Button {
                router.preview = PreviewTarget(skin: skin, price: hits.first?.price, base: hits.first.flatMap { $0.base > 0 ? $0.base : nil },
                                               context: hits.first.map { "\($0.label) \($0.night ? "밤시장" : "오늘의 상점")" })
            } label: {
                HStack(spacing: 10) {
                    ZStack {
                        (hit ? Palette.navy : tierTint(skin.tier, p))
                        RemoteImage(url: skin.img).padding(6).foregroundStyle(p.text)
                    }
                    .frame(width: 76, height: 50)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(skin.name).font(VF.plex(14, .bold)).lineLimit(1)
                        HStack(spacing: 6) {
                            Diamond().fill(Tier.color(skin.tier)).frame(width: 8, height: 8)
                            Text("\(skin.weaponEn) · \(priceText(price, state))").font(VF.barlow(14, .semibold)).tracking(0.8)
                        }
                        Text(status)
                            .font(VF.plex(12, hit ? .bold : .medium))
                            .foregroundStyle(hit ? p.onHit : p.muted)
                            .lineLimit(2)
                    }
                    Spacer(minLength: 0)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel("\(skin.name), \(priceText(price, state)), \(status). 미리보기 열기")

            Button {
                model.setWish(skin.id, !wished)
            } label: {
                Image(systemName: wished ? "heart.fill" : "heart")
                    .font(.system(size: 18, weight: .semibold))
                    .foregroundStyle(hit ? p.onHit : (wished ? p.red : p.muted))
                    .frame(width: 44, height: 44)
            }
            .buttonStyle(.plain)
            .accessibilityLabel(wished ? "\(skin.name) 위시리스트에서 빼기" : "\(skin.name) 위시리스트에 담기")
        }
        .foregroundStyle(fg)
        .padding(.leading, 8).padding(.trailing, 4).padding(.vertical, 8)
        .background(hit ? p.hit : p.panel, in: Notch(cut: 12))
    }

    private var status: String {
        if searchMode { return model.isWished(skin.id) ? "담겨 있어요" : "하트를 누르면 담겨요" }
        guard let h = hits.first else { return "아직 안 떴어요" }
        let more = hits.count > 1 ? " 외 \(hits.count - 1)곳" : ""
        return "\(h.label) \(h.night ? "밤시장" : "상점")에 있음 · 오늘\(more)"
    }

    private func priceText(_ v: Int, _ s: PriceState) -> String {
        switch s {
        case .ok: return "\(Fmt.n(v)) VP"
        case .estimate: return "약 \(Fmt.n(v)) VP"
        case .none: return "상점 판매 없음"
        case .unknown: return "가격 모름"
        }
    }
}

// MARK: - 알림 설정

private struct AlertSettings: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.palette) private var p
    @State private var allowed = true

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            SectionTitle(en: "ALERTS", ko: "알림")
            toggle("위시리스트 스킨이 뜨면 알림", "오전 9시 상점이 바뀐 뒤 확인해요", isOn: Binding(
                get: { model.settings.wishAlert }, set: { model.settings.wishAlert = $0; model.saveSettings() }))
            toggle("밤시장에 뜬 것도 알림", nil, isOn: Binding(
                get: { model.settings.nightAlert }, set: { model.settings.nightAlert = $0; model.saveSettings() }))
            toggle("앱 사용 기간이 하루 남으면 알림", "맥에서 다시 설치할 때를 알려 드려요", isOn: Binding(
                get: { model.settings.passAlert }, set: { model.settings.passAlert = $0; model.saveSettings(); model.schedulePassAlert() }))
            if !allowed {
                Text("아이폰 알림이 꺼져 있어요. 아이폰 설정 › 알림 › ValoROAD 에서 ‘알림 허용’을 켜 주세요.")
                    .font(VF.plex(13, .semibold))
                    .foregroundStyle(p.redText)
            }
            Button {
                Task { await model.testNotification() }
            } label: {
                Label("시험 알림 보내기", systemImage: "bell.badge")
                    .font(VF.plex(13, .bold))
                    .foregroundStyle(p.text)
                    .padding(.horizontal, 12).frame(minHeight: 40)
                    .overlay(Rectangle().stroke(p.line, lineWidth: 1.5))
            }
            .buttonStyle(.plain)
            Text("아이폰은 앱이 뒤에서 확인하는 때를 스스로 정해요. 그래서 알림이 9시 정각보다 늦을 수 있어요. 앱을 열면 바로 확인해요.")
                .font(VF.plex(12))
                .foregroundStyle(p.muted)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.top, 12)
                .overlay(alignment: .top) { Rectangle().fill(p.line).frame(height: 1) }
        }
        .padding(16)
        .background(p.panel, in: Notch(cut: 18))
        .task { allowed = Demo.on ? true : await model.notificationsAllowed() }
    }

    private func toggle(_ title: String, _ sub: String?, isOn: Binding<Bool>) -> some View {
        Toggle(isOn: isOn) {
            VStack(alignment: .leading, spacing: 2) {
                Text(title).font(VF.plex(14, .bold)).foregroundStyle(p.text)
                if let sub { Text(sub).font(VF.plex(12)).foregroundStyle(p.muted) }
            }
        }
        .tint(p.red)
    }
}
