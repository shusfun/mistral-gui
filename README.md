# mistral-gui

Go / Wails v3.1.0 桌面本地模型管理器，通过独立的 [mistral.rs](https://github.com/EricLBuehler/mistral.rs) 运行时提供推理及官方 WebUI。管理器、运行时、模型权重分别下载和更新，用户不需要 Node.js、Python 或 Rust 环境。

[下载安装包](https://github.com/shusfun/mistral-gui/releases/latest) · [中文使用指南](docs/usage.zh-CN.md) · [English guide](docs/usage.en.md) · [发行与密钥维护](docs/releases.md) · [Qingyi 插件](https://github.com/shusfun/Qingyi)

## 使用

用户安装文件只有 Windows x64 `mistral-gui-windows-x64-setup.exe`、macOS Intel `mistral-gui-macos-x64.dmg` 和 Apple Silicon `mistral-gui-macos-arm64.dmg`。暂不提供 Linux 发行包。

Windows 安装向导选择中文或 English；macOS 将应用拖到 Applications，首次启动选择语言。打开应用显示桌面窗口，再次启动唤起已有窗口。关闭缩到托盘；明确退出停止全部模型和管理器。默认不开机自启，操作确认使用应用内对话框，错误保留在界面，文件选择与保存使用系统对话框。支持浅色、深色和跟随系统；部署详情、任务与诊断分别组织，编辑中的草稿不会被后台刷新覆盖。

1. 在“设备与后端”检测设备，在“运行时与模型”选择后端。
2. 检查可用资产并下载运行时，来源会标明“官方 Release”或“补充 CI 资产”。
3. 从目录选择 HY-MT、Qwen3、SmolLM3 的兼容 GGUF／safetensors 版本，也可填写自定义仓库或导入本地模型。
4. 启动模型，执行测试推理，然后打开官方 WebUI 或复制 Qingyi 配置。

首次加载还会从 Hugging Face 获取模型的 tokenizer 配置并保存在本机缓存中。模型可用性取决于对应 mistral.rs 版本的 GGUF 支持；启动失败时显示引擎加载原因，不生成模拟译文。

Windows 可选择 CPU 或 CUDA 12.x；Intel Mac 使用 CPU，Apple Silicon 使用 Metal。CUDA 需要兼容的 NVIDIA 显卡及驱动；只有检测到驱动不代表模型一定能装入显存。Auto 在 Windows/Intel Mac 使用 CPU，在 Apple Silicon 使用 Metal。CPU 吞吐取决于硬件与模型，增加并发不会线性增加推理速度。

补充 CUDA 12.x 构建以 SM80（Ampere）为基线，并保留 PTX 供更新架构的驱动编译；较早的 GPU 使用 CPU。不同 GPU 的实际可用性仍需对应设备验证。

补充 Windows CUDA 构建会补上 Marlin 的 `<string>` 引用及上游遗漏的 Windows 条件编译分支，并修正 rotary FFI 的 64 位参数声明，避免 Windows 的 32 位 `long` 截断 CUDA 流句柄。FA3 与分页缓存写入在 Windows 上仍按上游限制处理，不伪装为可用；补丁仅应用于缺失资产的补充构建。

Windows 桌面使用 WebView2，缺失时可下载并验证微软 Evergreen 安装器；macOS 使用系统 WebKit。mistral.rs 官方 Windows 二进制还依赖 Microsoft Visual C++ 运行库，检测页提供微软下载入口，无需开发工具链。

默认只监听 `127.0.0.1:19820`，引擎使用另一个内部回环端口。聊天和会话操作在 mistral.rs 官方 `/ui` 完成，管理器不提供另一套聊天页面。

| 地址 | 用途 |
| --- | --- |
| `/manage` | 安装、下载、运行时及模型切换、停止、更新、语言设置 |
| `/ui` | mistral.rs 官方 WebUI，运行后可用 |
| `/v1/chat/completions` | OpenAI 兼容接口，保留流式响应 |
| `/api/v1/status` | 状态、模型修订、设备后端、任务与更新结果 |
| `/api/v1/models` | 已安装运行时及模型 |
| `/api/v1/runtime/start`、`stop`、`restart` | POST 启停接口 |
| `/api/v1/events` | SSE 任务事件流 |
| `/healthz` | 管理器健康状态与版本 |

API 使用 `Authorization: Bearer TOKEN`。从快捷方式进入管理页时自动建立本机 HttpOnly 会话；手动打开地址时需要输入 Token。管理页支持复制和重新生成 Token。更换后需更新 Qingyi 等客户端。Token 不出现在公开 HTML、状态接口或诊断导出中。

模型按需启动，默认空闲 10 分钟停止；0 表示关闭空闲停止。运行中的请求不会触发空闲停止。停止保留模型文件，“退出管理器”释放管理器与引擎进程。下载失败或取消保留 `.part`，重试时使用 Range 续传，完成后校验可信来源提供的 SHA-256。镜像按设置中的顺序在官方地址之后尝试，格式是 HTTPS 前缀加完整原始 URL；镜像不能改变校验摘要。

同名模型的不同修订分别保存，可在停止后选择具体修订或切回上一修订。GPU 启动失败后可点击“改用 CPU 恢复”；这会选择已安装的 CPU 运行时，缺少时下载，并明确显示降级状态。管理器不会把失败的 GPU 请求伪装成成功。

部署实例具有独立 ID、进程、参数、端口和空闲计时，不限制数量。推荐优先考虑内存预算，再选择加速后端；估算不足可确认后继续。高级设置按已安装引擎的实际 CLI 能力显示。请求的 `model` 使用部署 ID，`/v1/models` 列出运行实例；旧默认模型文件名继续兼容。目录支持搜索、格式和量化筛选，safetensors 快照包括配置、tokenizer 和分片。

应用数据默认保存在 Windows `%APPDATA%\mistral-gui` 或 macOS `~/Library/Application Support/mistral-gui`。卸载保留下载与模型。`--data-dir` 指定数据目录，`--addr` 仅接受回环地址，`--no-open` 使用无窗口服务模式，`--quit` 退出对应管理器。

## Qingyi

在 Qingyi 设置中配置本地管理器地址、Token、模型 ID 和后端。支持本地优先、仅本地、仅云端；本地优先会在本地不可用或某段翻译失败时回退该段。词库和用户修订先查询，命中不会联系管理器。本地结果按模型修订单独缓存，初始上限 256 MiB，自动淘汰不会删除词库、用户修订或云端缓存。

## 自更新与发行

管理器使用 Wails Updater，配合续传 Provider、SHA-256 和固定 Ed25519 公钥验证。Windows 使用 `.wails.update.zip`，macOS 使用 `.update.tar.gz`。Wails 负责替换及重启；独立恢复副本和健康守护处理新实例六十秒内未就绪的回滚。旧 `.update.zip`／`.update.tar.zst` 和签名清单仅供旧协议迁移。macOS 应先复制到可写的 Applications。

v0.2.0 提供新的分区工作台和自绘窗口头部。可通过“更新与设置”检查更新，或下载安装包覆盖升级；配置和模型保留，不随管理器更新重新下载。

`.github/workflows/release.yml` 构建 Windows x64、macOS Intel/Apple Silicon 安装包和机器更新包，生成 `SHA256SUMS`、签名及清单。不会将推理引擎、模型或额外源码压缩包放入管理器资产。GitHub 自动提供的源码下载属于 GitHub 平台功能。

发布仓库需要配置：

- Repository variable `UPDATE_PUBLIC_KEY`：Base64 Ed25519 公钥；secret `UPDATE_SIGNING_KEY`：对应的 Base64 64 字节私钥。私钥仅由维护者在可信设备生成和保存，不提交到代码库。没有密钥的开发构建无法执行自更新。
- 运行时补充资产使用本仓库独立的 `runtime-v上游版本` Release，标记为 prerelease 且不设为 latest，不会进入管理器更新清单。CI 使用本仓库受限的 `GITHUB_TOKEN` 上传，无需跨仓库 Token。
- macOS 正式签名可配置 `MAC_CERT_BASE64`、`MAC_CERT_PASSWORD`、`MAC_SIGN_IDENTITY`、`APPLE_ID`、`APPLE_APP_PASSWORD`、`APPLE_TEAM_ID`。未配置时只做 ad-hoc 签名，系统可能要求手动允许打开。Windows 当前安装包需要维护者在发行前按其证书流程执行 Authenticode 签名。

`.github/workflows/runtime-assets.yml` 手动指定上游版本，先检查该版本官方资产；仅构建缺失的 Intel Mac CPU 和 Windows CUDA，并发布至独立运行时 Release。构建 CUDA 需要 CUDA 工具链，构建本身不要求 GPU。官方下载或元数据失败不会被伪装成“官方缺包”。

本地构建使用 Go 1.26，工具链固定 Go 1.26.10；macOS 需要 Xcode Command Line Tools 和 CGO：

```powershell
go test ./...
go vet ./...
go build -trimpath -ldflags="-s -w -H windowsgui" -o dist/mistral-gui.exe ./cmd/mistral-gui
go build -trimpath -ldflags="-s -w -H windowsgui" -o dist/mistral-gui-updater.exe ./cmd/updater
makensis installer/windows/setup.nsi
go run ./cmd/release -os windows -arch amd64
```

macOS 在对应 runner 上先构建 `dist/mistral-gui` 和 `dist/mistral-gui-updater`，执行 `sh installer/macos/build-dmg.sh amd64` 或 `arm64`，再执行 `go run ./cmd/release -os darwin -arch ARCH`。更新仓库、版本、公钥和补充仓库通过 `internal/app` 包中同名构建变量注入，具体见发行工作流。

管理器代码采用 MIT。mistral.rs 和下载的模型分别遵循上游许可：[运行时](https://github.com/EricLBuehler/mistral.rs/blob/master/LICENSE)、[HY-MT 模型](https://huggingface.co/tencent/HY-MT1.5-1.8B-GGUF)。CUDA 运行库遵循 NVIDIA 对应许可。

## English

A Go/Wails v3.1.0 desktop manager. Closing hides to the tray; explicit Quit stops all engines. Run independent model deployments without an instance-count cap, browse compatible GGUF/safetensors repositories and apply memory recommendations with manual overrides. Inference and chat use the separately downloaded mistral.rs runtime and its official WebUI.

Install the Windows x64 setup EXE or the Intel/Apple Silicon macOS DMG. Choose Chinese or English, detect the device, install a runtime and an HY-MT GGUF model, start it, then open the official WebUI or copy the Qingyi configuration. First load also downloads tokenizer metadata. Windows supports CPU/CUDA 12.x; Intel Macs use CPU and Apple Silicon uses Metal. Linux distributions are not included.

The manager binds to `127.0.0.1:19820`. API clients must supply a Bearer token, available in the console. Models start on demand and stop after ten idle minutes by default. Downloads can be cancelled and resumed, with trusted SHA-256 verification. The manager, runtimes and weights remain separate. Signed manager updates use a helper, a health check and automatic rollback. Development builds without a configured repository/public key cannot self-update.

Qingyi offers local-first, local-only and cloud-only routing. Dictionary/correction hits require no manager call. Local inference results have a separate 256 MiB cache, isolated from cloud results and keyed by model revision.
