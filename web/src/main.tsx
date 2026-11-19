import { render } from "preact";
import "./manager-errors";
import "./shared/design-system/tokens.css";
import "./shared/design-system/components.css";
import "./styles.css";
import { App } from "./App";
import { ImagePreviewPage } from "./pages/ImagePreviewPage";
import { isDesktopMode } from "./api";

const preview = isDesktopMode &&
  new URLSearchParams(window.location.search).get("view") === "image-preview";

render(preview ? <ImagePreviewPage /> : <App />, document.getElementById("app")!);
