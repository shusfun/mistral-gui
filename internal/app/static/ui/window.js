import { confirmAction } from "./components.js";
export async function setupWindow(platform, nativeCommand) {
  const runtime = await import("/wails/runtime.js"),
    win = runtime.Window;
  runtime.Events.On("desktop:confirm", async (event) => {
    const data = event.data;
    const accepted = await confirmAction(data.message);
    await nativeCommand(
      "confirm-result",
      JSON.stringify({ id: data.id, accepted }),
      data.trace,
    );
  });
  document.documentElement.classList.add("desktop");
  if (platform?.startsWith("darwin"))
    document.documentElement.classList.add("mac");
  document.getElementById("titlebar").hidden = false;
  const controls = {
    "window-minimize": () => win.Minimise(),
    "window-maximize": () => win.ToggleMaximise(),
    "window-close": () => win.Close(),
  };
  for (const [id, action] of Object.entries(controls))
    document.getElementById(id).addEventListener("click", (event) => {
      void action();
    });
  const update = async () => {
    document.documentElement.classList.toggle(
      "fullscreen",
      await win.IsFullscreen(),
    );
    document.getElementById("window-maximize").textContent =
      (await win.IsMaximised()) ? "❐" : "□";
  };
  addEventListener("resize", () => {
    void update();
  });
  await update();
}
