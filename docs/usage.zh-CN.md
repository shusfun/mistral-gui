# 使用 mistral-gui

从 [最新管理器 Release](https://github.com/shusfun/mistral-gui/releases/latest) 下载与你的系统对应的一个安装包：

| 系统 | 文件 |
| --- | --- |
| Windows x64 | `mistral-gui-windows-x64-setup.exe` |
| Intel Mac | `mistral-gui-macos-x64.dmg` |
| Apple Silicon Mac | `mistral-gui-macos-arm64.dmg` |

Windows 安装向导先选择中文或 English。Mac 将 `.app` 拖到 Applications 后打开，首次运行选择语言。控制台也可随时切换语言。暂不支持 Linux。

没有 Authenticode / Apple Developer 证书的发行包可能被系统提示未识别开发者；Ed25519 更新签名不替代操作系统代码签名。请核对仓库来源和 Release 中的 SHA-256。

## 第一次使用

1. 启动应用打开桌面窗口；浏览器管理入口仍为 `http://127.0.0.1:19820/manage`。
2. 在“设备与后端”查看 CPU、GPU、内存和 Windows 运行库状态。
3. 选择后端并检查运行时资产。官方有对应资产时优先使用官方版本；缺失目标才使用标注“补充 CI 资产”的版本。
4. 下载运行时及模型。目录覆盖 HY-MT、Qwen3、SmolLM3 的兼容 GGUF／safetensors 版本，也可填写自定义仓库或导入本地文件。safetensors 下载配置、tokenizer 和完整权重；HY-MT GGUF 首次加载还需 tokenizer 元数据。
5. 启动模型，执行测试请求，再打开官方 WebUI。

Windows CPU 不需要显卡；CUDA 12.x 补充资产面向 NVIDIA Ampere / SM80 或更新架构，需要兼容驱动。Intel Mac 使用 CPU，Apple Silicon 使用 Metal。Auto 在 Windows/Intel Mac 选择 CPU，在 Apple Silicon 选择 Metal。

引擎、权重和管理器分别下载。取消或断网保留 `.part`，重试尝试续传，完成后校验 SHA-256。后台任务和失败原因在刷新页面后保留。启动 GPU 失败可明确选择“改用 CPU 恢复”。

## 配合 Qingyi

安装 [Qingyi](https://github.com/shusfun/Qingyi)，在管理器的“API 与诊断”复制配置，再在插件设置中填入：

- 本地地址：`http://127.0.0.1:19820`
- Token：从控制台复制；这是私密凭证，不要放入截图、仓库或共享配置。
- 模型 ID：使用部署 ID，插件设置页支持刷新和选择部署，也可手动填写；旧默认模型文件名继续兼容。
- 后端：与管理器选择一致，或选择 Auto。
- 路由：本地优先、仅本地、仅云端。

词库和用户修订优先命中，不启动模型。本地缓存按模型修订隔离，默认 256 MiB。仅本地不会把网页内容回退到云端；本地优先会对失败段落使用你配置的云端服务。

第三方客户端可使用 `http://127.0.0.1:19820/v1` 和 `Authorization: Bearer TOKEN`。`/ui` 是 mistral.rs 官方聊天界面，`/manage` 仅负责运行时与模型管理。

## 启停、更新和数据

默认允许 API 请求按需启动模型，并在空闲 10 分钟后停止。停止模型释放引擎进程，保留权重。退出管理器同时停止其引擎。进行中的请求不会被空闲停止打断。

关闭窗口只缩到托盘，明确退出才停止全部进程。再次打开应用唤起现有窗口。在“部署实例”填写 ID 和模型，自动推荐后可手动调整。每个部署独立启停，不限制数量；资源警告可确认后继续。停止后可编辑或删除配置，删除配置不会删除权重。

“更新与设置”使用 Wails Updater，应用内对话框确认下载和重启；Wails 校验、解包及替换，独立健康守护在新实例未就绪时回滚。`runtime-*` 独立下载，机器更新包均非首次安装包。同版本旧包需下载安装包覆盖一次，之后使用 Wails 更新通道，数据和模型保留。

数据位置：Windows `%APPDATA%\mistral-gui`；macOS `~/Library/Application Support/mistral-gui`。卸载保留数据。使用 `--data-dir` 可创建独立实例；只在退出应用并备份后手动清理数据目录。

启动失败时查看任务中的真实错误和脱敏日志；常见原因包括下载未完成、VC++ 运行库缺失、显卡架构或驱动不兼容、显存不足，以及 Hugging Face 网络访问失败。
