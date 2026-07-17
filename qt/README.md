# ClawdSecbot Qt 客户端

这是 Flutter 桌面客户端的 Qt6 Widgets 实现。Qt 只负责 UI、状态和桌面窗口；扫描、风险评估、防护、审计、Skill 安全分析、配置与 SQLite 持久化仍由仓库现有 `go_lib` 提供。

调用链保持为：

```text
Qt Widgets -> C++ GoBridge -> botsec.dylib/.so/.dll -> Go core/service -> Go core/repository -> SQLite
```

## 环境

- CMake 3.21+
- C++20 编译器
- Qt 6.4+：Core、Gui、Widgets、Concurrent、Test
- Go 1.25.5+

当前 Homebrew Qt 6.11 的 macOS 最低运行版本为 14.0，工程默认与该依赖保持一致；使用其他 Qt 发行版时可通过 `-DAPP_MACOS_MINIMUM_VERSION=...` 调整。

macOS 使用 Homebrew Qt 时，可显式传入：

```bash
-DCMAKE_PREFIX_PATH="$(brew --prefix qt)"
```

## 构建与测试

```bash
cmake -S qt -B qt/build/debug \
  -DCMAKE_BUILD_TYPE=Debug \
  -DCMAKE_PREFIX_PATH="$(brew --prefix qt)"
cmake --build qt/build/debug --parallel
ctest --test-dir qt/build/debug --output-on-failure
```

CMake 会用 `-buildmode=c-shared` 构建现有 `go_lib`，并将 `botsec` 动态库复制到应用运行目录；macOS 会放入 `ClawdSecbot.app/Contents/Resources/plugins/`。

## 一步启动与 pprof

```bash
./qt/scripts/run_with_pprof.sh
./qt/scripts/run_with_pprof.sh 9090 business
```

参数语义与 Flutter 的 `scripts/run_with_pprof.sh` 一致：第一个参数是 pprof 端口，第二个参数是 `community` 或 `business`。

## 目录

```text
qt/
  CMakeLists.txt
  scripts/
  src/
    app/       应用生命周期、托盘与顶层装配
    bridge/    C++ 到现有 Go c-shared 动态库的 JSON FFI
    common/    C++ 日志
    core/      路径与运行配置
    domain/    Qt 侧只读 UI 数据模型
    service/   扫描、防护、审计等可复用工作流
    ui/        主窗口、独立窗口、弹窗、QSS 和通用控件
  tests/
```

界面清单和 Flutter 对照关系见 [FLUTTER_UI_PARITY.md](FLUTTER_UI_PARITY.md)。

Qt 工程版本在 CMake 配置阶段从仓库根目录 `pubspec.yaml` 读取，数据库初始化和应用构建共用同一个版本号，避免迁移版本漂移。

当前 Qt 界面仅提供中文，不展示无法实际切换界面的伪语言入口。后续若增加语言，需要先把界面字符串迁移到 Qt 翻译资源，再恢复语言选择器。
