import SwiftUI

/// The launch sentence as a keyboard panel (docs/SPEC.md, "The trigger modal"). Return confirms a
/// word and moves on, and launches on the last. Cmd+Return launches what is selected.
public struct LaunchPanel: View {
    @State private var sentence: LaunchSentence
    @State private var note: String
    @State private var reading = false
    @State private var launching = false
    @FocusState private var focused: Bool
    private let refresh: (() async -> LaunchSentence)?
    private let complete: ((String) async -> [LaunchProject])?
    private let launch: (LaunchRequest) async -> String?
    private let close: () -> Void

    /// `refresh` reads the hosts after the panel shows. `complete` asks the hosts for the folders
    /// under a typed path. `launch` answers an error to show, or nil when the launch started.
    public init(sentence: LaunchSentence, note: String = "", refresh: (() async -> LaunchSentence)? = nil,
                complete: ((String) async -> [LaunchProject])? = nil,
                launch: @escaping (LaunchRequest) async -> String?, close: @escaping () -> Void) {
        _sentence = State(initialValue: sentence)
        _note = State(initialValue: note)
        self.refresh = refresh
        self.complete = complete
        self.launch = launch
        self.close = close
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                ForEach(sentence.words, id: \.self) { w in
                    if let join = LaunchSentence.joins[w] { Text(join).font(.system(size: 15)).foregroundStyle(.tertiary) }
                    WordView(text: sentence.text(w), on: w == sentence.word).onTapGesture { sentence.focus(w) }
                }
            }
            ScrollViewReader { proxy in
                ScrollView {
                    VStack(spacing: 0) {
                        ForEach(sentence.options) { o in
                            OptionRow(option: o, on: o.id == sentence.selected).id(o.id)
                                .contentShape(Rectangle())
                                .onTapGesture { sentence.pick(o.id) }
                        }
                    }
                }
                .scrollIndicators(.never)
                .onChange(of: sentence.selected) { proxy.scrollTo(sentence.selected) }
            }
            if !shownNote.isEmpty { Text(shownNote).font(.caption).foregroundStyle(.secondary) }
        }
        .padding(20)
        .frame(width: 820, height: 400)
        .background(.regularMaterial)
        .clipShape(RoundedRectangle(cornerRadius: 18))
        .focusable()
        .focusEffectDisabled()
        .focused($focused)
        .onAppear { focused = true }
        .onKeyPress(phases: .down, action: press)
        .task {
            guard let refresh else { return }
            reading = true
            sentence.adopt(await refresh())
            reading = false
        }
        // A new query cancels the last one's request, so a slow host cannot answer a stale query.
        .task(id: sentence.completing) {
            guard let prefix = sentence.completing, let complete else { return }
            try? await Task.sleep(for: .milliseconds(120))
            guard !Task.isCancelled else { return }
            let folders = await complete(prefix)
            guard !Task.isCancelled else { return }
            sentence.found(folders, for: prefix)
        }
    }

    /// An error first. With no projects yet, "no project" would read as an answer.
    private var shownNote: String {
        if !note.isEmpty { return note }
        return reading && sentence.projects.isEmpty ? "Reading hosts" : sentence.note
    }

    private func press(_ k: KeyPress) -> KeyPress.Result {
        switch k.key {
        case .tab:
            if k.modifiers.contains(.shift) { sentence.cycle(-1) } else if !sentence.complete() { sentence.cycle(1) }
        case KeyEquivalent("\u{19}"): sentence.cycle(-1) // macOS sends Shift+Tab as back-tab
        case .leftArrow: sentence.step(-1)
        case .rightArrow: sentence.step(1)
        case .upArrow: sentence.move(-1)
        case .downArrow: sentence.move(1)
        case .escape: if !sentence.back() { close() }
        case .return:
            if k.modifiers.contains(.command) || sentence.onLast { go() } else { sentence.confirm() }
        case .delete: sentence.erase()
        default: return typed(k.characters)
        }
        return .handled
    }

    private func typed(_ text: String) -> KeyPress.Result {
        guard let c = text.first, text.count == 1, c.isLetter || c.isNumber || " .-_/~".contains(c) else { return .ignored }
        if let n = c.wholeNumberValue, n >= 1, sentence.query.isEmpty, n <= sentence.options.count {
            sentence.pick(sentence.options[n - 1].id)
            if !sentence.onLast { sentence.step(1) }
        } else {
            sentence.type(text)
        }
        return .handled
    }

    /// One launch at a time: a second Return while `rex` runs would start a second agent.
    private func go() {
        guard !launching else { return }
        // During a read most problems may clear by themselves, but a waiting target is why
        // nothing launched, so it is said at once.
        guard let request = sentence.request else {
            note = reading && sentence.target == nil ? "" : sentence.problem ?? ""
            return
        }
        launching = true
        Task {
            if let error = await launch(request) { note = error } else { close() }
            launching = false
        }
    }
}

private struct WordView: View {
    let text: String
    let on: Bool
    var body: some View {
        VStack(spacing: 6) {
            Text(text).font(.system(size: 21, weight: .medium)).lineLimit(1).fixedSize()
                .foregroundStyle(on ? Color.primary : Color.secondary)
            Rectangle().fill(on ? Color.accentColor : .clear).frame(height: 2)
        }
        .fixedSize()
    }
}

private struct OptionRow: View {
    let option: LaunchSentence.Option
    let on: Bool
    var body: some View {
        HStack(spacing: 10) {
            if !option.mark.isEmpty { Text(option.mark) }
            Text(option.title)
            Spacer()
            Text(option.note).foregroundStyle(.secondary)
        }
        .font(.system(size: 15))
        .padding(.horizontal, 12)
        .frame(minHeight: 36)
        .background(RoundedRectangle(cornerRadius: 8).fill(on ? Color.primary.opacity(0.09) : .clear))
    }
}
