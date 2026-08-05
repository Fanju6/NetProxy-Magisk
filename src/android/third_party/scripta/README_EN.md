# scripta

[简体中文](README.md) | **English**

A self-drawn, virtualized code editor for [Compose Multiplatform](https://github.com/JetBrains/compose-multiplatform) — Android and Desktop (JVM).

scripta does not wrap `BasicTextField`: the piece-tree text buffer, viewport-virtualized rendering, gesture/selection/undo systems, and the IME session on both platforms are all its own — which is what keeps large documents fast and editor behavior fully controllable.

**Status: pre-release.** Android is the primary platform, desktop is maintained alongside; not yet published to Maven Central.

## Highlights

- **Virtualized rendering** — only visible lines are measured and drawn; per-line layouts are LRU-cached and invalidated by edit range.
- **Piece-tree buffer** — O(log n) edits and offset↔(line, column) mapping; cost independent of document size.
- **Long-line fast path** — in horizontal-scroll mode, printable-ASCII lines beyond 2 000 chars only measure the visible column window, with monospace-arithmetic geometry.
- **Self-managed IME** — Android drives a custom `InputConnection` straight into the engine; desktop gives CJK input methods a live document view, with the candidate window anchored at the caret.
- **Touch *and* desktop input as equals** — selection handles, drag magnifier, and pinch zoom on touch; full mouse and hardware-keyboard support on desktop — gated by *input type*, not platform.
- **Incremental syntax highlighting** — a per-line plugin interface with cross-line state chaining; YAML built in, custom languages in ~50 lines.
- **Pull-based host API** — a `@Stable` controller exposes snapshot state and imperative calls; no per-keystroke callbacks.

## Feature tour

### Editing
- Undo/redo with typing-unit merging: continuous typing undoes in one step, IME composing collapses to its net effect; capped at 1 000 units.
- Auto-indent on Enter.
- Tab / Shift+Tab block indent/outdent as one undo unit.
- Bracket/quote auto-close, type-over skip, and paired backspace — with a quote-intent heuristic so `it's` doesn't become `it''s`; opt out via `autoClosePairs = false`.
- Bracket-match highlight next to the caret.
- Ctrl+/ comment toggle: line comments aligned at the minimum indent column, mixed state adds a reversible layer; languages without line comments fall back to block-comment wrapping. Metadata comes from the highlighter plugin.
- Bottom symbol quick-input bar for soft keyboards.

### Navigation, find, selection
- Docked find & replace bar: case / whole-word / regex toggles, Enter/Shift+Enter to cycle, replace-all as one undo step.
- Goto-line bar (Ctrl+G).
- Word-wise movement, Home/End/PageUp/PageDown, visual-row Up/Down under soft wrap, Shift-extension on every navigation key.
- Programmatic jumps always reveal the caret — even when jumping to the same position twice.

### Touch
- Tap places the caret; tapping the caret opens a paste bubble; double-tap selects a word; long-press selects and drags by word.
- Caret/selection handles plus a "liquid glass" drag magnifier.
- Four-directional edge auto-scroll while dragging handles.
- Free pinch zoom, committed into scroll position and font size (8–40 sp) on release.
- Fade-out vertical scrollbar with a grabbable thumb for fast scrolling.

### Mouse & hardware keyboard
- Multi-click char → word → line selection granularity, Shift+click extension, hover I-beam, wheel at 3 lines per tick.
- Right-click context menu, on desktop and with mice on Android.
- Full shortcut set below; the macOS keymap switches automatically; AltGr combinations are never misread.
- Mouse interaction hides touch chrome; a stylus still behaves like touch.

### Documents
- CRLF fidelity: LF-normalized internally, dominant line ending detected at load and preserved through editing, `getText(lineEnding)` restores it on save.
- `rememberSaveableCodeEditorController` survives rotation and process death.
- Read-only mode keeps scrolling, zooming, selection, copy, and find.

## Platform support

| | Android | Desktop (JVM) |
|---|---|---|
| Minimum version | API 24 | JDK 21 toolchain |
| IME / CJK composing | self-managed `InputConnection` | text-input session, caret-anchored candidate window |
| Magnifier glass shader | Android 13+ (AGSL; plain border below) | skiko `RuntimeEffect` |
| Keymap | Ctrl-based | Ctrl-based, Cmd/Option on macOS |
| Back-gesture exclusion for the scrollbar | ✔ | n/a |

Android is the primary platform; the desktop target keeps the shared logic honest.

## Getting started

scripta is not on Maven Central yet — consume it via a Gradle composite build:

```kotlin
// settings.gradle.kts (scripta checked out or added as a git submodule)
includeBuild("third_party/scripta")
```

```kotlin
// build.gradle.kts — substituted with the included build's :editor project
dependencies {
    implementation("scripta:editor")
}
```

A composite build keeps scripta's own version catalog and plugin set intact; align your Kotlin/AGP/Compose versions with the toolchain below. The library's only external dependency is `org.jetbrains.compose.foundation:foundation`. Toolchain used by this repo: Kotlin 2.4.0, Compose Multiplatform 1.11.1, AGP 9.2.1, Gradle 9.6.1, JDK 21.

### Minimal usage

```kotlin
import top.yukonga.scripta.editor.CodeEditor
import top.yukonga.scripta.editor.EditorLanguage
import top.yukonga.scripta.editor.rememberSaveableCodeEditorController

@Composable
fun EditorScreen() {
    val controller = rememberSaveableCodeEditorController(initialText = "hello: world")
    CodeEditor(
        controller = controller,
        language = EditorLanguage.Yaml,
        modifier = Modifier.fillMaxSize(),
    )
}
```

The full surface:

```kotlin
@Composable
fun CodeEditor(
    controller: CodeEditorController,
    modifier: Modifier = Modifier,
    language: EditorLanguage = EditorLanguage.PlainText, // built-in highlighter selection
    colors: EditorColors = EditorColors.Default,         // dark
    readOnly: Boolean = false,
    softWrap: Boolean = false,                           // false = horizontal scrolling
    lineNumberMode: LineNumberMode = LineNumberMode.PinnedToScreen,
    symbols: List<EditorSymbol> = DefaultEditorSymbols,  // emptyList() hides the symbol bar
    autoClosePairs: Boolean = true,
    highlighter: SyntaxHighlighter? = null,              // custom plugin; overrides `language`
)
```

> **Insets:** the editor consumes bottom system-bar and IME insets itself, so the keyboard never covers the caret. Do **not** wrap it in `imePadding()` or add bottom navigation-bar padding.

### Opening and saving files

The API is pull-based: observe the snapshot state you care about, pull text when you need it.

```kotlin
// Open / swap documents (resets undo history and the dirty flag):
controller.setDocument(fileContent)

// Save — capture version and text together on the UI thread:
val version = controller.documentVersion
val text = controller.getText(controller.lineEnding)   // CRLF restored if the file had it
withContext(Dispatchers.IO) { file.writeText(text) }
controller.markSaved(version)                          // back on the UI thread

// Autosave guard — composing text is uncommitted IME state:
if (!controller.isComposing) { /* safe to snapshot */ }
```

### Controller reference

Created via `rememberCodeEditorController(initialText)` or `rememberSaveableCodeEditorController(initialText)`. All members are UI-thread only.

| Group | Members |
|---|---|
| Observe (snapshot state) | `selection`, `caret`, `documentVersion`, `isModified`, `isComposing`, `canUndo`, `canRedo`, `lineEnding` |
| Text | `getText()`, `getText(lineEnding)`, `setDocument(text)` |
| Save tracking | `markSaved(version)` |
| Edit (bypass `readOnly`) | `insertText(text)`, `replaceRange(start, end, text)` |
| Caret & selection | `select(start, end)`, `jumpTo(position)`, `jumpToLine(line)` |
| History | `undo()`, `redo()` |
| Built-in bars | `openFind()`, `openReplace()`, `closeFind()`, `isFindVisible`, `isReplaceVisible`, `openGotoLine()`, `closeGotoLine()`, `isGotoLineVisible` |

Positions are `TextPosition(line, column)` — 0-based, column in UTF-16 units; out-of-range values are clamped.

## Syntax highlighting

Highlighters are per-line incremental tokenizers. The editor owns state-chain propagation, caching, and invalidation; a plugin only maps one line + the previous line's exit state to spans:

```kotlin
class MyHighlighter : SyntaxHighlighter {
    override val lineCommentPrefix = "//"          // enables Ctrl+/
    // override val blockComment = BlockComment("<!--", "-->")  // fallback for languages without line comments

    override fun highlightLine(text: String, entryState: LineState?): LineHighlight {
        val i = text.indexOf("//")
        val spans = if (i >= 0) listOf(HighlightSpan(i, text.length, TokenType.Comment)) else emptyList()
        return LineHighlight(spans, exitState = null) // non-null exitState carries multi-line constructs
    }
}
```

```kotlin
CodeEditor(controller, highlighter = remember { MyHighlighter() })
```

`remember` the instance — it keys the highlight and layout caches. Cross-line constructs travel through immutable `LineState` values. The built-in `YamlHighlighter` is the reference implementation.

## Theming

`EditorColors` is a flat `@Immutable` bag: chrome slots (`background`, `cursor`, `selection`, `currentLine`, `bracketMatch`, …) plus `syntax: SyntaxColors`, which maps every `TokenType` to a `TokenStyle`. `EditorColors.Default` (dark) and `EditorColors.Light` ship in the box; likewise `SyntaxColors.Dark` / `.Light`.

## Keyboard shortcuts

| Action | Windows / Linux / Android | macOS |
|---|---|---|
| Select all / Copy / Cut / Paste | Ctrl+A / C / X / V | Cmd+A / C / X / V |
| Undo / Redo | Ctrl+Z / Ctrl+Shift+Z or Ctrl+Y | Cmd+Z / Cmd+Shift+Z |
| Find / Replace | Ctrl+F / Ctrl+H | Cmd+F / Cmd+Opt+F |
| Find next / previous | F3 / Shift+F3 | Cmd+G / Cmd+Shift+G |
| Go to line | Ctrl+G | Cmd+L |
| Toggle comment | Ctrl+/ | Cmd+/ |
| Word left / right | Ctrl+← / Ctrl+→ | Opt+← / Opt+→ |
| Line start / end | Home / End | Cmd+← / Cmd+→ |
| Document start / end | Ctrl+Home / Ctrl+End | Cmd+↑ / Cmd+↓ |
| Page up / down | PageUp / PageDown | PageUp / PageDown |
| Indent / outdent selection | Tab / Shift+Tab | Tab / Shift+Tab |
| Close find / goto bar | Esc | Esc |

## Repository layout

```
editor/            the library (commonMain + androidMain + desktopMain)
sandbox/shared/    demo UI shared across platforms
sandbox/android/   thin Android entry
sandbox/desktop/   thin desktop entry
```

### Running the sandbox

```
./gradlew :sandbox:desktop:run                # desktop demo app
./gradlew :sandbox:android:assembleDebug     # APK at sandbox/android/build/outputs/apk/debug/
```

### Tests

```
./gradlew :editor:desktopTest                 # engine/buffer/highlight/undo/find unit tests
```

## Known limitations

- **No accessibility semantics.** The editor is fully self-drawn and publishes no semantics tree, so screen readers cannot inspect the text.
- **The macOS keymap is implemented by convention** and not yet verified on Apple hardware.
- **No horizontal scrollbar, by design** — the horizontal extent only covers measured lines; drawing a thumb from an underestimated bound would mislead.
- **Documents over 200 000 chars skip instance-state restoration** (Bundle size limit) and fall back to the initial seed text.

## License

[Apache License 2.0](LICENSE)
