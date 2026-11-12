# Using mistral-gui

Download one installer from the [latest manager release](https://github.com/shusfun/mistral-gui/releases/latest):

| System | Installer |
| --- | --- |
| Windows x64 | `mistral-gui-windows-x64-setup.exe` |
| Intel Mac | `mistral-gui-macos-x64.dmg` |
| Apple Silicon Mac | `mistral-gui-macos-arm64.dmg` |

Choose Chinese or English in the Windows wizard, or on first launch on macOS. Copy the Mac app into Applications before opening it. Language can also be changed in the console. Linux distributions are not provided.

OS code signing and Ed25519 update signing are different. Builds without Authenticode or Apple Developer certificates can trigger an unknown-developer warning. Check the repository origin and the published SHA-256 sums.

## First launch

1. Open the Wails desktop application. Browser management remains available at `http://127.0.0.1:19820/manage`.
2. In Deployments, select a downloaded model or choose one from the model catalog, then click **Deploy and start**. Download a model first when needed; local model files and custom repositories are also supported.
3. The manager prepares and verifies the model and matching inference engine, saves the deployment, starts it and checks API availability. The deployment card shows progress and offers cancel or retry. Installed resources are reused; a GPU startup failure offers an explicit CPU retry.
4. When it is running, copy the OpenAI Base URL, Model ID and token, or test the API from the card. The default Base URL is `http://127.0.0.1:19820/v1`.

Windows supports CPU and supplemental CUDA 12.x builds for NVIDIA Ampere / SM80 and newer architectures with a compatible driver. Intel Macs use CPU; Apple Silicon uses Metal. Auto selects CPU on Windows/Intel Macs and Metal on Apple Silicon. No Rust, Node.js or Python installation is required for users. Official Windows runtimes may require the Microsoft VC++ runtime; device detection provides its official download link if missing.

The manager, runtime and model weights remain separate. Interrupted downloads retain partial files for resume and verify SHA-256 before installation. Tasks and errors survive a page refresh. GPU failures offer an explicit CPU recovery action.

## Qingyi and API clients

Install [Qingyi](https://github.com/shusfun/Qingyi). Copy the address, token and model ID from the manager's API page into extension settings. Use local-first, local-only or cloud-only routing. Dictionary and correction hits do not start the model. Local model results use a separate 256 MiB cache, partitioned by model revision. Local-only never falls back to a cloud translation service.

Other clients use `http://127.0.0.1:19820/v1` and `Authorization: Bearer TOKEN`. Keep the token private. `/ui` provides upstream chat; `/manage` controls installation, runtime and updates.

Models start on demand and stop after ten idle minutes by default. In-flight requests prevent idle shutdown. Stopping retains downloaded weights; quitting stops both the manager and its engine.

Closing hides to the tray; explicit Quit stops all instances. Opening the app again focuses its existing window. Deployments can be started, stopped and tested independently; preparing one does not change or stop another. Advanced controls are optional. Qingyi can refresh the deployment list; use the copied deployment ID as the OpenAI model value.

Manager updates use Wails Updater with resumable downloads, pinned Ed25519 verification and native confirmations. A separate recovery copy protects against a newly launched app failing its health check. Same-version overwritten releases require one manual installer replacement; subsequent updates use Wails. Runtime and model assets remain separate.

Data lives in `%APPDATA%\mistral-gui` on Windows or `~/Library/Application Support/mistral-gui` on macOS. Uninstallation preserves data. `--data-dir` selects an isolated data directory. Inspect task errors and redacted logs for missing runtime libraries, incompatible drivers, insufficient memory or network failures.
