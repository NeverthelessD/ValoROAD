import SwiftUI

// 확정 디자인 (PROJECT_NOTES 「결정 사항」 · 아이폰 시안 v1)
// 남색 #0F1923 · 패널 #182633 · 크림 #ECE8E1 · 빨강 #FF4655 · 위시 적중 청록 #3DDBC4
// 라이트: 크림 바탕 #ECE8E1, 글자 #0F1923, 빨간 글자 #C42536

extension Color {
    init(hex: UInt32, alpha: Double = 1) {
        self.init(.sRGB,
                  red: Double((hex >> 16) & 0xFF) / 255,
                  green: Double((hex >> 8) & 0xFF) / 255,
                  blue: Double(hex & 0xFF) / 255,
                  opacity: alpha)
    }
}

struct Palette {
    let bg: Color
    let head: Color
    let panel: Color
    let line: Color
    let text: Color
    let muted: Color
    let red: Color
    let redText: Color
    let hit: Color
    let onHit: Color
    let tintAlpha: Double
    let isDark: Bool

    static let navy = Color(hex: 0x0F1923)
    static let cream = Color(hex: 0xECE8E1)
    static let crimson = Color(hex: 0xFF4655)

    static let dark = Palette(bg: Color(hex: 0x0F1923), head: Color(hex: 0x0B131B), panel: Color(hex: 0x182633),
                              line: Color(hex: 0x2B3B4A), text: Color(hex: 0xECE8E1), muted: Color(hex: 0x9AA6B1),
                              red: Color(hex: 0xFF4655), redText: Color(hex: 0xFF4655), hit: Color(hex: 0x3DDBC4),
                              onHit: Color(hex: 0x0F1923), tintAlpha: 0.2, isDark: true)
    static let light = Palette(bg: Color(hex: 0xECE8E1), head: Color(hex: 0xF6F3EE), panel: Color(hex: 0xF8F6F2),
                               line: Color(hex: 0xCFC8BC), text: Color(hex: 0x0F1923), muted: Color(hex: 0x545E6A),
                               red: Color(hex: 0xFF4655), redText: Color(hex: 0xC42536), hit: Color(hex: 0x0B7F70),
                               onHit: Color(hex: 0xFFFFFF), tintAlpha: 0.26, isDark: false)

    static func of(_ scheme: ColorScheme) -> Palette { scheme == .dark ? .dark : .light }
}

private struct PaletteKey: EnvironmentKey {
    static let defaultValue = Palette.dark
}

extension EnvironmentValues {
    var palette: Palette {
        get { self[PaletteKey.self] }
        set { self[PaletteKey.self] = newValue }
    }
}

/// 화면 전체에 지금 색 묶음(다크 · 라이트)을 넣어 줘요
struct PaletteProvider<Content: View>: View {
    @Environment(\.colorScheme) private var scheme
    let content: Content
    init(@ViewBuilder content: () -> Content) { self.content = content() }
    var body: some View { content.environment(\.palette, Palette.of(scheme)) }
}

// MARK: - 글꼴 (모두 OFL, Fonts 폴더에 동봉)

enum VF {
    /// 큰 영어 제목 (STORE · DAILY OFFERS · 가격 숫자)
    static func anton(_ size: CGFloat) -> Font { .custom("Anton-Regular", size: size) }

    /// 라벨 · 숫자
    static func barlow(_ size: CGFloat, _ weight: Font.Weight = .bold) -> Font {
        switch weight {
        case .medium, .regular: return .custom("BarlowCondensed-Medium", size: size)
        case .semibold: return .custom("BarlowCondensed-SemiBold", size: size)
        default: return .custom("BarlowCondensed-Bold", size: size)
        }
    }

    /// 한글 본문
    static func plex(_ size: CGFloat, _ weight: Font.Weight = .regular) -> Font {
        switch weight {
        case .bold, .heavy, .black: return .custom("IBMPlexSansKR-Bold", size: size)
        case .semibold: return .custom("IBMPlexSansKR-SemiBold", size: size)
        case .medium: return .custom("IBMPlexSansKR-Medium", size: size)
        default: return .custom("IBMPlexSansKR-Regular", size: size)
        }
    }
}

// MARK: - 모양

/// 오른쪽 아래 모서리를 비스듬히 자른 카드 · 버튼 모양
struct Notch: Shape {
    var cut: CGFloat = 18
    func path(in r: CGRect) -> Path {
        var p = Path()
        let c = min(cut, r.width / 2, r.height / 2)
        p.move(to: CGPoint(x: r.minX, y: r.minY))
        p.addLine(to: CGPoint(x: r.maxX, y: r.minY))
        p.addLine(to: CGPoint(x: r.maxX, y: r.maxY - c))
        p.addLine(to: CGPoint(x: r.maxX - c, y: r.maxY))
        p.addLine(to: CGPoint(x: r.minX, y: r.maxY))
        p.closeSubpath()
        return p
    }
}

/// 등급 표시용 마름모
struct Diamond: Shape {
    func path(in r: CGRect) -> Path {
        var p = Path()
        p.move(to: CGPoint(x: r.midX, y: r.minY))
        p.addLine(to: CGPoint(x: r.maxX, y: r.midY))
        p.addLine(to: CGPoint(x: r.midX, y: r.maxY))
        p.addLine(to: CGPoint(x: r.minX, y: r.midY))
        p.closeSubpath()
        return p
    }
}

/// 번들 카드 오른쪽의 빨간 사선 블록
struct Slant: Shape {
    func path(in r: CGRect) -> Path {
        var p = Path()
        p.move(to: CGPoint(x: r.minX + r.width * 0.4, y: r.minY))
        p.addLine(to: CGPoint(x: r.maxX, y: r.minY))
        p.addLine(to: CGPoint(x: r.maxX, y: r.maxY))
        p.addLine(to: CGPoint(x: r.minX, y: r.maxY))
        p.closeSubpath()
        return p
    }
}

// MARK: - 등급

enum Tier {
    static func color(_ tier: String) -> Color {
        switch tier {
        case "select": return Color(hex: 0x5A9FE2)
        case "deluxe": return Color(hex: 0x2FC48D)
        case "premium": return Color(hex: 0xD1548D)
        case "exclusive": return Color(hex: 0xF5955B)
        case "ultra": return Color(hex: 0xFAD663)
        default: return Color(hex: 0x9AA6B1)
        }
    }

    static func name(_ tier: String) -> String {
        switch tier {
        case "select": return "셀렉트"
        case "deluxe": return "디럭스"
        case "premium": return "프리미엄"
        case "exclusive": return "익스클루시브"
        case "ultra": return "울트라"
        default: return "기본"
        }
    }
}

// MARK: - 작은 공통 조각

/// VP 아이콘 (마름모 안에 작은 마름모)
struct VPIcon: View {
    var size: CGFloat = 14
    var color: Color
    var body: some View {
        ZStack {
            Diamond().stroke(color, lineWidth: size * 0.11)
            Diamond().fill(color).frame(width: size * 0.38, height: size * 0.38)
        }
        .frame(width: size, height: size)
        .accessibilityHidden(true)
    }
}

/// 섹션 제목 (큰 영어 + 작은 한글)
struct SectionTitle: View {
    @Environment(\.palette) private var p
    let en: String
    let ko: String
    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 6) {
            Text(en).font(VF.anton(22)).tracking(0.4)
            Text(ko).font(VF.plex(13, .semibold)).foregroundStyle(p.muted)
        }
        .foregroundStyle(p.text)
    }
}

/// 배경에 크게 깔리는 외곽선 느낌 영어 글자 (STORE · WISHLIST · SETTINGS)
struct Watermark: View {
    @Environment(\.palette) private var p
    let text: String
    var size: CGFloat = 104
    var body: some View {
        Text(text)
            .font(VF.anton(size))
            .foregroundStyle(p.line.opacity(p.isDark ? 0.55 : 0.6))
            .lineLimit(1)
            .fixedSize()
            .accessibilityHidden(true)
            .allowsHitTesting(false)
    }
}

/// 화면 위쪽 머리 (로고 + ValoROAD)
struct BrandMark: View {
    @Environment(\.palette) private var p
    var body: some View {
        HStack(spacing: 10) {
            Image("Logo")
                .resizable()
                .frame(width: 32, height: 32)
                .clipShape(RoundedRectangle(cornerRadius: 7, style: .continuous))
                .accessibilityHidden(true)
            (Text("Valo").foregroundColor(p.text) + Text("ROAD").foregroundColor(p.redText))
                .font(VF.anton(24))
                .tracking(0.7)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("ValoROAD")
    }
}

/// 맨 아래 만든 이 · 안내 문구
struct Footer: View {
    @Environment(\.palette) private var p
    var body: some View {
        VStack(spacing: 4) {
            Text("made by. Nevertheless_D")
                .font(VF.barlow(15))
                .tracking(1.2)
                .foregroundStyle(p.text)
            Text("ValoROAD는 Riot Games가 만들거나 보증한 앱이 아니에요 · 확인 전용")
                .font(VF.plex(11))
                .foregroundStyle(p.muted)
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity)
        .padding(.top, 6)
    }
}

/// 등급 색을 옅게 깐 배경
func tierTint(_ tier: String, _ p: Palette) -> Color {
    Tier.color(tier).opacity(p.tintAlpha + 0.04)
}

/// 무기 분류 → 화면에 보이는 영어 이름 (분류 단추)
enum WeaponCat {
    static let filters: [(key: String, label: String)] = [
        ("all", "ALL"), ("rifle", "RIFLES"), ("sidearm", "SIDEARMS"), ("smg", "SMGS"),
        ("shotgun", "SHOTGUNS"), ("sniper", "SNIPERS"), ("heavy", "HEAVY"), ("melee", "MELEE")
    ]
}
