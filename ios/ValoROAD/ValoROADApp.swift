import SwiftUI
import UIKit
import UserNotifications

// ValoROAD 아이폰 — 발로란트 상점 확인 (계정 3개 · 자동 로그인 · 위시리스트 알림). 확인 전용, 구매 기능 없음.
// made by. Nevertheless_D

@main
struct ValoROADApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    @StateObject private var model = AppModel.shared
    @StateObject private var router = Router()
    @Environment(\.scenePhase) private var phase

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(model)
                .environmentObject(router)
                .preferredColorScheme(model.settings.theme == "dark" ? .dark : model.settings.theme == "light" ? .light : nil)
                .task { await model.start() }
        }
        .onChange(of: phase) { _, p in
            switch p {
            case .active: Task { await model.onForeground() }
            case .background: model.onBackground()
            default: break
            }
        }
        .backgroundTask(.appRefresh(AppModel.backgroundID)) {
            await AppModel.shared.backgroundCheck()
        }
    }
}

final class AppDelegate: NSObject, UIApplicationDelegate, UNUserNotificationCenterDelegate {
    func application(_ application: UIApplication, didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil) -> Bool {
        UNUserNotificationCenter.current().delegate = self
        return true
    }

    // 앱을 보고 있을 때도 알림을 위에 띄워요
    func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification) async -> UNNotificationPresentationOptions {
        [.banner, .sound, .list]
    }

    // 알림을 누르면 상점 화면으로
    func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse) async {
        await MainActor.run { Router.pendingTab = .store }
    }
}

// MARK: - 화면 이동

enum Tab: String {
    case store, wishlist, settings
}

struct PreviewTarget: Identifiable {
    let id = UUID()
    var skin: Skin
    var price: Int? = nil       // 상점에 있는 값 (없으면 가격표 · 보통 가격)
    var base: Int? = nil        // 밤시장 원래 가격
    var context: String? = nil  // "부캐#KR2 오늘의 상점"
    var ends: Int64? = nil
}

struct LoginTarget: Identifiable {
    var id: Int { slot }
    var slot: Int
}

@MainActor
final class Router: ObservableObject {
    static var pendingTab: Tab?
    @Published var tab: Tab = .store
    @Published var preview: PreviewTarget?
    @Published var login: LoginTarget?

    init() {
        if Demo.on, let t = Demo.arg("-tab").flatMap(Tab.init(rawValue:)) { tab = t }
    }

    func openLogin(slot: Int?) {
        let free = slot ?? AppModel.shared.freeSlot()
        guard let s = free else {
            AppModel.shared.say("계정 자리 3개가 모두 찼어요. 설정에서 한 자리를 비운 뒤 추가해 주세요", bad: true)
            return
        }
        login = LoginTarget(slot: s)
    }
}

// MARK: - 바탕 화면 (아래 탭 3개)

struct RootView: View {
    @EnvironmentObject private var model: AppModel
    @EnvironmentObject private var router: Router
    @Environment(\.scenePhase) private var phase

    var body: some View {
        PaletteProvider {
            RootBody()
        }
        .onChange(of: phase) { _, p in
            if p == .active, let t = Router.pendingTab {
                router.tab = t
                Router.pendingTab = nil
            }
        }
        .task {
            // 시험 화면 (맥 빌드 서버의 시뮬레이터)
            guard Demo.on else { return }
            try? await Task.sleep(nanoseconds: 6_000_000_000)
            if Demo.flag("-preview"), let acc = model.activeAccount, let st = model.stores[acc.id],
               let first = model.offers(st.daily).first {
                router.preview = PreviewTarget(skin: first.skin, price: first.price, context: "\(acc.label) 오늘의 상점", ends: st.dailyEnds)
            }
            if Demo.flag("-login") { router.openLogin(slot: 3) }
        }
    }
}

private struct RootBody: View {
    @EnvironmentObject private var model: AppModel
    @EnvironmentObject private var router: Router
    @Environment(\.palette) private var p

    var body: some View {
        VStack(spacing: 0) {
            ZStack {
                switch router.tab {
                case .store: StoreScreen()
                case .wishlist: WishlistScreen()
                case .settings: SettingsScreen()
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            TabBar()
        }
        .background(p.bg.ignoresSafeArea())
        .overlay(alignment: .top) { NoticeBanner() }
        .sheet(item: $router.preview) { t in
            PreviewSheet(target: t)
                .environmentObject(model)
                .environmentObject(router)
        }
        .fullScreenCover(item: $router.login) { t in
            LoginScreen(slot: t.slot)
                .environmentObject(model)
                .environmentObject(router)
        }
    }
}

private struct TabBar: View {
    @EnvironmentObject private var router: Router
    @EnvironmentObject private var model: AppModel
    @Environment(\.palette) private var p

    var body: some View {
        HStack(spacing: 0) {
            item(.store, "상점", icon: "bag", iconOn: "bag.fill")
            item(.wishlist, "위시리스트", icon: "heart", iconOn: "heart.fill", badge: model.allHits.count)
            item(.settings, "설정", icon: "slider.horizontal.3", iconOn: "slider.horizontal.3", badge: (model.pass.daysLeft ?? 9) <= 1 ? 1 : 0)
        }
        .padding(.horizontal, 8)
        .background(p.head.ignoresSafeArea(edges: .bottom))
        .overlay(alignment: .top) { Rectangle().fill(p.line).frame(height: 1) }
    }

    private func item(_ t: Tab, _ label: String, icon: String, iconOn: String, badge: Int = 0) -> some View {
        let on = router.tab == t
        return Button {
            router.tab = t
        } label: {
            VStack(spacing: 3) {
                ZStack(alignment: .topTrailing) {
                    Image(systemName: on ? iconOn : icon)
                        .font(.system(size: 20, weight: .regular))
                        .frame(width: 30, height: 24)
                    if badge > 0 {
                        Circle().fill(p.hit).frame(width: 9, height: 9).offset(x: 3, y: -1)
                    }
                }
                Text(label).font(VF.barlow(13)).tracking(1.2)
            }
            .foregroundStyle(on ? p.text : p.muted)
            .frame(maxWidth: .infinity, minHeight: 54)
            .overlay(alignment: .top) {
                if on { Rectangle().fill(p.red).frame(width: 40, height: 3) }
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityAddTraits(on ? .isSelected : [])
    }
}

/// 위쪽에 잠깐 나오는 안내 (4초 뒤 사라짐)
private struct NoticeBanner: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.palette) private var p

    var body: some View {
        if let n = model.notice {
            Text(n.text)
                .font(VF.plex(14, .semibold))
                .foregroundStyle(n.bad ? Color.white : p.onHit)
                .multilineTextAlignment(.leading)
                .padding(.horizontal, 14)
                .padding(.vertical, 12)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(n.bad ? p.red : p.hit, in: Notch(cut: 12))
                .padding(.horizontal, 16)
                .padding(.top, 6)
                .transition(.move(edge: .top).combined(with: .opacity))
                .onTapGesture { model.notice = nil }
                .task(id: n.id) {
                    try? await Task.sleep(nanoseconds: 4_500_000_000)
                    if model.notice?.id == n.id { withAnimation { model.notice = nil } }
                }
                .accessibilityAddTraits(.isStaticText)
        }
    }
}
