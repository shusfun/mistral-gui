import { useEffect, useState } from "preact/hooks";
import { getWailsRuntime, isDesktopMode } from "../api";
import { t, type ManagerLocale } from "../i18n";
import { Brand } from "./Brand";

export function Titlebar({
  locale,
  platform,
  showBrand = false,
}: {
  locale: ManagerLocale;
  platform: string;
  showBrand?: boolean;
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
        document.documentElement.classList.toggle(
          "fullscreen",
          Boolean(isFull),
        );
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
    <div
      class={
        showBrand ? "desktop-titlebar standalone" : "desktop-titlebar"
      }
      id="titlebar"
    >
      {showBrand ? (
        <span class="desktop-titlebar-brand">
          <Brand />
        </span>
      ) : null}
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
            <svg
              width="10"
              height="10"
              viewBox="0 0 10 10"
              fill="none"
              aria-hidden="true"
            >
              <line
                x1="1"
                y1="5"
                x2="9"
                y2="5"
                stroke="currentColor"
                stroke-width="1.1"
              />
            </svg>
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
            {maximised ? (
              <svg
                width="10"
                height="10"
                viewBox="0 0 10 10"
                fill="none"
                aria-hidden="true"
              >
                <rect
                  x="1.5"
                  y="3"
                  width="5.5"
                  height="5.5"
                  rx="0.6"
                  stroke="currentColor"
                  stroke-width="1.1"
                />
                <path
                  d="M3 3V1.8C3 1.5 3.2 1.3 3.5 1.3H8.2C8.5 1.3 8.7 1.5 8.7 1.8V6.5C8.7 6.8 8.5 7 8.2 7H7"
                  stroke="currentColor"
                  stroke-width="1.1"
                />
              </svg>
            ) : (
              <svg
                width="10"
                height="10"
                viewBox="0 0 10 10"
                fill="none"
                aria-hidden="true"
              >
                <rect
                  x="1.5"
                  y="1.5"
                  width="7"
                  height="7"
                  rx="0.8"
                  stroke="currentColor"
                  stroke-width="1.1"
                />
              </svg>
            )}
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
            <svg
              width="10"
              height="10"
              viewBox="0 0 10 10"
              fill="none"
              aria-hidden="true"
            >
              <line
                x1="1.5"
                y1="1.5"
                x2="8.5"
                y2="8.5"
                stroke="currentColor"
                stroke-width="1.15"
              />
              <line
                x1="8.5"
                y1="1.5"
                x2="1.5"
                y2="8.5"
                stroke="currentColor"
                stroke-width="1.15"
              />
            </svg>
          </button>
        </div>
      ) : null}
    </div>
  );
}
