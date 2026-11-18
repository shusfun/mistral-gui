import { useEffect, useRef, useState } from "preact/hooks";
import {
  Badge,
  Button,
  ConfirmHost,
  IconBox,
  IconCpu,
  IconLayoutGrid,
  IconListChecks,
  IconServer,
  IconSettings,
  IconTerminal,
  IconTranslate,
  StateBlock,
  ToastViewport,
  applyTheme,
  resolveError,
  toast,
} from "./shared/design-system";
import {
  ApiError,
  api,
  consumeUrlExchangeCode,
  initDesktopBridge,
  isDesktopMode,
  nativeCommand,
  onAuthRequired,
} from "./api";
import { Brand } from "./components/Brand";
import { LanguageModal, LoginModal } from "./components/Modals";
import { Titlebar } from "./components/Titlebar";
import {
  formatOpKind,
  normalizeManagerLocale,
  t,
  toUiLang,
  type ManagerLocale,
} from "./i18n";
import { ActivityPage } from "./pages/ActivityPage";
import { ConnectionPage } from "./pages/ConnectionPage";
import { DeploymentsPage } from "./pages/DeploymentsPage";
import { DevicePage } from "./pages/DevicePage";
import { ModelsPage } from "./pages/ModelsPage";
import { OverviewPage } from "./pages/OverviewPage";
import { SettingsPage, type UpdateCheckView } from "./pages/SettingsPage";
import { TranslatePage } from "./pages/TranslatePage";
import type {
  CatalogAsset,
  DeviceInfoResponse,
  OperationItem,
  RouteId,
  StatusResponse,
} from "./types";

const VALID_ROUTES = new Set<RouteId>([
  "overview",
  "translate",
  "instances",
  "models",
  "device",
  "connection",
  "activity",
  "updates",
]);

function parseHashRoute(hash: string): { route: RouteId; subPath: string } {
  const cleaned = hash.replace(/^#/, "").split("?")[0].trim();
  if (!cleaned || cleaned.startsWith("exchange=")) {
    return { route: "overview", subPath: "" };
  }
  const [first, ...rest] = cleaned.split("/");
  const route = VALID_ROUTES.has(first as RouteId)
    ? (first as RouteId)
    : "overview";
  return {
    route,
    subPath: rest.length > 0 ? decodeURIComponent(rest.join("/")) : "",
  };
}

export function App() {
  const [routeState, setRouteState] = useState(() =>
    parseHashRoute(window.location.hash),
  );
  const [data, setData] = useState<StatusResponse | null>(null);
  const [installedModels, setInstalledModels] = useState<CatalogAsset[]>([]);
  const [deviceInfo, setDeviceInfo] = useState<DeviceInfoResponse | null>(null);
  const [locale, setLocale] = useState<ManagerLocale>(() =>
    normalizeManagerLocale(navigator.language),
  );

  const [initialLoading, setInitialLoading] = useState(true);
  const [fatalLoadError, setFatalLoadError] = useState<string | null>(null);
  const [loginOpen, setLoginOpen] = useState(false);
  const [authDismissed, setAuthDismissed] = useState(false);
  const [managerStopped, setManagerStopped] = useState(false);

  const [trayUpdateCheck, setTrayUpdateCheck] = useState<{
    result?: UpdateCheckView;
    error?: string;
  } | null>(null);
  const [pendingScreenshot, setPendingScreenshot] = useState<string | null>(
    null,
  );

  // M-39: Keep user's optimistic theme preview until settings reload confirms it
  const themeOverrideRef = useRef<"system" | "light" | "dark" | null>(null);
  const sseRef = useRef<EventSource | null>(null);
  const seenCompletedOpsRef = useRef<Set<string> | null>(null);

  const uiLang = toUiLang(locale);
  const en = uiLang === "en";

  const navigateHash = (hash: string) => {
    window.location.hash = hash;
    setRouteState(parseHashRoute(hash));
  };

  const navigateRoute = (route: RouteId) => {
    navigateHash(`#${route}`);
  };

  const detectDevice = async () => {
    try {
      const dev = await api<DeviceInfoResponse>("/api/v1/devices");
      setDeviceInfo(dev);
    } catch {
      // non-fatal
    }
  };

  const loadModelsList = async (trace?: string) => {
    try {
      const res = await api<{ models: CatalogAsset[] }>("/api/v1/models", undefined, { trace });
      setInstalledModels(
        (res.models || []).filter((m) => m.kind === "model"),
      );
    } catch {
      // non-fatal
    }
  };

  const handleIncomingOperations = (
    ops: OperationItem[],
    loc: ManagerLocale,
  ) => {
    if (!seenCompletedOpsRef.current) {
      seenCompletedOpsRef.current = new Set(
        ops
          .filter((o) => o.state !== "running")
          .map((o) => `${o.id}:${o.state}`),
      );
      return;
    }
    for (const op of ops) {
      if (op.state === "running") continue;
      const key = `${op.id}:${op.state}`;
      if (seenCompletedOpsRef.current.has(key)) continue;
      seenCompletedOpsRef.current.add(key);

      if (op.state === "success") {
        const msg =
          loc === "en-US"
            ? `${formatOpKind(loc, op.kind)} completed${op.asset_id ? `: ${op.asset_id}` : ""}.`
            : `${formatOpKind(loc, op.kind)}已完成${op.asset_id ? `：${op.asset_id}` : ""}。`;
        if (isDesktopMode) {
          void nativeCommand("task-notification", op.id, op.trace_id)
            .then((sent) => {
              if (!sent) toast(msg, "success");
            })
            .catch(() => toast(msg, "success"));
        } else if (!document.hidden) {
          toast(msg, "success");
        }
        void loadModelsList();
      }
    }
  };

  const loadStatus = async (trace?: string): Promise<StatusResponse | null> => {
    const st = await api<StatusResponse>("/api/v1/status", undefined, {
      suppressAuthTrigger: true,
      trace,
    });
    setData(st);
    setFatalLoadError(null);
    setAuthDismissed(false);
    setLoginOpen(false);

    if (st.settings.language) {
      const nextLoc = normalizeManagerLocale(st.settings.language);
      setLocale(nextLoc);
      document.documentElement.lang = nextLoc;
      handleIncomingOperations(st.operations || [], nextLoc);
    } else {
      handleIncomingOperations(st.operations || [], locale);
    }

    if (!themeOverrideRef.current) {
      applyTheme(st.settings.theme || "system");
    }
    return st;
  };

  const startLiveStream = () => {
    sseRef.current?.close();
    const es = new EventSource("/api/v1/events");
    sseRef.current = es;
    es.addEventListener("status", (ev: MessageEvent) => {
      try {
        const payload = JSON.parse(ev.data) as {
          status: StatusResponse["status"];
          operations: OperationItem[];
          deployments: StatusResponse["deployments"];
        };
        setData((prev) => {
          if (!prev) return prev;
          const loc = prev.settings.language
            ? normalizeManagerLocale(prev.settings.language)
            : locale;
          handleIncomingOperations(payload.operations || [], loc);
          return {
            ...prev,
            status: payload.status,
            operations: payload.operations || [],
            deployments: payload.deployments || prev.deployments,
          };
        });
      } catch {
        // ignore malformed SSE frame
      }
    });
  };

  // Full initialization (runs on startup AND after Token login — M-04)
  const initializeSession = async () => {
    setInitialLoading(true);
    try {
      const redirectedHash = await consumeUrlExchangeCode().catch(() => null);
      if (redirectedHash) {
        setRouteState(parseHashRoute(redirectedHash));
      }
      const st = await loadStatus();
      if (st) {
        await Promise.all([loadModelsList(), detectDevice()]);
        await initDesktopBridge(st.platform, {
          onUpdateChecked: (payload) => {
            setRouteState({ route: "updates", subPath: "" });
            setTrayUpdateCheck({ ...payload });
          },
          onScreenshotCaptured: (payload) => {
            if (payload.image_base64) {
              setPendingScreenshot(payload.image_base64);
              navigateHash("#translate");
            } else if (payload.error) {
              toast(
                resolveError(payload.error, toUiLang(st.settings.language))
                  .message,
                "error",
              );
            }
          },
        });
        startLiveStream();
      }
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setLoginOpen(true);
      } else {
        setFatalLoadError(resolveError(err, uiLang).message);
      }
    } finally {
      setInitialLoading(false);
    }
  };

  useEffect(() => {
    if (isDesktopMode) {
      document.documentElement.classList.add("desktop");
    }
    onAuthRequired(() => {
      sseRef.current?.close();
      sseRef.current = null;
      if (!authDismissed) {
        setLoginOpen(true);
      }
    });

    const onHash = () => setRouteState(parseHashRoute(window.location.hash));
    window.addEventListener("hashchange", onHash);
    void initializeSession();

    const timer = setInterval(() => {
      if (document.hidden || loginOpen || authDismissed || managerStopped) {
        return;
      }
      void loadStatus().catch((err) => {
        if (err instanceof ApiError && err.status === 401) {
          sseRef.current?.close();
          setLoginOpen(true);
        }
      });
    }, 12000);

    return () => {
      window.removeEventListener("hashchange", onHash);
      clearInterval(timer);
      sseRef.current?.close();
    };
  }, [authDismissed, loginOpen, managerStopped]);

  if (managerStopped) {
    return (
      <>
        <Titlebar locale={locale} platform={data?.platform || ""} showBrand />
        <div style={{ maxWidth: "560px", margin: "96px auto", padding: "20px" }}>
          <StateBlock
            kind="empty"
            title={en ? "mistral Has Stopped" : "mistral 后台服务已安全退出"}
            message={
              en
                ? "All local inference engines have been unloaded and background polling is closed. Relaunch mistral anytime to reconnect."
                : "所有本地推理引擎均已停止卸载。再次启动 mistral 后点击下方重新连接即可。"
            }
            retryLabel={en ? "Reconnect" : "重新连接"}
            onRetry={() => {
              setManagerStopped(false);
              void initializeSession();
            }}
            lang={uiLang}
          />
        </div>
      </>
    );
  }

  const runningOps = (data?.operations || []).filter(
    (o) => o.state === "running",
  ).length;

  const navIcon = (id: RouteId) => {
    switch (id) {
      case "overview":
        return <IconLayoutGrid size={16} />;
      case "translate":
        return <IconTranslate size={16} />;
      case "instances":
        return <IconServer size={16} />;
      case "models":
        return <IconBox size={16} />;
      case "device":
        return <IconCpu size={16} />;
      case "connection":
        return <IconTerminal size={16} />;
      case "activity":
        return <IconListChecks size={16} />;
      case "updates":
        return <IconSettings size={16} />;
    }
  };

  const navItems: { id: RouteId; label: string; badge?: number }[] = [
    { id: "overview", label: t(locale, "navOverview") },
    { id: "translate", label: t(locale, "navTranslate") },
    {
      id: "instances",
      label: t(locale, "navInstances"),
      badge: data?.deployments.filter((d) => d.status.state === "running")
        .length,
    },
    { id: "models", label: t(locale, "navModels") },
    { id: "device", label: t(locale, "navDevice") },
    { id: "connection", label: t(locale, "navConnection") },
    { id: "activity", label: t(locale, "navActivity"), badge: runningOps },
    { id: "updates", label: t(locale, "navUpdates") },
  ];

  return (
    <>
      <Titlebar locale={locale} platform={data?.platform || ""} />

      <div class="mg-shell">
        <aside class="mg-sidebar">
          <div class="mg-sidebar-head">
            <Brand />
            <Badge
              tone={
                data
                  ? "success"
                  : initialLoading
                    ? "info"
                    : "warning"
              }
            >
              {data
                ? t(locale, "connected")
                : initialLoading
                  ? t(locale, "connecting")
                  : t(locale, "unauthorized")}
            </Badge>
          </div>

          <nav class="mg-nav" aria-label="Workspace navigation">
            {navItems.map((item) => (
              <button
                key={item.id}
                type="button"
                class="mg-nav-btn"
                aria-current={routeState.route === item.id ? "page" : undefined}
                onClick={() => navigateRoute(item.id)}
              >
                <span class="mg-nav-btn-label">
                  {navIcon(item.id)}
                  <span>{item.label}</span>
                </span>
                {item.badge && item.badge > 0 ? (
                  <Badge tone="info">{item.badge}</Badge>
                ) : null}
              </button>
            ))}
          </nav>

          <div class="mg-sidebar-foot">
            <span>{data?.version || ""}</span>
            {data ? <span>{data.selected_backend.toUpperCase()}</span> : null}
          </div>
        </aside>

        <div class="mg-main-scroll">
          <main
            class={
              routeState.route === "translate" ? "mg-main wide" : "mg-main"
            }
          >
            {initialLoading && !data ? (
              <StateBlock
                kind="loading"
                message={
                  en
                    ? "Connecting to local mistral manager…"
                    : "正在连接本机 mistral 服务…"
                }
                lang={uiLang}
              />
            ) : !data && (loginOpen || authDismissed) ? (
              <StateBlock
                kind="unauthorized"
                title={
                  en ? "Authentication Required" : "需要验证本机访问 Token"
                }
                message={t(locale, "loginHint")}
                action={
                  <Button
                    variant="primary"
                    onClick={() => {
                      setAuthDismissed(false);
                      setLoginOpen(true);
                    }}
                  >
                    {en ? "Enter Access Token" : "输入访问 Token 连接"}
                  </Button>
                }
                lang={uiLang}
              />
            ) : fatalLoadError && !data ? (
              <StateBlock
                kind="failed"
                title={en ? "Cannot Reach Manager" : "无法连接本地管理器"}
                message={fatalLoadError}
                onRetry={() => void initializeSession()}
                lang={uiLang}
              />
            ) : data ? (
              <div
                key={`${routeState.route}:${routeState.subPath}`}
                class="page-enter"
              >
                {routeState.route === "overview" ? (
                  <OverviewPage
                    data={data}
                    locale={locale}
                    onRefresh={async () => {
                      await loadStatus();
                      await loadModelsList();
                    }}
                    onNavigate={navigateRoute}
                  />
                ) : routeState.route === "translate" ? (
                  <TranslatePage
                    data={data}
                    locale={locale}
                    pendingScreenshot={pendingScreenshot}
                    onClearPendingScreenshot={() => setPendingScreenshot(null)}
                    onRefresh={async () => {
                      await loadStatus();
                    }}
                    onNavigate={navigateRoute}
                  />
                ) : routeState.route === "instances" ? (
                  <DeploymentsPage
                    data={data}
                    subPath={routeState.subPath}
                    installedModels={installedModels}
                    locale={locale}
                    onRefresh={async trace => {
                      await loadStatus(trace);
                      await loadModelsList(trace);
                    }}
                    onNavigateHash={navigateHash}
                  />
                ) : routeState.route === "models" ? (
                  <ModelsPage
                    data={data}
                    installedModels={installedModels}
                    deviceInfo={deviceInfo}
                    locale={locale}
                    onRefresh={async () => {
                      await loadStatus();
                      await loadModelsList();
                    }}
                  />
                ) : routeState.route === "device" ? (
                  <DevicePage
                    deviceInfo={deviceInfo}
                    locale={locale}
                    onDetect={detectDevice}
                  />
                ) : routeState.route === "connection" ? (
                  <ConnectionPage
                    data={data}
                    locale={locale}
                    onRefresh={async () => {
                      await loadStatus();
                    }}
                  />
                ) : routeState.route === "activity" ? (
                  <ActivityPage
                    operations={data.operations || []}
                    locale={locale}
                    onRefresh={async () => {
                      await loadStatus();
                      await loadModelsList();
                    }}
                  />
                ) : (
                  <SettingsPage
                    data={data}
                    locale={locale}
                    trayUpdateCheck={trayUpdateCheck}
                    onRefresh={async trace => {
                      themeOverrideRef.current = null;
                      await loadStatus(trace);
                    }}
                    onLocaleSaved={(loc) => {
                      setLocale(loc);
                      document.documentElement.lang = loc;
                    }}
                    onThemePreview={(th) => {
                      themeOverrideRef.current = th;
                    }}
                    onManagerQuit={() => {
                      sseRef.current?.close();
                      sseRef.current = null;
                      setManagerStopped(true);
                    }}
                  />
                )}
              </div>
            ) : null}
          </main>
        </div>
      </div>

      {/* M-43: Language selection modal only shown when settings.language === "" */}
      <LanguageModal
        open={Boolean(data && !data.settings.language && !loginOpen)}
        onSelected={async (chosen) => {
          await api("/api/v1/settings", { language: chosen });
          setLocale(chosen);
          document.documentElement.lang = chosen;
          await loadStatus();
        }}
      />

      <LoginModal
        open={loginOpen}
        locale={locale}
        onSuccess={async () => {
          setLoginOpen(false);
          setAuthDismissed(false);
          await initializeSession();
        }}
        onDismiss={() => {
          setLoginOpen(false);
          setAuthDismissed(true);
        }}
      />

      <ConfirmHost lang={uiLang} />
      <ToastViewport />
    </>
  );
}
