import Foundation

// 남은 사용 기간 (APP PASS)
//
// 무료 애플 ID 로 맥의 Xcode 에서 설치한 앱은 7일 동안만 열려요.
// 그 날짜는 앱 안의 embedded.mobileprovision 파일(애플이 서명한 설치 허가서)의 ExpirationDate 에 적혀 있어요.
// 끝나기 전에 아이폰을 맥에 연결하고 Xcode 에서 ▶ 를 누르면 새 허가서가 들어와 다시 7일이 시작돼요(자료는 남아요).

struct AppPass {
    var expires: Date?
    var created: Date?
    var free: Bool        // 무료 애플 ID(개인 팀)로 설치했는지
    var teamName: String

    static func read() -> AppPass {
        if Demo.on {
            let now = Date()
            return AppPass(expires: now.addingTimeInterval(5 * 86400 + 7 * 3600), created: now.addingTimeInterval(-2 * 86400),
                           free: true, teamName: "시험 모드")
        }
        guard let url = Bundle.main.url(forResource: "embedded", withExtension: "mobileprovision"),
              let raw = try? Data(contentsOf: url) else {
            return AppPass(expires: nil, created: nil, free: false, teamName: "")
        }
        // 서명 안에 들어 있는 plist 부분만 꺼내요
        guard let start = raw.range(of: Data("<?xml".utf8)),
              let end = raw.range(of: Data("</plist>".utf8), in: start.lowerBound..<raw.endIndex) else {
            return AppPass(expires: nil, created: nil, free: false, teamName: "")
        }
        let plist = raw.subdata(in: start.lowerBound..<end.upperBound)
        guard let dict = (try? PropertyListSerialization.propertyList(from: plist, format: nil)) as? [String: Any] else {
            return AppPass(expires: nil, created: nil, free: false, teamName: "")
        }
        let team = dict["TeamName"] as? String ?? ""
        let devices = dict["ProvisionedDevices"] as? [String] ?? []
        let days = dict["TimeToLive"] as? Int ?? 0
        return AppPass(expires: dict["ExpirationDate"] as? Date, created: dict["CreationDate"] as? Date,
                       free: days <= 7 && !devices.isEmpty, teamName: team)
    }

    /// 남은 날 (올림, 0~7)
    var daysLeft: Int? {
        guard let e = expires else { return nil }
        let s = e.timeIntervalSinceNow
        if s <= 0 { return 0 }
        return Int((s / 86400).rounded(.up))
    }

    /// 7칸 막대에 칠할 칸 수 (하루 미만이 남아도 1칸)
    var cells: Int { min(7, max(0, daysLeft ?? 0)) }

    var untilText: String {
        guard let e = expires else { return "" }
        let f = DateFormatter()
        f.locale = Locale(identifier: "ko_KR")
        f.dateFormat = "M월 d일 (E) a h시"
        let m = Calendar.current.component(.minute, from: e)
        if m != 0 { f.dateFormat = "M월 d일 (E) a h시 m분" }
        return f.string(from: e)
    }
}
