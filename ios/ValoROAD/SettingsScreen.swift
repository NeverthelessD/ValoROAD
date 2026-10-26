import SwiftUI
import UIKit

// 설정 화면 (시안 ⑥): 남은 사용 기간(APP PASS, 7칸 막대) · 계정 · 화면 모드 · 앱 정보 · 진단

struct SettingsScreen: View {
    @EnvironmentObject private var model: AppModel
    @EnvironmentObject private var router: Router
    @Environment(\.palette) private var p
    @State private var removing: Account?
    @State private var showDiag = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                BrandMark()
                VStack(alignment: .leading, spacing: 4) {
                    Text("SETTINGS").font(VF.anton(40)).foregroundStyle(p.text)
                    Text("설정 · 계정 · 앱 정보").font(VF.plex(13, .semibold)).foregroundStyle(p.muted)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(alignment: .topTrailing) {
                    Watermark(text: "SETTINGS", size: 96).offset(x: 14, y: -26)
                }
                PassCard(pass: model.pass)
                accounts
                display
                info
                Footer()
            }
            .padding(.horizontal, 16)
            .padding(.top, 8)
            .padding(.bottom, 20)
        }
        .scrollIndicators(.hidden)
        .confirmationDialog("이 계정을 지울까요?", isPresented: Binding(get: { removing != nil }, set: { if !$0 { removing = nil } }),
                            titleVisibility: .visible, presenting: removing) { acc in
            Button("\(acc.label) 지우기", role: .destructive) { model.remove(acc.id) }
            Button("취소", role: .cancel) {}
        } message: { _ in
            Text("이 아이폰에 저장된 로그인 정보만 지워져요. 라이엇 계정에는 아무 일도 없어요.")
        }
        .sheet(isPresented: $showDiag) { DiagnosticsSheet() }
    }

    // MARK: 계정

    private var accounts: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .firstTextBaseline) {
                SectionTitle(en: "ACCOUNTS", ko: "계정")
                Spacer()
                Text("\(model.accounts.count) / \(AppModel.maxAccounts)").font(VF.barlow(16)).foregroundStyle(p.muted)
            }
            ForEach(model.accounts) { a in
                HStack(spacing: 12) {
                    Text(a.slotText).font(VF.anton(26)).foregroundStyle(p.muted.opacity(0.7))
                    VStack(alignment: .leading, spacing: 2) {
                        (Text(a.name + " ").foregroundColor(p.text) + Text("#" + a.tag).foregroundColor(p.muted))
                            .font(VF.plex(15, .bold))
                            .lineLimit(1)
                        HStack(spacing: 6) {
                            Rectangle().fill(stateColor(a)).frame(width: 7, height: 7)
                            Text(stateText(a)).font(VF.plex(12, .semibold)).foregroundStyle(stateColor(a)).lineLimit(2)
                        }
                    }
                    Spacer(minLength: 0)
                    if a.status == "expired" {
                        Button { router.openLogin(slot: a.slot) } label: {
                            Text("다시 로그인").font(VF.plex(13, .bold)).foregroundStyle(Palette.navy)
                                .padding(.horizontal, 12).frame(minHeight: 40).background(Palette.crimson)
                        }
                        .buttonStyle(.plain)
                    }
                    Button { removing = a } label: {
                        Image(systemName: "trash").font(.system(size: 16)).foregroundStyle(p.muted).frame(width: 44, height: 44)
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("\(a.label) 계정 지우기")
                }
                .padding(.leading, 12).padding(.trailing, 4).padding(.vertical, 10)
                .background(p.panel)
            }
            if model.accounts.count < AppModel.maxAccounts {
                Button { router.openLogin(slot: nil) } label: {
                    HStack(spacing: 8) {
                        Image(systemName: "plus").font(.system(size: 13, weight: .heavy))
                        Text("ADD").font(VF.barlow(15)).tracking(1.5)
                        Text("계정 추가").font(VF.plex(13, .semibold))
                    }
                    .foregroundStyle(p.text)
                    .frame(maxWidth: .infinity, minHeight: 46)
                    .overlay(Rectangle().stroke(p.line, lineWidth: 1.5))
                }
                .buttonStyle(.plain)
            } else {
                Text("자리 3개가 모두 찼어요. 새 계정을 넣으려면 하나를 지워 주세요.")
                    .font(VF.plex(12)).foregroundStyle(p.muted)
            }
        }
    }

    private func stateText(_ a: Account) -> String {
        if model.busy.contains(a.id) { return "상점 받는 중…" }
        switch a.status {
        case "expired": return "로그인이 풀렸어요"
        case "error": return a.error.isEmpty ? "잠시 연결이 안 됐어요" : a.error
        default: return "자동 로그인 유지 중 · \(a.region.uppercased())"
        }
    }

    private func stateColor(_ a: Account) -> Color {
        switch a.status {
        case "expired": return p.red
        case "error": return Color(hex: 0xF5955B)
        default: return p.hit
        }
    }

    // MARK: 화면 모드

    private var display: some View {
        VStack(alignment: .leading, spacing: 10) {
            SectionTitle(en: "DISPLAY", ko: "화면")
            HStack(spacing: 6) {
                ForEach(["auto", "dark", "light"], id: \.self) { key in
                    let label = key == "auto" ? "자동" : key == "dark" ? "다크" : "라이트"
                    let on = model.settings.theme == key
                    Button {
                        model.settings.theme = key
                        model.saveSettings()
                    } label: {
                        Text(label).font(VF.plex(14, .bold))
                            .foregroundStyle(on ? (p.isDark ? Palette.navy : Palette.cream) : p.text)
                            .frame(maxWidth: .infinity, minHeight: 44)
                            .background(on ? p.text : Color.clear)
                            .overlay(Rectangle().stroke(on ? p.text : p.line, lineWidth: 1.5))
                    }
                    .buttonStyle(.plain)
                    .accessibilityAddTraits(on ? .isSelected : [])
                }
            }
            Text("자동은 아이폰 설정(라이트 · 다크)을 따라가요").font(VF.plex(12)).foregroundStyle(p.muted)
        }
    }

    // MARK: 앱 정보

    private var info: some View {
        VStack(spacing: 0) {
            HStack {
                Text("버전").font(VF.plex(14, .semibold))
                Spacer()
                Text("v\(appVersion)").font(VF.barlow(17))
            }
            .frame(minHeight: 50)
            .padding(.horizontal, 16)
            .overlay(alignment: .bottom) { Rectangle().fill(p.line).frame(height: 1) }
            VStack(alignment: .leading, spacing: 2) {
                Text("새 버전").font(VF.plex(14, .semibold))
                Text("새 버전은 맥에서 다시 설치할 때(Xcode 의 ▶) 함께 들어와요.")
                    .font(VF.plex(12)).foregroundStyle(p.muted)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 16).padding(.vertical, 12)
            .overlay(alignment: .bottom) { Rectangle().fill(p.line).frame(height: 1) }
            Button { showDiag = true } label: {
                HStack {
                    Text("진단 정보 보기").font(VF.plex(14, .semibold))
                    Spacer()
                    Image(systemName: "chevron.right").font(.system(size: 13, weight: .bold)).foregroundStyle(p.muted)
                }
                .frame(minHeight: 50)
                .padding(.horizontal, 16)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
        }
        .foregroundStyle(p.text)
        .background(p.panel, in: Notch(cut: 18))
    }
}

// MARK: - 남은 사용 기간

struct PassCard: View {
    @Environment(\.palette) private var p
    let pass: AppPass

    var body: some View {
        let days = pass.daysLeft
        let warn = (days ?? 9) <= 1
        VStack(alignment: .leading, spacing: 12) {
            HStack(alignment: .bottom) {
                VStack(alignment: .leading, spacing: 2) {
                    Text("APP PASS · 남은 사용 기간").font(VF.barlow(13)).tracking(1.8).foregroundStyle(p.muted)
                    if let days {
                        HStack(alignment: .lastTextBaseline, spacing: 4) {
                            Text("\(days)").font(VF.anton(52))
                            Text("일").font(VF.plex(18, .bold))
                        }
                        .foregroundStyle(warn ? p.redText : p.text)
                    } else {
                        Text("기간 제한 없음").font(VF.plex(20, .bold)).foregroundStyle(p.text)
                    }
                }
                Spacer()
                if pass.expires != nil {
                    Text("\(pass.untilText)\n까지 써져요")
                        .font(VF.plex(13))
                        .multilineTextAlignment(.trailing)
                        .foregroundStyle(p.text.opacity(0.85))
                }
            }
            if days != nil {
                HStack(spacing: 4) {
                    ForEach(0..<7, id: \.self) { i in
                        Rectangle()
                            .fill(i < pass.cells ? (warn ? p.red : p.hit) : p.line)
                            .frame(height: 8)
                    }
                }
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("7일 중 \(days ?? 0)일 남음")
                Text("끝나기 전에 아이폰을 맥에 연결하고 Xcode 에서 ▶ 를 누르면 다시 7일이 시작돼요. 계정 · 위시리스트는 그대로 남아요. 하루 전에 알림으로 알려 드려요.")
                    .font(VF.plex(13))
                    .foregroundStyle(p.text.opacity(0.85))
                    .fixedSize(horizontal: false, vertical: true)
            } else {
                Text("이 설치에는 사용 기간 정보가 없어요 (시뮬레이터 또는 유료 개발자 계정).")
                    .font(VF.plex(13)).foregroundStyle(p.muted)
            }
        }
        .padding(16)
        .background(p.panel)
        .overlay(alignment: .top) { Rectangle().fill(p.red).frame(height: 3) }
        .clipShape(Notch(cut: 18))
    }
}

// MARK: - 진단

private struct DiagnosticsSheet: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var text = ""
    @State private var copied = false

    var body: some View {
        NavigationStack {
            ScrollView {
                Text(text)
                    .font(.system(size: 12, design: .monospaced))
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(16)
            }
            .navigationTitle("진단 정보")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("닫기") { dismiss() } }
                ToolbarItem(placement: .primaryAction) {
                    Button(copied ? "복사됨" : "복사") {
                        UIPasteboard.general.string = text
                        copied = true
                    }
                }
            }
            .safeAreaInset(edge: .bottom) {
                Text("문제가 생기면 ‘복사’를 눌러 이 내용을 그대로 보내 주세요. 비밀번호 · 로그인 열쇠는 들어 있지 않아요.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .padding(12)
                    .frame(maxWidth: .infinity)
                    .background(.bar)
            }
        }
        .task { text = model.diagnostics() }
    }
}
