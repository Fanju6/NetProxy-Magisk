# scripta

**简体中文** | [English](README_EN.md)

一个为 [Compose Multiplatform](https://github.com/JetBrains/compose-multiplatform) 打造的自绘、虚拟化代码编辑器，支持 Android 与桌面端（JVM）。

scripta 不是 `BasicTextField` 的包装：piece-tree 文本缓冲、视口虚拟化渲染、手势/选区/撤销系统、两端自管的 IME 会话，整条栈都是自己的——大文档因此流畅，编辑器行为因此完全可控。

**状态：预发布。** Android 为主平台，桌面端同步维护；尚未发布到 Maven Central。

## 亮点

- **虚拟化渲染** —— 只测量、只绘制可见行；逐行版式 LRU 缓存，按编辑范围增量失效。
- **piece-tree 缓冲** —— O(log n) 的编辑与 offset↔(行, 列) 映射，开销与文档大小无关。
- **超长行快速路径** —— 横向滚动模式下，超过 2 000 字符的可打印 ASCII 行只测量可见列窗口、几何走等宽算术。
- **自管 IME** —— Android 用自定义 `InputConnection` 直通引擎；桌面给 CJK 输入法实时文档视图，候选窗锚定光标。
- **触屏与桌面输入都是一等公民** —— 触屏有选区手柄、拖拽放大镜、双指缩放；桌面有完整鼠标与硬件键盘支持——按*输入类型*而非平台区分。
- **增量语法高亮** —— 逐行插件接口 + 跨行状态链；内置 YAML，自定义语言约 50 行。
- **拉取式宿主 API** —— `@Stable` 控制器暴露快照状态与命令式调用，没有逐键回调。

## 功能一览

### 编辑
- 撤销/重做带键入单元合并：连续键入一步撤销、IME 组合折叠为净效果；上限 1 000 单元。
- 回车自动缩进。
- Tab / Shift+Tab 对选区整块缩进/反缩进，一个撤销单元。
- 括号/引号自动配对、闭合符跳过、成对退格——引号带意图启发式，`it's` 不会变成 `it''s`；`autoClosePairs = false` 可关。
- 光标相邻括号的匹配高亮。
- Ctrl+/ 注释切换：行注释最小缩进列对齐、混合状态加层可逆；无行注释的语言回退块注释。元数据来自高亮插件。
- 软键盘底部符号快捷条。

### 导航、查找、选区
- 停靠条式查找替换：大小写 / 全词 / 正则开关，Enter/Shift+Enter 循环跳转，全部替换一步撤销。
- 跳转行号停靠条（Ctrl+G）。
- 按词移动、Home/End/PageUp/PageDown、软换行下按视觉行上下移动，导航键全支持 Shift 扩选。
- 编程式跳转永远把光标滚回视野——连续两次跳到同一位置也一样。

### 触屏
- 单击立即落光标；点在光标上弹出粘贴气泡；双击选词；长按选词并按词拖拽扩选。
- 光标/选区手柄 + "液态玻璃"拖拽放大镜。
- 拖拽手柄时四向边缘自动滚动。
- 双指自由缩放，松手折算进滚动位置与字号（8–40 sp）。
- 纵向淡出滚动条，可抓取 thumb 快滚。

### 鼠标与硬件键盘
- 多连击升级字符 → 词 → 行选择粒度，Shift+点击扩选，悬停 I 形光标，滚轮每格 3 行。
- 右键上下文菜单，桌面与 Android 外接鼠标均可用。
- 完整快捷键集见下表，macOS 自动切键位；AltGr 组合不会被误读。
- 鼠标交互自动隐去触屏部件；手写笔仍走触屏行为。

### 文档
- CRLF 保真：内部统一 LF，加载时检测主导换行符并全程保持，`getText(lineEnding)` 保存时还原。
- `rememberSaveableCodeEditorController` 挺过旋转与进程死亡。
- 只读模式保留滚动、缩放、选区、复制与查找。

## 平台支持

| | Android | 桌面（JVM） |
|---|---|---|
| 最低版本 | API 24 | JDK 21 工具链 |
| IME / CJK 组合输入 | 自管 `InputConnection` | 文本输入会话，候选窗锚定光标 |
| 放大镜玻璃着色器 | Android 13+（AGSL；低版本描边） | skiko `RuntimeEffect` |
| 键位映射 | Ctrl 系 | Ctrl 系，macOS 自动切 Cmd/Option |
| 滚动条让出返回手势 | ✔ | 不适用 |

Android 是主平台；桌面目标保证共享逻辑的多平台成色。

## 快速开始

scripta 尚未发布到 Maven Central，请以 Gradle 复合构建引入：

```kotlin
// settings.gradle.kts（scripta 已检出或作为 git submodule）
includeBuild("third_party/scripta")
```

```kotlin
// build.gradle.kts —— 自动替换为被包含构建的 :editor
dependencies {
    implementation("scripta:editor")
}
```

复合构建保住 scripta 自己的版本目录与插件集；你的 Kotlin/AGP/Compose 版本需与之对齐。库唯一的外部依赖是 `org.jetbrains.compose.foundation:foundation`。本仓库工具链：Kotlin 2.4.0、Compose Multiplatform 1.11.1、AGP 9.2.1、Gradle 9.6.1、JDK 21。

### 最小用法

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

完整参数面：

```kotlin
@Composable
fun CodeEditor(
    controller: CodeEditorController,
    modifier: Modifier = Modifier,
    language: EditorLanguage = EditorLanguage.PlainText, // 内置高亮选择
    colors: EditorColors = EditorColors.Default,         // 深色
    readOnly: Boolean = false,
    softWrap: Boolean = false,                           // false = 横向滚动
    lineNumberMode: LineNumberMode = LineNumberMode.PinnedToScreen,
    symbols: List<EditorSymbol> = DefaultEditorSymbols,  // emptyList() 隐藏符号条
    autoClosePairs: Boolean = true,
    highlighter: SyntaxHighlighter? = null,              // 自定义插件；优先于 language
)
```

> **Insets 注意：** 编辑器自行消费底部系统栏与 IME insets，键盘不会盖住光标。**不要**再包 `imePadding()` 或加底部导航栏 padding。

### 打开与保存文件

API 是拉取式的：观察你关心的快照状态，需要时再拉取文本。

```kotlin
// 打开 / 切换文档（重置撤销历史与脏标记）：
controller.setDocument(fileContent)

// 保存——在 UI 线程同时取版本号与文本：
val version = controller.documentVersion
val text = controller.getText(controller.lineEnding)   // 原文件是 CRLF 就还原 CRLF
withContext(Dispatchers.IO) { file.writeText(text) }
controller.markSaved(version)                          // 回到 UI 线程调用

// 自动保存守卫——组合文本是 IME 未提交状态：
if (!controller.isComposing) { /* 可以安全快照 */ }
```

### 控制器速查

经 `rememberCodeEditorController(initialText)` 或 `rememberSaveableCodeEditorController(initialText)` 创建。所有成员仅限 UI 线程调用。

| 分组 | 成员 |
|---|---|
| 观察（快照状态） | `selection`、`caret`、`documentVersion`、`isModified`、`isComposing`、`canUndo`、`canRedo`、`lineEnding` |
| 文本 | `getText()`、`getText(lineEnding)`、`setDocument(text)` |
| 保存追踪 | `markSaved(version)` |
| 编辑（绕过 `readOnly`） | `insertText(text)`、`replaceRange(start, end, text)` |
| 光标与选区 | `select(start, end)`、`jumpTo(position)`、`jumpToLine(line)` |
| 历史 | `undo()`、`redo()` |
| 内置工具条 | `openFind()`、`openReplace()`、`closeFind()`、`isFindVisible`、`isReplaceVisible`、`openGotoLine()`、`closeGotoLine()`、`isGotoLineVisible` |

位置类型 `TextPosition(line, column)`：0 基，列为 UTF-16 单位；越界自动钳制。

## 语法高亮

高亮插件是逐行增量分词器。状态链推进、缓存、失效全由编辑器承担；插件只需把"一行文本 + 上一行的退出状态"映射为着色段：

```kotlin
class MyHighlighter : SyntaxHighlighter {
    override val lineCommentPrefix = "//"          // 启用 Ctrl+/
    // override val blockComment = BlockComment("<!--", "-->")  // 无行注释语言的回退

    override fun highlightLine(text: String, entryState: LineState?): LineHighlight {
        val i = text.indexOf("//")
        val spans = if (i >= 0) listOf(HighlightSpan(i, text.length, TokenType.Comment)) else emptyList()
        return LineHighlight(spans, exitState = null) // 非空 exitState 承载跨行结构
    }
}
```

```kotlin
CodeEditor(controller, highlighter = remember { MyHighlighter() })
```

记得 `remember` 实例——它是高亮与版式缓存的键。跨行结构经不可变的 `LineState` 值在行间传递。内置的 `YamlHighlighter` 就是参考实现。

## 主题

`EditorColors` 是扁平的 `@Immutable` 配色袋：部件色槽（`background`、`cursor`、`selection`、`currentLine`、`bracketMatch`……）加 `syntax: SyntaxColors`，把每个 `TokenType` 映射到 `TokenStyle`。内置 `EditorColors.Default`（深色）与 `EditorColors.Light`，`SyntaxColors.Dark` / `.Light` 同理。

## 快捷键

| 操作 | Windows / Linux / Android | macOS |
|---|---|---|
| 全选 / 复制 / 剪切 / 粘贴 | Ctrl+A / C / X / V | Cmd+A / C / X / V |
| 撤销 / 重做 | Ctrl+Z / Ctrl+Shift+Z 或 Ctrl+Y | Cmd+Z / Cmd+Shift+Z |
| 查找 / 替换 | Ctrl+F / Ctrl+H | Cmd+F / Cmd+Opt+F |
| 下一个 / 上一个匹配 | F3 / Shift+F3 | Cmd+G / Cmd+Shift+G |
| 跳转行号 | Ctrl+G | Cmd+L |
| 注释切换 | Ctrl+/ | Cmd+/ |
| 按词左移 / 右移 | Ctrl+← / Ctrl+→ | Opt+← / Opt+→ |
| 行首 / 行尾 | Home / End | Cmd+← / Cmd+→ |
| 文档头 / 文档尾 | Ctrl+Home / Ctrl+End | Cmd+↑ / Cmd+↓ |
| 上翻页 / 下翻页 | PageUp / PageDown | PageUp / PageDown |
| 选区缩进 / 反缩进 | Tab / Shift+Tab | Tab / Shift+Tab |
| 关闭查找 / 跳转条 | Esc | Esc |

## 仓库结构

```
editor/            库本体（commonMain + androidMain + desktopMain）
sandbox/shared/    跨平台示例 UI
sandbox/android/   Android 薄入口
sandbox/desktop/   桌面薄入口
```

### 运行示例应用

```
./gradlew :sandbox:desktop:run                # 桌面示例
./gradlew :sandbox:android:assembleDebug     # APK 在 sandbox/android/build/outputs/apk/debug/
```

### 测试

```
./gradlew :editor:desktopTest                 # 引擎/缓冲/高亮/撤销/查找单元测试
```

## 已知限制

- **无无障碍语义。** 编辑器完全自绘、不发布语义树，屏幕阅读器读不到文本。
- **macOS 键位按约定实现**，尚未在 Apple 硬件上实机验证。
- **有意不画横向滚动条**——横向上界只来自已测量过的行，按低估的上界画 thumb 是误导。
- **超 200 000 字符的文档跳过实例状态恢复**（Bundle 上限），回退到初始种子文本。

## 许可证

[Apache License 2.0](LICENSE)
