import { button, details, copyField, taskProgress } from "./components.js";

export function deploymentCard(
  d,
  op,
  { t, errorText, endpoint, token, testResult, inDetail, act },
) {
  const row = document.createElement("div"),
    head = document.createElement("div"),
    title = document.createElement("h3"),
    badge = document.createElement("span"),
    model = document.createElement("p"),
    actions = document.createElement("div");
  row.className = "row deployment-card";
  row.dataset.deployment = d.id;
  row.dataset.state =
    op?.state === "running"
      ? "preparing"
      : op && ["failed", "cancelled", "interrupted"].includes(op.state)
        ? op.state
        : d.status.state;
  head.className = "card-head";
  title.textContent = d.id;
  badge.className =
    "badge " +
    (d.status.state === "running"
      ? "good"
      : d.status.state === "failed"
        ? "bad"
        : "");
  badge.textContent = t(row.dataset.state);
  head.append(title, badge);
  model.className = "muted";
  model.textContent =
    d.model + " · " + (d.status.backend || d.backend || t("unknown"));
  row.append(head, model);
  const busy =
    op?.state === "running" ||
    ["starting", "loading", "stopping"].includes(d.status.state);
  const add = (key, kind = "") => {
    const control = button(
      t(key),
      async () => {
        control.disabled = true;
        try {
          await act(key, d, op);
        } catch (e) {
          message.textContent = e.message;
          message.dataset.tone = "error";
        } finally {
          control.disabled = false;
        }
      },
      kind,
    );
    control.dataset.action = key;
    actions.append(control);
  };
  const message = document.createElement("p");
  message.setAttribute("role", "status");
  if (op && op.state !== "success") {
    message.textContent =
      op.state === "cancelled"
        ? t("taskCancelled")
        : op.error
          ? errorText(op.error)
          : t(op.step || op.state);
    if (op.error) message.dataset.tone = "error";
    if (op.state === "running")
      row.append(taskProgress(op.progress, op.total, t(op.step)));
  } else if (d.status.error) {
    message.textContent = errorText(d.status.error);
    message.dataset.tone = "error";
  }
  if (testResult) message.textContent = testResult;
  row.append(message);
  actions.className = "actions";
  if (op?.state === "running") add("cancel");
  else if (!busy) {
    if (d.status.state === "running") {
      add("stop");
      add("testAPI");
    } else
      add(
        op && ["failed", "cancelled", "interrupted"].includes(op.state)
          ? "retry"
          : "start",
        "primary",
      );
    if (
      (d.status.state === "failed" || op?.state === "failed") &&
      ["cuda", "metal"].includes(
        op?.prepare?.backend || d.status.backend || d.backend,
      )
    )
      add("retryCPU");
  }
  add(inDetail ? "edit" : "view");
  if (inDetail && !busy) {
    if (d.status.state === "running") add("openUI");
    if (d.previous_model) add("rollbackModel");
    if (!d.legacy) add("removeDeployment");
  }
  row.append(actions);
  if (d.status.state === "running") {
    const connection = document.createElement("details"),
      summary = document.createElement("summary");
    connection.dataset.connection = d.id;
    summary.textContent = t("connectionInfo");
    connection.open = inDetail;
    connection.append(
      summary,
      copyField(
        "API Base URL",
        endpoint.replace(/\/$/, "") + "/v1",
        t("copied"),
      ),
      copyField("Model ID", d.id, t("copied")),
      copyField("Token", token, t("copied"), true),
    );
    row.append(connection);
  }
  if (op?.error || d.status.detail)
    row.append(
      details(t("details"), {
        error: op?.error || d.status.error,
        detail: op?.detail || d.status.detail,
        trace_id: op?.trace_id,
      }),
    );
  return row;
}
