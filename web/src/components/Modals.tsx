import { useState } from "preact/hooks";
import {
  Button,
  Dialog,
  FeedbackBanner,
  TextInput,
  feedbackStore,
  runTracked,
  useFeedbackStore,
} from "../shared/design-system";
import { api } from "../api";
import { t, toUiLang, type ManagerLocale } from "../i18n";

// M-43: Only persists language AFTER explicit user click (never silently auto-commits before selection!)
export function LanguageModal({
  open,
  onSelected,
}: {
  open: boolean;
  onSelected: (locale: ManagerLocale) => Promise<void>;
}) {
  const store = useFeedbackStore();
  if (!open) return null;

  const choose = (loc: ManagerLocale) =>
    runTracked(
      { key: `lang:${loc}`, scope: "lang-modal", lang: toUiLang(loc) },
      async () => {
        await onSelected(loc);
      },
    );

  return (
    <Dialog
      open={open}
      title="选择界面语言 / Choose Language"
      labelledById="lang-modal-title"
    >
      <p class="muted">
        请选择管理台显示语言（之后可随时在「更新与设置」中切换）。
        <br />
        Choose your preferred interface language.
      </p>
      <FeedbackBanner scope="lang-modal" lang="zh-CN" />
      <div class="actions">
        <Button
          variant="primary"
          busy={store.isBusy("lang:zh-CN")}
          onClick={() => void choose("zh-CN")}
        >
          简体中文
        </Button>
        <Button
          busy={store.isBusy("lang:en-US")}
          onClick={() => void choose("en-US")}
        >
          English
        </Button>
      </div>
    </Dialog>
  );
}

// M-04, M-20, M-27: Real <form> Enter submit, Esc closes cleanly to unauthorized view without 10s popup spam
export function LoginModal({
  open,
  locale,
  onSuccess,
  onDismiss,
}: {
  open: boolean;
  locale: ManagerLocale;
  onSuccess: () => Promise<void>;
  onDismiss: () => void;
}) {
  const store = useFeedbackStore();
  const [tokenInput, setTokenInput] = useState("");
  const uiLang = toUiLang(locale);
  const en = uiLang === "en";

  const handleSubmit = (e: Event) => {
    e.preventDefault();
    const cleaned = tokenInput.trim();
    if (!cleaned) {
      feedbackStore.reportError(
        "login-modal",
        new Error("invalid_token"),
        uiLang,
      );
      return;
    }
    void runTracked(
      { key: "auth:login", scope: "login-modal", lang: uiLang },
      async () => {
        await api(
          "/api/v1/session",
          {},
          { tokenOverride: cleaned, suppressAuthTrigger: true },
        );
        setTokenInput("");
        feedbackStore.clearBanner("login-modal");
        await onSuccess();
      },
    );
  };

  return (
    <Dialog
      open={open}
      title={t(locale, "loginTitle")}
      labelledById="login-modal-title"
      onClose={onDismiss}
    >
      <form onSubmit={handleSubmit}>
        <p class="muted">{t(locale, "loginHint")}</p>
        <FeedbackBanner scope="login-modal" lang={uiLang} />
        <TextInput
          type="password"
          label={t(locale, "tokenLabel")}
          placeholder={en ? "Paste 64-char hex token…" : "粘贴本机 64 位访问 Token…"}
          autocomplete="off"
          value={tokenInput}
          onValueInput={(v) => setTokenInput(v)}
        />
        <div class="actions">
          <Button
            type="submit"
            variant="primary"
            busy={store.isBusy("auth:login")}
          >
            {t(locale, "connectBtn")}
          </Button>
          <Button type="button" variant="ghost" onClick={onDismiss}>
            {t(locale, "cancelBtn")}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
