import Foundation
import Security

// 로그인 열쇠(라이엇 쿠키) 보관: 아이폰 키체인 (서비스 이름 ValoROAD, 계정마다 하나)
// '처음 잠금 해제 뒤 · 이 기기에서만' — 뒤에서 상점을 확인할 때도 읽을 수 있고, 다른 기기로 옮겨지지 않아요.
// 앱을 맥에서 다시 설치해도(7일마다) 키체인은 그대로 남아요.

enum Keychain {
    static let service = "ValoROAD"

    private static func base(_ account: String) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword,
         kSecAttrService as String: service,
         kSecAttrAccount as String: account]
    }

    @discardableResult
    static func set(_ account: String, _ data: Data) -> Bool {
        var q = base(account)
        let attrs: [String: Any] = [kSecValueData as String: data,
                                    kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly]
        var status = SecItemUpdate(q as CFDictionary, attrs as CFDictionary)
        if status == errSecItemNotFound {
            q.merge(attrs) { _, new in new }
            status = SecItemAdd(q as CFDictionary, nil)
        }
        if status != errSecSuccess {
            Log.write("⚠️ 키체인 저장 실패 (\(status))")
        }
        return status == errSecSuccess
    }

    static func get(_ account: String) -> Data? {
        var q = base(account)
        q[kSecReturnData as String] = true
        q[kSecMatchLimit as String] = kSecMatchLimitOne
        var out: CFTypeRef?
        let status = SecItemCopyMatching(q as CFDictionary, &out)
        if status != errSecSuccess {
            if status != errSecItemNotFound { Log.write("⚠️ 키체인 읽기 실패 (\(status))") }
            return nil
        }
        return out as? Data
    }

    static func delete(_ account: String) {
        SecItemDelete(base(account) as CFDictionary)
    }

    /// 설정 › 진단 정보에서 쓰는 자가 시험
    static func selfTest() -> Bool {
        let id = "selftest-" + Fmt.randomID(4)
        let v = Data("ok".utf8)
        let a = set(id, v)
        let b = get(id) == v
        delete(id)
        return a && b && get(id) == nil
    }
}
