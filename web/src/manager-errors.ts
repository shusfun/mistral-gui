import { ERROR_DICTIONARY } from "./shared/design-system/errors";

// App-specific presentation belongs here; the shared dictionary remains synchronized.
for (const entry of Object.values(ERROR_DICTIONARY)) {
  entry.zh = entry.zh.replaceAll("mistral-gui", "mistral");
  entry.en = entry.en.replaceAll("mistral-gui", "mistral");
}
Object.assign(ERROR_DICTIONARY, {
  update_sources_failed: {
    zh: "所有更新来源均未通过连接或校验。请检查网络及已配置的镜像，然后重试；也可下载完整安装包保留数据升级。",
    en: "No update source could be reached and verified. Check your network and configured mirrors, then retry, or use the full installer to preserve your data.",
  },
  update_requires_installer: {
    zh: "当前版本需要使用完整安装包升级，配置和模型会保留。",
    en: "This version requires the full installer. Your settings and models will be preserved.",
  },
  update_response_invalid: {
    zh: "更新检查返回的数据不完整，请重试。",
    en: "The update check returned incomplete data. Please retry.",
  },
});
