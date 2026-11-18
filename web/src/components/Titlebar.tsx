import { useEffect, useState } from "preact/hooks";
import { getWailsRuntime, isDesktopMode } from "../api";
import { t, type ManagerLocale } from "../i18n";

export function Titlebar({
  locale,
  platform,
}: {
  locale: ManagerLocale;
  platform: string;
}) {
  const [maximised, setMaximised] = useState(false);

  useEffect(() => {
    if (!isDesktopMode) return;
    let disposed = false;
    const syncState = async () => {
      const rt = await getWailsRuntime();
      if (!rt?.Window || disposed) return;
      try {
        const isFull = await rt.Window.IsFullscreen();
        document.documentElement.classList.toggle("fullscreen", Boolean(isFull));
        setMaximised(Boolean(await rt.Window.IsMaximised()));
      } catch {
        // ignore window state query race
      }
    };
    void syncState();
    window.addEventListener("resize", syncState);
    return () => {
      disposed = true;
      window.removeEventListener("resize", syncState);
    };
  }, []);

  if (!isDesktopMode) return null;
  const isMac = platform.startsWith("darwin");

  return (
    <div class="desktop-titlebar" id="titlebar">
      <span class="desktop-titlebar-brand">mistral-gui</span>
      {!isMac ? (
        <div class="desktop-window-controls">
          <button
            type="button"
            id="window-minimize"
            aria-label={t(locale, "minimizeWindow")}
            title={t(locale, "minimizeWindow")}
            onClick={async () => {
              const rt = await getWailsRuntime();
              await rt?.Window?.Minimise?.();
            }}
          >
            ─
          </button>
          <button
            type="button"
            id="window-maximize"
            aria-label={t(locale, "maximizeWindow")}
            title={t(locale, "maximizeWindow")}
            onClick={async () => {
              const rt = await getWailsRuntime();
              await rt?.Window?.ToggleMaximise?.();
              setMaximised(Boolean(await rt?.Window?.IsMaximised?.()));
            }}
          >
            {maximised ? "❐" : "□"}
          </button>
          <button
            type="button"
            id="window-close"
            class="close-btn"
            aria-label={t(locale, "closeWindow")}
            title={t(locale, "closeWindow")}
            onClick={async () => {
              const rt = await getWailsRuntime();
              await rt?.Window?.Close?.();
            }}
          >
            ✕
          </button>
        </div>
      ) : null}
    </div>
  );
}
