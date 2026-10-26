import Foundation

// 저장 위치 · 기록
//
// Application Support/ValoROAD : accounts.json · settings.json · stores/<id>.json · content.json · prices.json · notified.json · log.txt
// Caches/ValoROAD/img          : 스킨 이미지 (지워져도 다시 받아요)
// 로그인 열쇠(라이엇 쿠키)는 파일이 아니라 아이폰 키체인에 있어요 (Keychain.swift).

enum Paths {
    static let support: URL = {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        let u = base.appendingPathComponent("ValoROAD", isDirectory: true)
        try? FileManager.default.createDirectory(at: u, withIntermediateDirectories: true)
        return u
    }()

    static let images: URL = {
        let base = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0]
        let u = base.appendingPathComponent("ValoROAD/img", isDirectory: true)
        try? FileManager.default.createDirectory(at: u, withIntermediateDirectories: true)
        return u
    }()

    static func file(_ name: String) -> URL { support.appendingPathComponent(name) }
}

enum Disk {
    static func load<T: Decodable>(_ type: T.Type, _ name: String) -> T? {
        guard let d = try? Data(contentsOf: Paths.file(name)) else { return nil }
        return try? JSONDecoder().decode(T.self, from: d)
    }

    static func save<T: Encodable>(_ value: T, _ name: String) {
        let u = Paths.file(name)
        try? FileManager.default.createDirectory(at: u.deletingLastPathComponent(), withIntermediateDirectories: true)
        guard let d = try? JSONEncoder().encode(value) else { return }
        do {
            try d.write(to: u, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
        } catch {
            Log.write("⚠️ 저장 실패 \(name): \(error.localizedDescription)")
        }
    }

    static func remove(_ name: String) {
        try? FileManager.default.removeItem(at: Paths.file(name))
    }
}

/// 진단 기록: 설정 › 진단 정보 보기 에서 볼 수 있고, 문제가 생기면 사용자가 그대로 보내 줄 수 있어요.
enum Log {
    private static let queue = DispatchQueue(label: "valoroad.log")
    private static let url = Paths.file("log.txt")
    private static let stamp: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "MM-dd HH:mm:ss"
        return f
    }()

    static func write(_ message: String) {
        let line = "\(stamp.string(from: Date())) \(message)\n"
        print(line, terminator: "")
        queue.async {
            let data = Data(line.utf8)
            if let h = try? FileHandle(forWritingTo: url) {
                h.seekToEndOfFile()
                h.write(data)
                try? h.close()
            } else {
                try? data.write(to: url)
            }
            // 너무 길어지면 뒤쪽 절반만 남겨요
            if let attrs = try? FileManager.default.attributesOfItem(atPath: url.path),
               let size = attrs[.size] as? Int, size > 300_000,
               let all = try? Data(contentsOf: url) {
                try? all.suffix(150_000).write(to: url)
            }
        }
    }

    static func text() -> String {
        queue.sync {
            (try? String(contentsOf: url, encoding: .utf8)) ?? ""
        }
    }
}

enum Fmt {
    static let number: NumberFormatter = {
        let f = NumberFormatter()
        f.numberStyle = .decimal
        f.locale = Locale(identifier: "ko_KR")
        return f
    }()

    static func n(_ v: Int) -> String { number.string(from: NSNumber(value: v)) ?? "\(v)" }

    /// 남은 시간 "14:22:08"
    static func clock(_ seconds: TimeInterval) -> String {
        let s = max(0, Int(seconds))
        return String(format: "%02d:%02d:%02d", s / 3600, (s % 3600) / 60, s % 60)
    }

    /// 남은 기간 "3일 07:12" (하루 미만이면 "07:12:30")
    static func days(_ seconds: TimeInterval) -> String {
        let s = max(0, Int(seconds))
        let d = s / 86400
        if d == 0 { return clock(TimeInterval(s)) }
        return String(format: "%d일 %02d:%02d", d, (s % 86400) / 3600, (s % 3600) / 60)
    }

    /// 받침이 있으면 "이", 없으면 "가" (한글이 아니면 "이(가)")
    static func josa(_ word: String) -> String {
        guard let last = word.trimmingCharacters(in: .whitespaces).unicodeScalars.last else { return "이(가)" }
        let v = last.value
        if v >= 0xAC00 && v <= 0xD7A3 { return (v - 0xAC00) % 28 != 0 ? "이" : "가" }
        return "이(가)"
    }

    static func randomID(_ n: Int = 8) -> String {
        let chars = Array("abcdefghijklmnopqrstuvwxyz0123456789")
        return String((0..<n).map { _ in chars.randomElement()! })
    }
}
