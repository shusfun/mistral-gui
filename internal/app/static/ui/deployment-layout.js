export const deploymentMessages = {
  models: ["模型", "Models"],
  updates: ["设置", "Settings"],
  instances: ["部署", "Deployments"],
  deployments: ["部署", "Deployments"],
  newDeployment: ["部署模型", "Deploy model"],
  deployModel: ["部署此模型", "Deploy this model"],
  deployStart: ["部署并启动", "Deploy and start"],
  downloadModel: ["部署此模型", "Deploy this model"],
  advanced: ["高级设置", "Advanced settings"],
  saveRestart: ["保存并重启此部署", "Save and restart this deployment"],
  confirmRestart: [
    "保存后将重启此部署，正在处理的请求会中断。",
    "Saving will restart this deployment and interrupt its active requests.",
  ],
  prepare: ["部署并启动", "Deploy and start"],
  preparing: ["准备中", "Preparing"],
  queued: ["等待资源准备", "Waiting for resource preparation"],
  preparing_model: ["正在准备模型", "Preparing model"],
  preparing_engine: ["正在准备推理引擎", "Preparing inference engine"],
  saving_deployment: ["正在保存部署", "Saving deployment"],
  ready: ["API 已就绪", "API ready"],
  testAPI: ["测试 API", "Test API"],
  connectionInfo: ["连接信息", "Connection details"],
  retryCPU: ["改用 CPU 重试", "Retry with CPU"],
  deployment_busy: [
    "此部署正在准备，请等待或取消当前任务。",
    "This deployment is being prepared. Wait or cancel its task.",
  ],
  prepareAccepted: ["部署任务已开始。", "Deployment task started."],
  taskCancelled: ["部署准备已取消，已下载资源会保留。", "Deployment preparation was cancelled. Downloaded resources are kept."],
  chooseModel: [
    "请先选择模型；未下载的模型可从模型目录选择。",
    "Choose a model first, or select one from the model catalog.",
  ],
  deploymentHint: [
    "选择模型，启动服务，然后复制连接信息。每个部署独立运行。",
    "Choose a model, start it, and copy its connection details. Deployments run independently.",
  ],
  recommendedDefaults: ["自动推荐运行参数", "Use recommended settings"],
  view: ["更多设置", "Details"],
  deploymentID: ["部署名称 / Model ID", "Deployment name / Model ID"],
  emptyDeployments: [
    "还没有匹配的部署。点击“部署模型”开始。",
    "No matching deployments. Choose Deploy model to begin.",
  ],
  engine_not_ready: [
    "推理引擎尚未就绪，请重试。",
    "The inference engine is not ready. Retry.",
  ],
  model_file_invalid: [
    "模型文件缺失或校验失败。请从模型页重新下载或导入，再重试。",
    "Model files are missing or invalid. Download or import the model again, then retry.",
  ],
};

export function setupDeploymentLayout() {
  const $ = (id) => document.getElementById(id);
  for (const id of ["device", "connection", "activity"]) {
    const section = $(id),
      wrapper = document.createElement("details"),
      summary = document.createElement("summary");
    summary.dataset.t = id === "activity" ? "tasks" : id;
    wrapper.append(summary, section);
    section.hidden = false;
    $("updates").append(wrapper);
  }
  const advanced = document.createElement("details"),
    summary = document.createElement("summary"),
    grid = document.createElement("div");
  summary.dataset.t = "advanced";
  advanced.append(summary);
  grid.className = "form-grid";
  for (const control of $("deployment-form").querySelectorAll("input,select")) {
    if (
      !["deployment-id", "deployment-model", "deployment-recommend"].includes(
        control.id,
      )
    )
      grid.append(control.closest("label"));
  }
  advanced.append(grid, $("recommend-deployment"), $("recommendation"));
  $("deployment-form").insertBefore(
    advanced,
    $("save-deployment").closest(".actions"),
  );
  const runtime = $("engine-info").closest("article"),
    runtimeDetails = document.createElement("details"),
    runtimeSummary = document.createElement("summary");
  runtimeSummary.dataset.t = "installedEngines";
  runtimeDetails.append(runtimeSummary, runtime);
  $("models").append(runtimeDetails);
  runtimeDetails.append(
    $("installed-engines").previousElementSibling,
    $("installed-engines"),
  );
  for (const id of ["save-backend", "rollback-model", "rollback-engine"])
    $(id).hidden = true;
  $("auto-start").closest("label").hidden = true;
  $("idle").closest("label").hidden = true;
  const sync = () => {
    for (const e of grid.querySelectorAll("input,select")) {
      if (["deployment-context", "deployment-sequences"].includes(e.id))
        e.readOnly = $("deployment-recommend").checked;
    }
  };
  $("deployment-recommend").addEventListener("change", sync);
  for (const e of grid.querySelectorAll("input,select"))
    e.addEventListener("change", () => {
      if (
        !["deployment-idle", "deployment-auto", "deployment-backend"].includes(
          e.id,
        )
      )
        $("deployment-recommend").checked = false;
    });
}
