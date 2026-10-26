import SwiftUI
import WebKit

// 계정 추가 (시안 ④): 라이엇 공식 로그인 화면을 그대로 띄워요.
// 아이디 · 비밀번호 · 2단계 인증은 라이엇 화면에서 직접 하고, 로그인이 끝나 playvalorant.com/opt_in#access_token=… 으로
// 가는 순간 앱이 가로채요. 그 창의 riotgames.com 쿠키(로그인 열쇠)만 키체인에 보관해요. 비밀번호는 앱이 보지도 저장하지도 않아요.

struct LoginScreen: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    let slot: Int

    @State private var saving = false
    @State private var failure: String?
    @State private var loading = true
    @State private var reloadKey = 0

    private let p = Palette.dark

    var body: some View {
        VStack(spacing: 0) {
            VStack(alignment: .leading, spacing: 12) {
                HStack {
                    Button("취소") { dismiss() }
                        .font(VF.plex(15, .semibold))
                        .foregroundStyle(p.muted)
                        .frame(minHeight: 44)
                    Spacer()
                    Text("ADD ACCOUNT · 슬롯 \(String(format: "%02d", slot))").font(VF.barlow(15)).tracking(2)
                    Spacer()
                    Button { reloadKey += 1; failure = nil } label: {
                        Image(systemName: "arrow.clockwise").font(.system(size: 15, weight: .bold)).frame(width: 44, height: 44)
                    }
                    .accessibilityLabel("로그인 화면 다시 불러오기")
                }
                VStack(alignment: .leading, spacing: 4) {
                    Text("RIOT SIGN IN").font(VF.anton(34))
                    Text("라이엇 계정으로 로그인해 주세요 · 비밀번호는 앱이 보지 않아요")
                        .font(VF.plex(13))
                        .foregroundStyle(Color(hex: 0xC9D0D6))
                }
            }
            .foregroundStyle(p.text)
            .padding(.horizontal, 16)
            .padding(.top, 8)
            .padding(.bottom, 12)
            .background(p.bg)
            .overlay(alignment: .top) { Rectangle().fill(p.red).frame(height: 3).ignoresSafeArea(edges: .top) }

            ZStack {
                RiotLoginWeb(onDone: capture, onLoading: { loading = $0 }, onError: { failure = $0 })
                    .id(reloadKey)
                if loading && !saving {
                    ProgressView().tint(Palette.navy).scaleEffect(1.2)
                }
                if saving {
                    VStack(spacing: 14) {
                        ProgressView().tint(Palette.cream).scaleEffect(1.3)
                        Text("계정을 확인하고 저장하는 중이에요…").font(VF.plex(15, .semibold))
                    }
                    .foregroundStyle(Palette.cream)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .background(Palette.navy.opacity(0.92))
                }
            }

            VStack(alignment: .leading, spacing: 6) {
                bullet("로그인 열쇠는 이 아이폰의 키체인에만 보관돼요")
                bullet("다음부터는 자동으로 로그인돼요 · 와이파이든 데이터든 어디서나")
                Text("‘로그인 상태 유지’를 켜면 자동 로그인이 더 오래 이어져요")
                    .font(VF.plex(12)).foregroundStyle(p.muted)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 16).padding(.vertical, 10)
            .background(p.bg)
        }
        .background(p.bg.ignoresSafeArea())
        .alert("로그인하지 못했어요", isPresented: Binding(get: { failure != nil }, set: { if !$0 { failure = nil } })) {
            Button("다시 해 보기") { failure = nil; reloadKey += 1 }
            Button("닫기", role: .cancel) { dismiss() }
        } message: {
            Text(failure ?? "")
        }
        .interactiveDismissDisabled(saving)
    }

    private func bullet(_ s: String) -> some View {
        HStack(alignment: .top, spacing: 8) {
            Image(systemName: "checkmark").font(.system(size: 12, weight: .heavy)).foregroundStyle(p.hit)
            Text(s).font(VF.plex(12)).foregroundStyle(p.text)
        }
    }

    private func capture(_ result: LoginCapture) {
        guard !saving else { return }
        saving = true
        Task {
            do {
                let note = try await model.login(slot: slot, access: result.access, idToken: result.idToken, cookies: result.cookies)
                let name = model.activeAccount?.label ?? "계정"
                model.say(note.isEmpty ? "\(name) 계정을 저장했어요. 상점을 받는 중이에요" : note, bad: !note.isEmpty && note.contains("못했"))
                dismiss()
            } catch {
                saving = false
                Log.write("⚠️ 로그인 저장 실패: \(error.localizedDescription)")
                failure = (error as? RiotError)?.errorDescription ?? error.localizedDescription
            }
        }
    }
}

struct LoginCapture {
    var access: String
    var idToken: String
    var cookies: [RiotCookie]
}

/// 라이엇 공식 로그인 페이지 (계정마다 새 비영구 저장소 — 다른 계정의 로그인이 섞이지 않아요)
struct RiotLoginWeb: UIViewRepresentable {
    let onDone: (LoginCapture) -> Void
    let onLoading: (Bool) -> Void
    let onError: (String) -> Void

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    func makeUIView(context: Context) -> WKWebView {
        let cfg = WKWebViewConfiguration()
        cfg.websiteDataStore = .nonPersistent()
        let web = WKWebView(frame: .zero, configuration: cfg)
        web.customUserAgent = iPhoneSafariUA
        web.navigationDelegate = context.coordinator
        web.uiDelegate = context.coordinator
        web.allowsBackForwardNavigationGestures = true
        web.isOpaque = false
        web.backgroundColor = UIColor(red: 0.925, green: 0.91, blue: 0.882, alpha: 1)
        Log.write("로그인 화면 열기")
        if let u = URL(string: riotAuthorizeURL) { web.load(URLRequest(url: u)) }
        return web
    }

    func updateUIView(_ uiView: WKWebView, context: Context) {}

    final class Coordinator: NSObject, WKNavigationDelegate, WKUIDelegate {
        let parent: RiotLoginWeb
        var finished = false

        init(_ p: RiotLoginWeb) { parent = p }

        private func isDone(_ u: URL?) -> Bool {
            guard let u, let host = u.host else { return false }
            return host.hasSuffix("playvalorant.com") && u.path.hasPrefix("/opt_in")
        }

        private func capture(_ u: URL, _ web: WKWebView) {
            guard !finished else { return }
            guard let t = parseTokens(u.absoluteString) else {
                Log.write("⚠️ 로그인 완료 주소에 토큰이 없어요")
                parent.onError("로그인 정보를 받지 못했어요. 다시 시도해 주세요")
                return
            }
            finished = true
            web.stopLoading()
            web.configuration.websiteDataStore.httpCookieStore.getAllCookies { cookies in
                let riot = cookies.map(RiotCookie.init).filter(\.isRiot)
                Log.write("로그인 완료 → 저장 (쿠키 \(riot.count)개)")
                DispatchQueue.main.async {
                    self.parent.onDone(LoginCapture(access: t.access, idToken: t.id, cookies: riot))
                }
            }
        }

        func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction,
                     decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
            if let u = action.request.url, isDone(u) {
                decisionHandler(.cancel)
                capture(u, webView)
                return
            }
            decisionHandler(.allow)
        }

        func webView(_ webView: WKWebView, didReceiveServerRedirectForProvisionalNavigation navigation: WKNavigation!) {
            if let u = webView.url, isDone(u) {
                webView.stopLoading()
                capture(u, webView)
            }
        }

        func webView(_ webView: WKWebView, didStartProvisionalNavigation navigation: WKNavigation!) {
            parent.onLoading(true)
        }

        func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
            parent.onLoading(false)
        }

        func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
            parent.onLoading(false)
            let ns = error as NSError
            if let u = ns.userInfo[NSURLErrorFailingURLErrorKey] as? URL, isDone(u) {
                capture(u, webView)
            } else if ns.code != NSURLErrorCancelled && ns.code != 102 && !finished {
                Log.write("⚠️ 로그인 페이지를 불러오지 못함: \(error.localizedDescription)")
                parent.onError("라이엇 로그인 화면을 불러오지 못했어요. 인터넷 연결을 확인해 주세요")
            }
        }

        func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
            parent.onLoading(false)
        }

        // '구글로 로그인' 같은 새 창 요청은 같은 화면에서 열어요
        func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration,
                     for action: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
            if action.targetFrame == nil { webView.load(action.request) }
            return nil
        }
    }
}
