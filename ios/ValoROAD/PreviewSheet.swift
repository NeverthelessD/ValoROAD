import AVFoundation
import AVKit
import SwiftUI

// 미리보기 (시안 ③): 게임처럼 아이템을 누르면 열려요
// 레벨 버튼 → 라이엇이 공개한 레벨 영상(소리와 함께 바로 재생) · 변형 견본 → 그 색의 이미지 · 가격 · 위시 버튼

struct PreviewSheet: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    let target: PreviewTarget

    var body: some View {
        // 미리보기는 늘 다크 (시안)
        PreviewBody(target: target, close: { dismiss() })
            .environment(\.palette, .dark)
            .presentationDetents([.large])
            .presentationDragIndicator(.visible)
            .presentationBackground(Palette.dark.bg)
    }
}

private enum Media: Equatable {
    case image(String)
    case video(String, String) // 주소, 설명
}

private struct PreviewBody: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.palette) private var p
    let target: PreviewTarget
    let close: () -> Void

    @State private var level = 0
    @State private var chroma = 0
    @State private var media: Media = .image("")
    @State private var player: AVPlayer?

    private var skin: Skin { target.skin }
    private var detail: SkinDetail {
        model.content.detail(skin.id) ?? SkinDetail(chromas: [Chroma(name: "기본", img: skin.img)], levels: [])
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                header
                mediaBox
                if detail.levels.count > 1 { levels }
                if detail.chromas.count > 1 { chromas }
                priceBox
                wishButton
                Text("영상은 라이엇이 공개한 미리보기 영상이에요 · 소리와 함께 바로 재생 · 영상의 전체 화면 단추로 크게")
                    .font(VF.plex(11))
                    .foregroundStyle(p.muted)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: .infinity)
            }
            .padding(.horizontal, 16)
            .padding(.top, 22)
            .padding(.bottom, 30)
        }
        .scrollIndicators(.hidden)
        .background(p.bg)
        .overlay(alignment: .top) { Rectangle().fill(p.red).frame(height: 3) }
        .foregroundStyle(p.text)
        .onAppear {
            Log.write("미리보기 열기: \(skin.name) · 레벨 \(detail.levels.count) · 변형 \(detail.chromas.count)")
            try? AVAudioSession.sharedInstance().setCategory(.playback, mode: .moviePlayback)
            media = .image(detail.chromas.first?.img.isEmpty == false ? detail.chromas[0].img : skin.img)
        }
        .onDisappear {
            player?.pause()
            player = nil
        }
        .onChange(of: media) { _, m in
            player?.pause()
            if case .video(let u, _) = m, let url = URL(string: u) {
                let pl = AVPlayer(url: url)
                player = pl
                pl.play()
            } else {
                player = nil
            }
        }
    }

    // MARK: 머리

    private var header: some View {
        HStack(alignment: .top, spacing: 12) {
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 8) {
                    Diamond().fill(Tier.color(skin.tier)).frame(width: 10, height: 10)
                    Text("\(skin.weaponEn.isEmpty ? skin.weapon : skin.weaponEn) · \(Tier.name(skin.tier))")
                        .font(VF.barlow(14)).tracking(1.8)
                        .foregroundStyle(p.muted)
                }
                Text(skin.name).font(VF.plex(24, .bold)).fixedSize(horizontal: false, vertical: true)
            }
            Spacer(minLength: 4)
            Button(action: close) {
                Image(systemName: "xmark")
                    .font(.system(size: 16, weight: .bold))
                    .frame(width: 44, height: 44)
                    .background(p.panel)
            }
            .buttonStyle(.plain)
            .accessibilityLabel("미리보기 닫기")
        }
    }

    // MARK: 영상 · 이미지

    private var mediaBox: some View {
        ZStack(alignment: .bottomLeading) {
            tierTint(skin.tier, p)
            switch media {
            case .image(let u):
                RemoteImage(url: u).padding(.horizontal, 18).padding(.top, 18).padding(.bottom, 40)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            case .video:
                if let player {
                    VideoPlayer(player: player)
                }
            }
            Text(caption)
                .font(VF.barlow(13)).tracking(1.3)
                .foregroundStyle(Palette.navy)
                .padding(.horizontal, 8).padding(.vertical, 3)
                .background(Palette.cream)
                .padding(10)
        }
        .frame(height: 210)
        .clipShape(Notch(cut: 18))
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(skin.name) \(caption)")
    }

    private var caption: String {
        switch media {
        case .image: return level == 0 || detail.levels.isEmpty ? "LV1 · 기본 이미지" : "LV\(level + 1) · 이미지"
        case .video(_, let what): return what
        }
    }

    private func levelName(_ l: Level, _ i: Int) -> String {
        switch l.item {
        case "": return i == 0 ? "기본" : "레벨 \(i + 1)"
        case "vfx": return "시각 효과"
        case "animation": return "애니메이션"
        case "finisher": return "피니셔"
        case "soundeffects": return "효과음"
        case "killcounter": return "킬 카운터"
        case "inspectandkill": return "살펴보기 · 처치"
        case "killbanner": return "처치 배너"
        case "transformation": return "변신"
        case "killeffect": return "처치 효과"
        case "randomizer": return "랜덤"
        case "topframe": return "프레임"
        default: return "레벨 \(i + 1)"
        }
    }

    private var levels: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("LEVELS · 레벨 영상").font(VF.barlow(14)).tracking(1.8).foregroundStyle(p.muted)
            LazyVGrid(columns: [GridItem(.flexible(), spacing: 8), GridItem(.flexible(), spacing: 8)], spacing: 8) {
                ForEach(Array(detail.levels.enumerated()), id: \.offset) { i, l in
                    let on = i == level && isLevelMedia
                    Button {
                        level = i
                        if !l.video.isEmpty {
                            media = .video(l.video, "LV\(i + 1) · \(levelName(l, i)) 영상")
                        } else {
                            let img = detail.chromas.indices.contains(chroma) ? detail.chromas[chroma].img : skin.img
                            media = .image(l.img.isEmpty ? img : (chroma == 0 ? l.img : img))
                        }
                    } label: {
                        HStack(spacing: 8) {
                            Text("LV\(i + 1)").font(VF.anton(16)).tracking(0.6)
                            Text(levelName(l, i)).font(VF.plex(13, .semibold)).lineLimit(1).minimumScaleFactor(0.8)
                            Spacer(minLength: 0)
                            if !l.video.isEmpty { Image(systemName: "play.fill").font(.system(size: 10)) }
                        }
                        .foregroundStyle(on ? Palette.navy : p.text)
                        .padding(.horizontal, 12)
                        .frame(maxWidth: .infinity, minHeight: 46)
                        .background(on ? p.red : Color.clear)
                        .overlay(Rectangle().stroke(on ? p.red : p.line, lineWidth: 1.5))
                    }
                    .buttonStyle(.plain)
                    .accessibilityAddTraits(on ? .isSelected : [])
                }
            }
        }
    }

    private var isLevelMedia: Bool {
        if case .video(_, let what) = media { return what.hasPrefix("LV") }
        return true
    }

    private var chromas: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 6) {
                Text("VARIANTS · 변형").font(VF.barlow(14)).tracking(1.8).foregroundStyle(p.muted)
                Text(detail.chromas.indices.contains(chroma) ? detail.chromas[chroma].name : "")
                    .font(VF.plex(13, .semibold)).lineLimit(1)
            }
            ScrollView(.horizontal) {
                HStack(spacing: 10) {
                    ForEach(Array(detail.chromas.enumerated()), id: \.offset) { i, c in
                        Button {
                            chroma = i
                            media = .image(c.img.isEmpty ? skin.img : c.img)
                        } label: {
                            Group {
                                if c.swatch.isEmpty {
                                    Rectangle().fill(p.panel)
                                } else {
                                    RemoteImage(url: c.swatch, mode: .fill)
                                }
                            }
                            .frame(width: 38, height: 38)
                            .clipped()
                            .padding(3)
                            .overlay(Rectangle().stroke(i == chroma ? p.text : p.line, lineWidth: 2))
                        }
                        .buttonStyle(.plain)
                        .accessibilityLabel(c.name)
                        .accessibilityAddTraits(i == chroma ? .isSelected : [])
                    }
                }
                .padding(2)
            }
            .scrollIndicators(.hidden)
            if detail.chromas.indices.contains(chroma), !detail.chromas[chroma].video.isEmpty {
                Button {
                    media = .video(detail.chromas[chroma].video, "\(detail.chromas[chroma].name) 영상")
                } label: {
                    Label("이 변형 영상", systemImage: "play.fill")
                        .font(VF.plex(13, .bold))
                        .padding(.horizontal, 12).frame(minHeight: 40)
                        .overlay(Rectangle().stroke(p.line, lineWidth: 1.5))
                }
                .buttonStyle(.plain)
            }
        }
    }

    // MARK: 가격 · 위시

    private var priceBox: some View {
        let pr = model.price(skin)
        let est = pr.0, state = pr.1
        let price = target.price ?? est
        let approx = target.price == nil && state == .estimate
        return HStack(spacing: 12) {
            VPIcon(size: 18, color: p.text)
            Text(price > 0 ? (approx ? "약 " : "") + Fmt.n(price) : "–").font(VF.anton(30))
            if let b = target.base, b > price {
                Text(Fmt.n(b)).font(VF.barlow(17, .semibold)).strikethrough().foregroundStyle(p.muted)
            }
            Spacer(minLength: 4)
            VStack(alignment: .trailing, spacing: 2) {
                if let c = target.context {
                    Text(c).font(VF.plex(12)).foregroundStyle(p.muted)
                } else {
                    Text(approx ? "등급별 보통 가격" : "가격").font(VF.plex(12)).foregroundStyle(p.muted)
                }
                if let e = target.ends, e > Int64(Date().timeIntervalSince1970) {
                    HStack(spacing: 4) {
                        Text("남은 시간").font(VF.plex(12))
                        Countdown(ends: e, days: e - Int64(Date().timeIntervalSince1970) > 86400).font(VF.barlow(14))
                    }
                    .foregroundStyle(p.muted)
                }
            }
        }
        .padding(14)
        .background(p.panel, in: Notch(cut: 14))
    }

    private var wishButton: some View {
        let wished = model.isWished(skin.id)
        return Button {
            model.setWish(skin.id, !wished)
        } label: {
            HStack(spacing: 10) {
                Image(systemName: wished ? "heart.fill" : "heart").font(.system(size: 18, weight: .semibold))
                Text(wished ? "위시리스트에 담겨 있어요" : "위시리스트에 담기").font(VF.plex(15, .bold))
            }
            .foregroundStyle(Palette.navy)
            .frame(maxWidth: .infinity, minHeight: 52)
            .background(wished ? p.hit : p.red, in: Notch(cut: 14))
        }
        .buttonStyle(.plain)
        .disabled(skin.tier.isEmpty && skin.name == Skin.placeholder("").name)
    }
}
