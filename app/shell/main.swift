// ValoROAD — Dock 에 보이는 맥 앱 껍데기
// made by. Nevertheless_D
//
// 왜 이 껍데기가 있나요?
//   macOS 27 의 '백그라운드 앱 활동' 규칙: 앱을 종료한 뒤에도 창·메뉴 막대·Dock 없이 남아 도는 프로그램은
//   사용자에게 보여 주고, 허용되지 않으면 시스템이 끕니다. 예외는 '그 프로그램을 켠 앱이 Dock 에 있을 때'예요.
//   그래서 ValoROAD 는 Dock 에 보이는 정식 앱이고, 엔진은 이 앱의 자식으로 '앱이 켜져 있는 동안만' 돌아요.
//   규칙을 피해 가는 장치(종료 신호 무시, 몰래 도는 도우미 앱)는 쓰지 않아요.
//
// 하는 일
//   1) 앱 안의 엔진을 ~/Library/Application Support/ValoROAD/bin 으로 복사 (업데이트로 앱 묶음이 바뀌어도 안전하게)
//   2) 엔진(지킴이 모드)을 자식으로 켜고, 127.0.0.1:47931 + 토큰 주소를 앱 창(WKWebView)에 띄움
//   3) Dock 아이콘을 누르면 창을 다시 보여 주고, 앱을 종료하면 엔진도 깔끔하게 끔
//   4) 엔진이 끝나면 앱도 끝남 (화면의 '종료' 버튼) · 업데이트 후에는 새 버전을 다시 엶
//   5) 라이엇 로그인 창: 라이엇 공식 페이지를 그대로 띄우고, 로그인이 끝나는 순간의 토큰 · 쿠키를 엔진에 넘김 (비밀번호는 모름)
//   6) 맥 알림: 엔진이 만든 위시리스트 알림을 15초마다 가져와 띄움
//   7) `ValoROAD --riot-fetch`: 엔진이 riotgames.com 에 보내는 요청을 맥의 URLSession 으로 대신 보내 줌 (창 없이 끝남)
//   8) 모든 단계를 launcher.log 에 기록

import AppKit
import Darwin
import UserNotifications
import WebKit

let riotAuthorizeURL = "https://auth.riotgames.com/authorize?redirect_uri=https%3A%2F%2Fplayvalorant.com%2Fopt_in&client_id=play-valorant-web-prod&response_type=token%20id_token&nonce=1&scope=account%20openid"
let safariUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15"

// MARK: - 경로 · 기록

enum Paths {
    static let support: URL = {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        return base.appendingPathComponent("ValoROAD", isDirectory: true)
    }()
    static let bin = support.appendingPathComponent("bin", isDirectory: true)
    static let engine = bin.appendingPathComponent("valoroad-engine")
    static func file(_ name: String) -> URL { support.appendingPathComponent(name) }
}

enum Log {
    static func write(_ message: String) {
        let fm = FileManager.default
        try? fm.createDirectory(at: Paths.support, withIntermediateDirectories: true)
        let url = Paths.file("launcher.log")
        if let attrs = try? fm.attributesOfItem(atPath: url.path), let size = attrs[.size] as? NSNumber, size.intValue > 262_144 {
            let prev = Paths.file("launcher.prev.log")
            try? fm.removeItem(at: prev)
            try? fm.moveItem(at: url, to: prev)
        }
        let df = DateFormatter()
        df.locale = Locale(identifier: "en_US_POSIX")
        df.dateFormat = "MM-dd HH:mm:ss"
        let line = "\(df.string(from: Date())) [앱 \(getpid())] \(message)\n"
        guard let data = line.data(using: .utf8) else { return }
        if let h = try? FileHandle(forWritingTo: url) {
            h.seekToEndOfFile()
            h.write(data)
            h.closeFile()
        } else {
            try? data.write(to: url)
        }
    }
}

func cookieDict(_ c: HTTPCookie) -> [String: Any] {
    var d: [String: Any] = ["name": c.name, "value": c.value, "domain": c.domain, "path": c.path,
                            "secure": c.isSecure, "httpOnly": c.isHTTPOnly]
    if let e = c.expiresDate { d["expires"] = Int64(e.timeIntervalSince1970) }
    return d
}

// MARK: - 엔진

final class Engine {
    var process: Process?
    var port: Int = 0
    var token: String = ""
    let build: String
    var onExit: ((Int32, Bool) -> Void)?

    init() {
        let v = Bundle.main.url(forResource: "version", withExtension: "txt")
            .flatMap { try? String(contentsOf: $0, encoding: .utf8) } ?? "0.0.0"
        build = v.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func readText(_ name: String) -> String? {
        guard let s = try? String(contentsOf: Paths.file(name), encoding: .utf8) else { return nil }
        let t = s.trimmingCharacters(in: .whitespacesAndNewlines)
        return t.isEmpty ? nil : t
    }

    /// 앱 안의 엔진을 앱 전용 폴더로 복사해요 (버전이 바뀌었을 때만)
    func prepare() throws {
        let fm = FileManager.default
        try fm.createDirectory(at: Paths.bin, withIntermediateDirectories: true)
        let buildFile = Paths.bin.appendingPathComponent("build")
        let installed = (try? String(contentsOf: buildFile, encoding: .utf8))?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let want = build + "-native"
        if installed != want || !fm.isExecutableFile(atPath: Paths.engine.path) {
            guard let src = Bundle.main.url(forResource: "valoroad-engine", withExtension: nil) else {
                throw NSError(domain: "ValoROAD", code: 1, userInfo: [NSLocalizedDescriptionKey: "앱 안에서 엔진을 찾지 못했어요"])
            }
            let tmp = Paths.bin.appendingPathComponent("valoroad-engine.new")
            try? fm.removeItem(at: tmp)
            try fm.copyItem(at: src, to: tmp)
            chmod(tmp.path, 0o755)
            removexattr(tmp.path, "com.apple.quarantine", 0)
            if rename(tmp.path, Paths.engine.path) != 0 {
                throw NSError(domain: "ValoROAD", code: 2, userInfo: [NSLocalizedDescriptionKey: "엔진을 준비하지 못했어요"])
            }
            try want.write(to: buildFile, atomically: true, encoding: .utf8)
            Log.write("엔진 준비 (v\(build))")
        }
        removexattr(Paths.engine.path, "com.apple.quarantine", 0)
    }

    /// 127.0.0.1 엔진에 짧게 요청해요 (상태 코드와 내용을 돌려줘요)
    func call(_ method: String, port: Int, path: String, token: String, body: Data?, timeout: TimeInterval) -> (Int, Data?) {
        guard let url = URL(string: "http://127.0.0.1:\(port)\(path)") else { return (0, nil) }
        var req = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: timeout)
        req.httpMethod = method
        req.setValue(token, forHTTPHeaderField: "x-token")
        if method == "POST" {
            req.httpBody = body ?? "{}".data(using: .utf8)
            req.setValue("application/json", forHTTPHeaderField: "content-type")
        }
        let sem = DispatchSemaphore(value: 0)
        var status = 0
        var out: Data?
        let task = URLSession.shared.dataTask(with: req) { data, resp, _ in
            if let h = resp as? HTTPURLResponse { status = h.statusCode }
            out = data
            sem.signal()
        }
        task.resume()
        if sem.wait(timeout: .now() + timeout + 0.5) == .timedOut { task.cancel() }
        return (status, out)
    }

    func request(_ method: String, port: Int, path: String, token: String, timeout: TimeInterval) -> Data? {
        let (status, data) = call(method, port: port, path: path, token: token, body: nil, timeout: timeout)
        return status == 200 ? (data ?? Data()) : nil
    }

    /// 이미 켜져 있는 엔진이 있으면 끄고 새로 켜요
    func stopExisting() {
        guard let p = readText("port").flatMap({ Int($0) }), let t = readText("token") else { return }
        guard request("GET", port: p, path: "/api/ping", token: t, timeout: 1) != nil else { return }
        Log.write("이미 켜져 있던 엔진(포트 \(p))을 끄고 새로 켜요")
        _ = request("POST", port: p, path: "/api/quit", token: t, timeout: 2)
        for _ in 0..<40 {
            if request("GET", port: p, path: "/api/ping", token: t, timeout: 0.5) == nil { break }
            usleep(150_000)
        }
    }

    func start() throws {
        let fm = FileManager.default
        token = readText("token") ?? Engine.randomToken()
        try? fm.removeItem(at: Paths.file("port"))
        try? fm.removeItem(at: Paths.file("restart-requested"))

        let p = Process()
        p.executableURL = Paths.engine
        var env = ProcessInfo.processInfo.environment
        for k in ["VALOROAD_RESTART", "VALOROAD_PORT", "VALOROAD_CHILD", "VALOROAD_RELAUNCHED"] { env.removeValue(forKey: k) }
        env["VALOROAD_SUPERVISOR"] = "1"
        env["VALOROAD_SHELL"] = "native"
        env["VALOROAD_VIA"] = "native"
        env["VALOROAD_TOKEN"] = token
        env["VALOROAD_SUPPORT"] = Paths.support.path
        env["VALOROAD_APP"] = Bundle.main.bundlePath
        env["VALOROAD_PARENT_PID"] = String(getpid())
        env["VALOROAD_PORT_WAIT"] = "1"
        p.environment = env
        p.standardInput = FileHandle.nullDevice
        p.standardOutput = FileHandle.nullDevice
        p.standardError = FileHandle.nullDevice
        p.terminationHandler = { [weak self] proc in
            let code = proc.terminationStatus
            DispatchQueue.main.async { self?.exited(code) }
        }
        try p.run()
        process = p
        Log.write("엔진 켬 (지킴이 pid \(p.processIdentifier))")
    }

    func waitForPort() -> Int? {
        for _ in 0..<100 {
            if let v = readText("port").flatMap({ Int($0) }) {
                port = v
                return v
            }
            usleep(150_000)
        }
        return nil
    }

    private func exited(_ code: Int32) {
        let restart = FileManager.default.fileExists(atPath: Paths.file("restart-requested").path)
        process = nil
        onExit?(code, restart)
    }

    /// 앱을 종료할 때: 엔진에게 정중히 끄라고 하고, 안 꺼지면 종료 신호를 보내요
    func stop(completion: @escaping () -> Void) {
        guard let p = process, p.isRunning else { completion(); return }
        let port = self.port, token = self.token
        DispatchQueue.global(qos: .userInitiated).async {
            _ = self.request("POST", port: port, path: "/api/quit", token: token, timeout: 2)
            for _ in 0..<30 where p.isRunning { usleep(100_000) }
            if p.isRunning {
                p.terminate()
                for _ in 0..<20 where p.isRunning { usleep(100_000) }
            }
            if p.isRunning { kill(p.processIdentifier, SIGKILL) }
            DispatchQueue.main.async { completion() }
        }
    }

    static func randomToken() -> String {
        let chars = Array("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
        return String((0..<32).map { _ in chars[Int(arc4random_uniform(UInt32(chars.count)))] })
    }
}

// MARK: - 라이엇 연결 도우미 (ValoROAD --riot-fetch)

/// 엔진이 표준 입력으로 {method, url, headers, body(base64), noRedirect} 를 주면,
/// 맥의 URLSession 으로 보내고 {status, location, cookies, body(base64), error} 를 표준 출력으로 돌려줘요.
enum RiotFetch {
    final class Redirects: NSObject, URLSessionTaskDelegate {
        let block: Bool
        init(block: Bool) { self.block = block }
        func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                        newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) {
            completionHandler(block ? nil : request)
        }
    }

    static func emit(_ out: [String: Any]) {
        if let d = try? JSONSerialization.data(withJSONObject: out) {
            FileHandle.standardOutput.write(d)
        }
    }

    static func run() -> Int32 {
        let input = FileHandle.standardInput.readDataToEndOfFile()
        guard let obj = (try? JSONSerialization.jsonObject(with: input)) as? [String: Any],
              let urlStr = obj["url"] as? String, let url = URL(string: urlStr),
              let host = url.host, host.hasSuffix("riotgames.com") || host.hasSuffix("pvp.net") else {
            emit(["status": 0, "error": "잘못된 요청"])
            return 2
        }
        var req = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: 25)
        req.httpMethod = (obj["method"] as? String) ?? "GET"
        req.httpShouldHandleCookies = false
        if let h = obj["headers"] as? [String: String] {
            for (k, v) in h { req.setValue(v, forHTTPHeaderField: k) }
        }
        if let b = obj["body"] as? String, let d = Data(base64Encoded: b) { req.httpBody = d }
        let cfg = URLSessionConfiguration.ephemeral
        cfg.httpCookieAcceptPolicy = .never
        cfg.httpShouldSetCookies = false
        cfg.httpCookieStorage = nil
        cfg.urlCache = nil
        let delegate = Redirects(block: (obj["noRedirect"] as? Bool) ?? false)
        let session = URLSession(configuration: cfg, delegate: delegate, delegateQueue: nil)
        let sem = DispatchSemaphore(value: 0)
        var out: [String: Any] = ["status": 0, "location": "", "cookies": [[String: Any]](), "body": ""]
        session.dataTask(with: req) { data, resp, err in
            if let err = err { out["error"] = err.localizedDescription }
            if let h = resp as? HTTPURLResponse {
                out["status"] = h.statusCode
                out["location"] = h.value(forHTTPHeaderField: "Location") ?? ""
                var fields: [String: String] = [:]
                for (k, v) in h.allHeaderFields {
                    if let ks = k as? String, let vs = v as? String { fields[ks] = vs }
                }
                out["cookies"] = HTTPCookie.cookies(withResponseHeaderFields: fields, for: url).map(cookieDict)
            }
            out["body"] = (data ?? Data()).base64EncodedString()
            sem.signal()
        }.resume()
        if sem.wait(timeout: .now() + 30) == .timedOut { out["error"] = "응답 없음" }
        emit(out)
        return 0
    }
}

// MARK: - 라이엇 로그인 창

/// 라이엇 공식 로그인 페이지를 그대로 띄워요. 계정마다 새 비영구 저장소라 계정끼리 섞이지 않아요.
/// 로그인이 끝나 playvalorant.com/opt_in#access_token=… 으로 가려는 순간 가로채서, 토큰과 riotgames.com 쿠키를 엔진에 넘겨요.
final class LoginController: NSObject, WKNavigationDelegate, WKUIDelegate, NSWindowDelegate {
    let slot: Int
    let engine: Engine
    var reply: (([String: Any]) -> Void)?
    var window: NSWindow!
    var web: WKWebView!
    var finished = false
    var onClose: (() -> Void)?

    init(slot: Int, engine: Engine, reply: @escaping ([String: Any]) -> Void) {
        self.slot = slot
        self.engine = engine
        self.reply = reply
    }

    func start(over parent: NSWindow?) {
        let cfg = WKWebViewConfiguration()
        cfg.websiteDataStore = .nonPersistent()
        web = WKWebView(frame: NSRect(x: 0, y: 0, width: 520, height: 780), configuration: cfg)
        web.customUserAgent = safariUA
        web.navigationDelegate = self
        web.uiDelegate = self
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 520, height: 780),
                          styleMask: [.titled, .closable, .resizable], backing: .buffered, defer: false)
        window.title = String(format: "라이엇 로그인 · 슬롯 %02d", slot)
        window.isReleasedWhenClosed = false
        window.minSize = NSSize(width: 420, height: 560)
        window.contentView = web
        window.delegate = self
        if let p = parent {
            let f = p.frame
            window.setFrameOrigin(NSPoint(x: f.midX - 260, y: f.midY - 390))
        } else {
            window.center()
        }
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        Log.write("로그인 창 열기 (슬롯 \(slot))")
        if let u = URL(string: riotAuthorizeURL) { web.load(URLRequest(url: u)) }
    }

    func cancel() {
        window?.close()
    }

    private func isDone(_ u: URL?) -> Bool {
        guard let u = u, let host = u.host else { return false }
        return host.hasSuffix("playvalorant.com") && u.path.hasPrefix("/opt_in")
    }

    private func finish(_ result: [String: Any]) {
        guard !finished else { return }
        finished = true
        reply?(result)
        reply = nil
        window?.orderOut(nil)
        window?.close()
        onClose?()
    }

    private func capture(_ u: URL) {
        guard !finished else { return }
        var q: [String: String] = [:]
        for part in (u.fragment ?? u.query ?? "").split(separator: "&") {
            let kv = part.split(separator: "=", maxSplits: 1).map(String.init)
            if kv.count == 2 { q[kv[0]] = kv[1].removingPercentEncoding ?? kv[1] }
        }
        guard let access = q["access_token"], !access.isEmpty else {
            Log.write("⚠️ 로그인 완료 주소에 토큰이 없어요")
            finish(["error": "로그인 정보를 받지 못했어요. 다시 시도해 주세요"])
            return
        }
        let idToken = q["id_token"] ?? ""
        web.stopLoading()
        web.configuration.websiteDataStore.httpCookieStore.getAllCookies { cookies in
            let riot = cookies.filter { $0.domain.hasSuffix("riotgames.com") }.map(cookieDict)
            Log.write("로그인 완료 → 엔진에 넘김 (쿠키 \(riot.count)개)")
            let body: [String: Any] = ["slot": self.slot, "accessToken": access, "idToken": idToken, "cookies": riot]
            let data = try? JSONSerialization.data(withJSONObject: body)
            let port = self.engine.port, token = self.engine.token
            DispatchQueue.global(qos: .userInitiated).async {
                let (status, out) = self.engine.call("POST", port: port, path: "/api/accounts/login", token: token, body: data, timeout: 40)
                let obj = out.flatMap { (try? JSONSerialization.jsonObject(with: $0)) as? [String: Any] } ?? [:]
                DispatchQueue.main.async {
                    if status == 200, obj["ok"] as? Bool == true {
                        self.finish(obj)
                    } else {
                        let msg = (obj["error"] as? String) ?? "계정을 저장하지 못했어요 (\(status))"
                        Log.write("⚠️ 로그인 저장 실패: \(msg)")
                        self.finish(["error": msg])
                    }
                }
            }
        }
    }

    func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction,
                 decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        if let u = action.request.url, isDone(u) {
            decisionHandler(.cancel)
            capture(u)
            return
        }
        decisionHandler(.allow)
    }

    func webView(_ webView: WKWebView, didReceiveServerRedirectForProvisionalNavigation navigation: WKNavigation!) {
        if let u = webView.url, isDone(u) {
            webView.stopLoading()
            capture(u)
        }
    }

    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
        let ns = error as NSError
        if let u = ns.userInfo[NSURLErrorFailingURLErrorKey] as? URL, isDone(u) {
            capture(u)
        } else if ns.code != NSURLErrorCancelled && ns.code != 102 {
            Log.write("⚠️ 로그인 페이지를 불러오지 못함: \(error.localizedDescription)")
        }
    }

    // '구글로 로그인' 같은 새 창 요청은 같은 창에서 열어요
    func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration,
                 for action: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
        if action.targetFrame == nil { webView.load(action.request) }
        return nil
    }

    func windowWillClose(_ notification: Notification) {
        if !finished {
            finished = true
            Log.write("로그인 창을 닫음 (취소)")
            reply?(["canceled": true])
            reply = nil
            onClose?()
        }
    }
}

// MARK: - 화면과 이야기하는 다리

/// 화면에서 오는 부탁: window.webkit.messageHandlers.valoroad.postMessage({type: 'login' | 'loginCancel', …})
final class Bridge: NSObject, WKScriptMessageHandlerWithReply {
    weak var app: AppDelegate?
    func userContentController(_ ucc: WKUserContentController, didReceive message: WKScriptMessage,
                               replyHandler: @escaping (Any?, String?) -> Void) {
        guard message.frameInfo.securityOrigin.host == "127.0.0.1",
              let body = message.body as? [String: Any], let type = body["type"] as? String, let app = app else {
            replyHandler(nil, "허용되지 않은 요청이에요")
            return
        }
        switch type {
        case "login":
            let slot = (body["slot"] as? Int) ?? 0
            app.openLogin(slot: slot) { result in replyHandler(result, nil) }
        case "loginCancel":
            app.login?.cancel()
            replyHandler(["ok": true], nil)
        default:
            replyHandler(nil, "알 수 없는 요청이에요")
        }
    }
}

// MARK: - 앱

final class AppDelegate: NSObject, NSApplicationDelegate, WKNavigationDelegate, WKUIDelegate, NSWindowDelegate, UNUserNotificationCenterDelegate {
    var window: NSWindow!
    var web: WKWebView!
    let engine = Engine()
    var quitting = false
    var pageURL: URL?
    var login: LoginController?
    var noteTimer: Timer?

    func applicationDidFinishLaunching(_ notification: Notification) {
        let env = ProcessInfo.processInfo.environment
        // 앱 파일을 직접 실행해 업데이트된 경우: macOS 정식 방법으로 다시 열고 이 실행은 끝내요
        if env["VALOROAD_RESTART"] == "1" && env["VALOROAD_RELAUNCHED"] != "1" {
            Log.write("업데이트 뒤 직접 실행됨 → 정식 방법으로 다시 열어요")
            relaunch()
            return
        }
        Log.write("시작 v\(engine.build) · macOS \(ProcessInfo.processInfo.operatingSystemVersionString) · \(Bundle.main.bundlePath)")
        buildMenu()
        makeWindow()
        showLoading()
        setupNotifications()
        engine.onExit = { [weak self] code, restart in self?.engineExited(code: code, restart: restart) }
        boot()
    }

    func boot() {
        DispatchQueue.global(qos: .userInitiated).async {
            do {
                try self.engine.prepare()
                self.engine.stopExisting()
                try self.engine.start()
                guard let port = self.engine.waitForPort() else {
                    throw NSError(domain: "ValoROAD", code: 3, userInfo: [NSLocalizedDescriptionKey: "엔진이 준비되지 않았어요"])
                }
                Log.write("엔진 준비됨 (포트 \(port))")
                DispatchQueue.main.async { self.loadApp(port: port) }
            } catch {
                Log.write("⚠️ 시작 실패: \(error.localizedDescription)")
                DispatchQueue.main.async { self.startFailed(error) }
            }
        }
    }

    func loadApp(port: Int) {
        guard let url = URL(string: "http://127.0.0.1:\(port)/?t=\(engine.token)") else { return }
        pageURL = url
        web.load(URLRequest(url: url))
        startNotePolling()
    }

    func startFailed(_ error: Error) {
        let a = NSAlert()
        a.alertStyle = .warning
        a.messageText = "ValoROAD를 시작하지 못했어요"
        a.informativeText = "\(error.localizedDescription)\n앱을 다시 실행해 보세요. 계속 안 되면 맥을 재시동한 뒤 시도해 주세요."
        a.addButton(withTitle: "다시 시도")
        a.addButton(withTitle: "종료")
        if a.runModal() == .alertFirstButtonReturn {
            boot()
        } else {
            quitting = true
            NSApp.terminate(nil)
        }
    }

    func engineExited(code: Int32, restart: Bool) {
        if quitting { return }
        if restart {
            relaunch()
            return
        }
        Log.write("엔진이 끝나서 앱도 닫아요 (코드 \(code))")
        quitting = true
        NSApp.terminate(nil)
    }

    /// 업데이트된 새 버전을 macOS 정식 방법(LaunchServices)으로 다시 열고, 지금 앱은 끝내요
    func relaunch() {
        Log.write("새 버전으로 다시 열어요")
        quitting = true
        let appURL = URL(fileURLWithPath: Bundle.main.bundlePath)
        let cfg = NSWorkspace.OpenConfiguration()
        cfg.createsNewApplicationInstance = true
        cfg.activates = true
        // 새로 여는 앱은 지금 앱의 환경 변수를 물려받아요. '다시 열어 줘' 표시를 빼고, 다시 연 앱이라는 표시를 붙여요.
        var env = ProcessInfo.processInfo.environment
        env.removeValue(forKey: "VALOROAD_RESTART")
        env["VALOROAD_RELAUNCHED"] = "1"
        cfg.environment = env
        unsetenv("VALOROAD_RESTART")
        NSWorkspace.shared.openApplication(at: appURL, configuration: cfg) { _, error in
            if let error = error { Log.write("⚠️ 다시 열지 못했어요: \(error.localizedDescription)") }
            DispatchQueue.main.async { NSApp.terminate(nil) }
        }
    }

    // Dock 아이콘을 누르면 창을 다시 보여 줘요
    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        if !flag { showMain() }
        return true
    }

    // 창을 닫아도 앱은 Dock 에 남아요 (Dock 아이콘을 누르면 다시 열림)
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        noteTimer?.invalidate()
        login?.cancel()
        guard engine.process != nil else { return .terminateNow }
        quitting = true
        Log.write("종료: 엔진을 끄는 중")
        engine.stop {
            Log.write("종료 완료")
            NSApp.reply(toApplicationShouldTerminate: true)
        }
        return .terminateLater
    }

    // MARK: 로그인

    func openLogin(slot: Int, reply: @escaping ([String: Any]) -> Void) {
        if let old = login {
            old.cancel()
            login = nil
        }
        guard engine.port > 0 else {
            reply(["error": "앱이 아직 준비 중이에요. 잠시 뒤 다시 시도해 주세요"])
            return
        }
        let c = LoginController(slot: slot, engine: engine, reply: reply)
        c.onClose = { [weak self, weak c] in
            if let s = self, s.login === c { s.login = nil }
            self?.showMain()
        }
        login = c
        c.start(over: window)
    }

    // MARK: 알림

    func setupNotifications() {
        let center = UNUserNotificationCenter.current()
        center.delegate = self
        center.requestAuthorization(options: [.alert, .sound, .badge]) { granted, error in
            Log.write("알림 허용: \(granted)\(error.map { " (\($0.localizedDescription))" } ?? "")")
        }
    }

    func startNotePolling() {
        noteTimer?.invalidate()
        noteTimer = Timer.scheduledTimer(withTimeInterval: 15, repeats: true) { [weak self] _ in self?.pollNotes() }
        DispatchQueue.main.asyncAfter(deadline: .now() + 5) { [weak self] in self?.pollNotes() }
    }

    func pollNotes() {
        let port = engine.port, token = engine.token
        guard port > 0 else { return }
        DispatchQueue.global(qos: .utility).async {
            guard let data = self.engine.request("GET", port: port, path: "/api/notify/pending", token: token, timeout: 3),
                  let obj = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any],
                  let notes = obj["notes"] as? [[String: Any]], !notes.isEmpty else { return }
            for n in notes {
                let content = UNMutableNotificationContent()
                content.title = (n["title"] as? String) ?? "ValoROAD"
                content.body = (n["body"] as? String) ?? ""
                if (n["sound"] as? Bool) == true { content.sound = .default }
                let id = (n["id"] as? String) ?? UUID().uuidString
                UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: id, content: content, trigger: nil)) { err in
                    Log.write(err == nil ? "알림 보냄: \(content.title)" : "⚠️ 알림 실패: \(err!.localizedDescription)")
                }
            }
        }
    }

    // 앱이 앞에 있어도 알림을 보여 줘요
    func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification,
                                withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void) {
        completionHandler([.banner, .sound, .list])
    }

    // 알림을 누르면 앱 창을 보여 줘요
    func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse,
                                withCompletionHandler completionHandler: @escaping () -> Void) {
        DispatchQueue.main.async { self.showMain() }
        completionHandler()
    }

    // MARK: 창

    func makeWindow() {
        let cfg = WKWebViewConfiguration()
        cfg.applicationNameForUserAgent = "ValoROADShell/\(engine.build)"
        // 미리보기 영상(레벨 효과 · 사운드)이 누르자마자 소리와 함께 나오게 해요
        cfg.mediaTypesRequiringUserActionForPlayback = []
        if #available(macOS 12.3, *) { cfg.preferences.isElementFullscreenEnabled = true }
        let bridge = Bridge()
        bridge.app = self
        cfg.userContentController.addScriptMessageHandler(bridge, contentWorld: .page, name: "valoroad")
        web = WKWebView(frame: NSRect(x: 0, y: 0, width: 1380, height: 900), configuration: cfg)
        web.navigationDelegate = self
        web.uiDelegate = self
        web.allowsMagnification = false
        if #available(macOS 13.3, *) { web.isInspectable = true }

        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1380, height: 900),
                          styleMask: [.titled, .closable, .miniaturizable, .resizable],
                          backing: .buffered, defer: false)
        window.title = "ValoROAD"
        window.minSize = NSSize(width: 1000, height: 660)
        window.isReleasedWhenClosed = false
        window.tabbingMode = .disallowed
        window.backgroundColor = NSColor(red: 0x0F / 255.0, green: 0x19 / 255.0, blue: 0x23 / 255.0, alpha: 1)
        window.collectionBehavior.insert(.fullScreenPrimary)
        window.delegate = self
        window.contentView = web
        if !window.setFrameUsingName("ValoROADMain") { window.center() }
        window.setFrameAutosaveName("ValoROADMain")
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    func showLoading() {
        let html = """
        <!doctype html><meta charset="utf-8"><style>
        body{margin:0;height:100vh;display:grid;place-items:center;font:600 14px -apple-system,"Apple SD Gothic Neo",sans-serif;
             background:#0F1923;color:#9AA6B1}
        .w{display:flex;flex-direction:column;align-items:center;gap:16px;letter-spacing:.04em}
        .s{width:34px;height:34px;border-radius:50%;border:3px solid rgba(255,70,85,.25);border-top-color:#FF4655;animation:r .9s linear infinite}
        @keyframes r{to{transform:rotate(360deg)}}
        </style><div class="w"><div class="s"></div>ValoROAD 준비 중…</div>
        """
        web.loadHTMLString(html, baseURL: nil)
    }

    @objc func showMain() {
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    @objc func openInBrowser() {
        if let u = pageURL { NSWorkspace.shared.open(u) }
    }

    @objc func openDiag() {
        if engine.port > 0, let u = URL(string: "http://127.0.0.1:\(engine.port)/diag") { NSWorkspace.shared.open(u) }
    }

    @objc func reloadPage() {
        if let u = pageURL { web.load(URLRequest(url: u)) }
    }

    // MARK: WKWebView

    func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction,
                 decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        guard let u = action.request.url else { decisionHandler(.allow); return }
        let scheme = u.scheme ?? ""
        if scheme == "about" || scheme == "data" || scheme == "blob" || u.host == "127.0.0.1" || u.host == "localhost" {
            decisionHandler(.allow)
            return
        }
        // 바깥 주소는 기본 브라우저로 열어요
        NSWorkspace.shared.open(u)
        decisionHandler(.cancel)
    }

    func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration,
                 for action: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
        if let u = action.request.url { NSWorkspace.shared.open(u) }
        return nil
    }

    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
        guard pageURL != nil, !quitting else { return }
        Log.write("⚠️ 화면을 불러오지 못함: \(error.localizedDescription) → 다시 시도")
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.0) { [weak self] in self?.reloadPage() }
    }

    func webViewWebContentProcessDidTerminate(_ webView: WKWebView) {
        Log.write("⚠️ 화면 프로세스가 멈춤 → 다시 불러와요")
        reloadPage()
    }

    func webViewDidClose(_ webView: WKWebView) {
        window.orderOut(nil)
    }

    func webView(_ webView: WKWebView, runJavaScriptAlertPanelWithMessage message: String,
                 initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping () -> Void) {
        let a = NSAlert()
        a.messageText = message
        a.addButton(withTitle: "확인")
        a.beginSheetModal(for: window) { _ in completionHandler() }
    }

    func webView(_ webView: WKWebView, runJavaScriptConfirmPanelWithMessage message: String,
                 initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (Bool) -> Void) {
        let a = NSAlert()
        let parts = message.components(separatedBy: "\n")
        a.messageText = parts.first ?? message
        if parts.count > 1 { a.informativeText = parts.dropFirst().joined(separator: "\n") }
        a.addButton(withTitle: "확인")
        a.addButton(withTitle: "취소")
        a.beginSheetModal(for: window) { r in completionHandler(r == .alertFirstButtonReturn) }
    }

    // MARK: 메뉴

    func buildMenu() {
        let main = NSMenu()

        let appItem = NSMenuItem()
        main.addItem(appItem)
        let appMenu = NSMenu()
        appMenu.addItem(withTitle: "ValoROAD에 관하여", action: #selector(NSApplication.orderFrontStandardAboutPanel(_:)), keyEquivalent: "")
        appMenu.addItem(NSMenuItem.separator())
        appMenu.addItem(withTitle: "ValoROAD 가리기", action: #selector(NSApplication.hide(_:)), keyEquivalent: "h")
        let others = appMenu.addItem(withTitle: "기타 가리기", action: #selector(NSApplication.hideOtherApplications(_:)), keyEquivalent: "h")
        others.keyEquivalentModifierMask = [.command, .option]
        appMenu.addItem(withTitle: "모두 보기", action: #selector(NSApplication.unhideAllApplications(_:)), keyEquivalent: "")
        appMenu.addItem(NSMenuItem.separator())
        appMenu.addItem(withTitle: "ValoROAD 종료", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        appItem.submenu = appMenu

        let editItem = NSMenuItem()
        main.addItem(editItem)
        let edit = NSMenu(title: "편집")
        edit.addItem(withTitle: "실행 취소", action: Selector(("undo:")), keyEquivalent: "z")
        let redo = edit.addItem(withTitle: "실행 복귀", action: Selector(("redo:")), keyEquivalent: "z")
        redo.keyEquivalentModifierMask = [.command, .shift]
        edit.addItem(NSMenuItem.separator())
        edit.addItem(withTitle: "오려두기", action: #selector(NSText.cut(_:)), keyEquivalent: "x")
        edit.addItem(withTitle: "복사하기", action: #selector(NSText.copy(_:)), keyEquivalent: "c")
        edit.addItem(withTitle: "붙여넣기", action: #selector(NSText.paste(_:)), keyEquivalent: "v")
        edit.addItem(withTitle: "모두 선택", action: #selector(NSText.selectAll(_:)), keyEquivalent: "a")
        editItem.submenu = edit

        let viewItem = NSMenuItem()
        main.addItem(viewItem)
        let view = NSMenu(title: "보기")
        let fs = view.addItem(withTitle: "전체 화면", action: #selector(NSWindow.toggleFullScreen(_:)), keyEquivalent: "f")
        fs.keyEquivalentModifierMask = [.command, .control]
        view.addItem(NSMenuItem.separator())
        let reload = view.addItem(withTitle: "화면 다시 불러오기", action: #selector(reloadPage), keyEquivalent: "R")
        reload.keyEquivalentModifierMask = [.command, .shift]
        reload.target = self
        let browser = view.addItem(withTitle: "브라우저에서 열기", action: #selector(openInBrowser), keyEquivalent: "")
        browser.target = self
        let diag = view.addItem(withTitle: "진단 정보 보기", action: #selector(openDiag), keyEquivalent: "")
        diag.target = self
        viewItem.submenu = view

        let winItem = NSMenuItem()
        main.addItem(winItem)
        let win = NSMenu(title: "윈도우")
        win.addItem(withTitle: "최소화", action: #selector(NSWindow.performMiniaturize(_:)), keyEquivalent: "m")
        win.addItem(withTitle: "확대/축소", action: #selector(NSWindow.performZoom(_:)), keyEquivalent: "")
        win.addItem(withTitle: "닫기", action: #selector(NSWindow.performClose(_:)), keyEquivalent: "w")
        win.addItem(NSMenuItem.separator())
        let show = win.addItem(withTitle: "ValoROAD 창 보기", action: #selector(showMain), keyEquivalent: "")
        show.target = self
        winItem.submenu = win

        NSApp.mainMenu = main
        NSApp.windowsMenu = win
    }
}

// MARK: - 시작

// 엔진이 부르는 도우미: ValoROAD --riot-fetch (창 없이 요청 하나만 보내고 끝나요)
if CommandLine.arguments.contains("--riot-fetch") {
    exit(RiotFetch.run())
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.setActivationPolicy(.regular)
app.run()
