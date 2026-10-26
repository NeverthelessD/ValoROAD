import CryptoKit
import SwiftUI
import UIKit

// 스킨 이미지: media.valorant-api.com 에서 받아 Caches/ValoROAD/img 에 저장해 두고 보여 줘요 (지워져도 다시 받아요)

actor ImageStore {
    static let shared = ImageStore()
    private let memory = NSCache<NSString, UIImage>()
    private var running: [String: Task<UIImage?, Never>] = [:]

    init() { memory.countLimit = 300 }

    private func file(_ url: String) -> URL {
        let h = SHA256.hash(data: Data(url.utf8)).map { String(format: "%02x", $0) }.joined()
        return Paths.images.appendingPathComponent(String(h.prefix(40)) + ".img")
    }

    func cached(_ url: String) -> UIImage? { memory.object(forKey: url as NSString) }

    func image(_ url: String) async -> UIImage? {
        if let m = memory.object(forKey: url as NSString) { return m }
        if let t = running[url] { return await t.value }
        let path = file(url)
        let t = Task<UIImage?, Never> {
            if let d = try? Data(contentsOf: path), let img = UIImage(data: d) { return img }
            guard let u = URL(string: url), u.host?.hasSuffix("valorant-api.com") == true else { return nil }
            guard let res = try? await URLSession.shared.data(from: u), (res.1 as? HTTPURLResponse)?.statusCode == 200,
                  let img = UIImage(data: res.0) else { return nil }
            try? res.0.write(to: path, options: .atomic)
            return img
        }
        running[url] = t
        let img = await t.value
        running[url] = nil
        if let img { memory.setObject(img, forKey: url as NSString) }
        return img
    }
}

/// 스킨 · 번들 이미지 (받는 동안은 흐릿한 무기 모양 자리)
struct RemoteImage: View {
    let url: String
    var mode: ContentMode = .fit
    @State private var img: UIImage?

    var body: some View {
        Group {
            if let img {
                // 어떤 크기의 칸에 넣어도 가운데에 오도록 (아이폰에서 위쪽에 붙던 문제, v1.0.1)
                Image(uiImage: img).resizable().aspectRatio(contentMode: mode)
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .center)
            } else {
                Image(systemName: "scope")
                    .font(.system(size: 22, weight: .light))
                    .opacity(0.25)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .task(id: url) {
            guard !url.isEmpty else { img = nil; return }
            if let c = await ImageStore.shared.cached(url) { img = c; return }
            img = nil
            let loaded = await ImageStore.shared.image(url)
            if !Task.isCancelled { withAnimation(.easeOut(duration: 0.2)) { img = loaded } }
        }
        .accessibilityHidden(true)
    }
}
