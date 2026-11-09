# 发行维护

## Wails 桌面更新

使用 Wails v3.1.0 / Go 1.26.10。Windows `.wails.update.zip` 仅含 EXE，macOS `.update.tar.gz` 仅含完整 `.app` 并保留可执行位。各更新包有独立 `.sig`：用现有 Ed25519 密钥签署原始 SHA-256 摘要字节，以 Base64 文本保存。`SHA256SUMS` 包括安装包、新更新包和旧协议包。

旧 `.update.zip`／`.update.tar.zst` 保留原布局及签名 `release-manifest.json`，供旧协议迁移。Wails Provider 精确匹配平台文件，独立运行时不会进入管理器更新。原生确认后才下载与重启；独立健康守护在 `updates/recovery` 保留恢复副本并检测新进程就绪，不依赖 Wails 临时备份。

覆盖现有 Release 时手动运行工作流且 `publish=false`，先取得 `manager-release-complete` 全平台产物，保存现有资产及标签指向，再将标签关联到最终提交。统一替换包，最后发布签名清单及摘要。同版本旧客户端需下载安装包覆盖一次，之后使用 Wails 更新。

管理器仓库：[shusfun/mistral-gui](https://github.com/shusfun/mistral-gui)。配套插件：[shusfun/Qingyi](https://github.com/shusfun/Qingyi)。两者各自版本、各自 Release；只通过 localhost OpenAI 兼容 API 配合使用。

## 管理器通道

推送 `vX.Y.Z` tag 触发 `.github/workflows/release.yml`。Windows x64、Intel Mac、Apple Silicon Mac 原生 runner 分别构建。公开的首次安装包只有 EXE 和两种 DMG。其他资产为机器更新包、`release-manifest.json`、`SHA256SUMS` 及 Ed25519 签名。

仓库变量 `UPDATE_PUBLIC_KEY` 保存 Base64 Ed25519 公钥，Actions secret `UPDATE_SIGNING_KEY` 保存对应的 Base64 64 字节私钥。公钥编译进管理器；私钥仅用于签署发布元数据。私钥不能放到 Git、Release、issue 或聊天中。请将初次生成的密钥在受控存储中备份；密钥丢失会影响已安装客户端的更新信任链。

本机发行密钥保存在用户配置目录下的 `.config/mistral-gui-release`，不在项目中。更换发行设备时使用加密渠道迁移；不要重新生成公钥后直接替换现有通道。

Apple Developer 签名、公证为可选配置：`MAC_CERT_BASE64`、`MAC_CERT_PASSWORD`、`MAC_SIGN_IDENTITY`、`APPLE_ID`、`APPLE_APP_PASSWORD`、`APPLE_TEAM_ID`。没有证书时为 ad-hoc 签名，不声称通过 Gatekeeper 公证。Windows Authenticode 证书需要维护者另行配置，与更新签名密钥不同。

## 独立运行时通道

运行 `.github/workflows/runtime-assets.yml` 并指定精确上游 tag（例如 `v0.9.5`）。先查询 EricLBuehler/mistral.rs 的同版本资产，只补缺失目标。当前补充目标为 Intel Mac CPU、Windows CUDA 12.x。官方下载或元数据错误直接失败，不以补充资产隐藏错误。

补充资产发布到本仓库 `runtime-v0.9.5` 形式的独立 Release，标记 prerelease 且 `latest=false`。管理器更新不会读取这些 Release。补充资产不进入管理器安装包或管理器更新包；模型权重继续从独立 Hugging Face 模型仓库下载。

目录查询先用未加前缀的上游版本，再在确实缺包时查询 `runtime-` 前缀通道；GUI 中保留实际上游版本及官方/补充来源。CI 使用本仓库 `GITHUB_TOKEN`，不需要长期跨仓库上传凭证。

Windows 补充构建包含 v0.9.5 的小范围兼容修复和 64 位 FFI 类型检查。CUDA 内核基线 SM80，附带 PTX；CI 无显卡时只能编译与检查类型，GPU 推理仍需有兼容显卡的设备。构建使用通用 x86-64 CPU 目标，不把 runner 的 native CPU 指令集作为用户设备要求。

更新协议、首次安装资产与运行时资产的命名不可随意更改。发布失败时保留失败的 Actions 记录，修复后重跑；不要上传未经校验的手工替代文件。
