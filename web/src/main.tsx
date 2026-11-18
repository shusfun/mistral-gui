import { render } from "preact";
import "./manager-errors";
import "./shared/design-system/tokens.css";
import "./shared/design-system/components.css";
import "./styles.css";
import { App } from "./App";

render(<App />, document.getElementById("app")!);
