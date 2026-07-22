import QRCode from "qrcode";
import { useEffect, useMemo, useState } from "react";
import { AppHeader } from "./components/AppHeader";
import { CalibrationDialog } from "./components/CalibrationDialog";
import { ConfirmDialog } from "./components/ConfirmDialog";
import { DevicesPage } from "./components/DevicesPage";
import { HeroStatus } from "./components/HeroStatus";
import { LogsPage } from "./components/LogsPage";
import { ParameterDialog, type ParameterDialogKind } from "./components/ParameterDialog";
import { SecurityTimeline } from "./components/SecurityTimeline";
import { SettingsPage } from "./components/SettingsPage";
import { SignalChart } from "./components/SignalChart";
import { UnlockSettings } from "./components/UnlockSettings";
import { useServiceStatus } from "./hooks/useServiceStatus";
import {
  lockComputer,
  getSystemIntegration,
  pauseService,
  revokePairing,
  runSetupAction,
  setAutoLock,
  setFailureCooldown,
  setDopplerPrediction,
  setDopplerSensitivity,
  setHighSensitivity,
  setHighSensitivityThreshold,
  setImmediateUnlock,
  setMode,
  setThresholds,
  startPairing,
} from "./lib/backend";
import { buildSecurityEvents, describeError } from "./lib/format";
import type { AppSection, SystemIntegration, UnlockMode } from "./types";

type Confirmation = "convenience" | "immediate" | "cooldown" | "sensitivity" | "doppler" | "revoke" | "uninstall" | null;

const confirmationCopy: Record<Exclude<Confirmation, null>, { title: string; description: string; label: string; tone: "danger" | "warning" }> = {
  convenience: {
    title: "切换到便捷模式？",
    description: "便捷模式允许手机在锁屏时完成签名。持有锁屏手机的人可能解锁附近的电脑。",
    label: "确认切换",
    tone: "warning",
  },
  immediate: {
    title: "开启锁屏后立即解锁？",
    description: "开启后，只要手机仍在附近并能完成签名，主动按 Win+L 也可能在几秒内自动解锁。",
    label: "仍要开启",
    tone: "warning",
  },
  cooldown: {
    title: "关闭认证失败冷却？",
    description: "关闭后，签名错误、重放或协议非法也不会触发 5 分钟保护。普通蓝牙超时本来就不会触发此冷却。",
    label: "仍要关闭",
    tone: "warning",
  },
  sensitivity: {
    title: "开启高灵敏模式？",
    description: "开启后将使用独立的高灵敏触发阈值。锁屏时首个符合阈值的新鲜广播会在约 0.2 秒内发起认证；认证失败后仍会持续检测，只有连续 10 秒都未恢复才锁定。响应更快，但也会增加手机耗电。",
    label: "开启高灵敏模式",
    tone: "warning",
  },
  doppler: {
    title: "开启信号趋势预测？",
    description: "系统会根据 RSSI 增强趋势提前发起认证，但只有手机签名验证通过后才会提交解锁凭据。预测解锁后若连续 10 秒拿不到新的有效证明，电脑会重新锁定。",
    label: "开启趋势预测",
    tone: "warning",
  },
  revoke: {
    title: "撤销已配对手机？",
    description: "电脑端将清除手机公钥和 presence key，之后必须重新配对才能使用蓝牙解锁。",
    label: "撤销设备",
    tone: "danger",
  },
  uninstall: {
    title: "完整卸载蓝牙解锁？",
    description: "这会停用 Credential Provider，并删除服务、手机配对、LSA 密码、电脑身份密钥和登录启动项。Windows PIN、密码和 Hello 不受影响。",
    label: "确认卸载",
    tone: "danger",
  },
};

export default function App() {
  const [section, setSection] = useState<AppSection>("overview");
  const [busy, setBusy] = useState<string | null>(null);
  const [confirmation, setConfirmation] = useState<Confirmation>(null);
  const [toast, setToast] = useState<string | null>(null);
  const [qrCode, setQrCode] = useState<string | null>(null);
  const [pairingExpiresAt, setPairingExpiresAt] = useState<string | null>(null);
  const [calibrationOpen, setCalibrationOpen] = useState(false);
  const [parameterDialog, setParameterDialog] = useState<ParameterDialogKind | null>(null);
  const [systemIntegration, setSystemIntegration] = useState<SystemIntegration | null>(null);
  const { status, error, loading, signalPoints, refresh } = useServiceStatus();
  const events = useMemo(() => buildSecurityEvents(status), [status]);

  useEffect(() => {
    let active = true;
    void getSystemIntegration().then((value) => {
      if (active) setSystemIntegration(value);
    });
    return () => {
      active = false;
    };
  }, []);

  function showToast(message: string) {
    setToast(message);
    window.setTimeout(() => setToast((current) => (current === message ? null : current)), 3_200);
  }

  async function runOperation(name: string, operation: () => Promise<void>, success: string) {
    if (busy) return;
    setBusy(name);
    try {
      await operation();
      await refresh();
      showToast(success);
    } catch (operationError) {
      showToast(describeError(operationError));
    } finally {
      setBusy(null);
    }
  }

  function handleModeChange(mode: UnlockMode) {
    if (mode === status?.mode) return;
    if (mode === "convenience") {
      setConfirmation("convenience");
      return;
    }
    void runOperation("mode", () => setMode(mode), "已切换到安全模式");
  }

  function handleImmediateUnlock(enabled: boolean) {
    if (enabled) {
      setConfirmation("immediate");
      return;
    }
    void runOperation("immediate", () => setImmediateUnlock(false), "已关闭锁屏后立即解锁");
  }

  function handleFailureCooldown(enabled: boolean) {
    if (!enabled) {
      setConfirmation("cooldown");
      return;
    }
    void runOperation("cooldown", () => setFailureCooldown(true), "认证失败冷却已开启");
  }

  function handleHighSensitivity(enabled: boolean) {
    if (enabled) {
      setConfirmation("sensitivity");
      return;
    }
    void runOperation("sensitivity", () => setHighSensitivity(false), "高灵敏模式已关闭");
  }

  function handleDopplerPrediction(enabled: boolean) {
    if (enabled) {
      setConfirmation("doppler");
      return;
    }
    void runOperation("doppler", () => setDopplerPrediction(false), "信号趋势预测已关闭");
  }

  async function confirmSensitiveAction() {
    const action = confirmation;
    setConfirmation(null);
    if (action === "convenience") {
      await runOperation("mode", () => setMode("convenience"), "已切换到便捷模式");
    }
    if (action === "immediate") {
      await runOperation("immediate", () => setImmediateUnlock(true), "已开启锁屏后立即解锁");
    }
    if (action === "cooldown") {
      await runOperation("cooldown", () => setFailureCooldown(false), "认证失败冷却已关闭");
    }
    if (action === "sensitivity") {
      await runOperation("sensitivity", () => setHighSensitivity(true), "高灵敏模式已开启");
    }
    if (action === "doppler") {
      await runOperation("doppler", () => setDopplerPrediction(true), "信号趋势预测已开启");
    }
    if (action === "revoke") {
      await runOperation("revoke", revokePairing, "已撤销手机配对");
      setQrCode(null);
      setPairingExpiresAt(null);
    }
    if (action === "uninstall") {
      await runOperation("uninstall", () => runSetupAction("uninstall"), "卸载已完成");
    }
  }

  async function handleCredentialProvider() {
    const enabled = Boolean(systemIntegration?.credential_provider_registered);
    await runOperation(
      "credential-provider",
      () => runSetupAction(enabled ? "disable-credential-provider" : "enable-credential-provider"),
      enabled ? "锁屏自动解锁组件已停用" : "锁屏自动解锁组件已启用",
    );
    try {
      setSystemIntegration(await getSystemIntegration());
    } catch {
      // The service status remains usable even if the registry refresh is delayed.
    }
  }

  async function handlePairing() {
    if (busy) return;
    setBusy("pairing");
    try {
      const payload = await startPairing();
      const image = await QRCode.toDataURL(payload.uri, {
        width: 300,
        margin: 2,
        color: { dark: "#11254f", light: "#ffffff" },
        errorCorrectionLevel: "M",
      });
      setQrCode(image);
      setPairingExpiresAt(payload.expires_at);
      showToast("配对二维码已生成，请在两分钟内扫描");
    } catch (pairingError) {
      showToast(describeError(pairingError));
    } finally {
      setBusy(null);
    }
  }

  const settingsProps = {
    status,
    busy,
    onModeChange: handleModeChange,
    onAutoLockChange: (enabled: boolean) => void runOperation("auto-lock", () => setAutoLock(enabled), enabled ? "自动锁定已开启" : "自动锁定已关闭"),
    onHighSensitivityChange: handleHighSensitivity,
    onDopplerPredictionChange: handleDopplerPrediction,
    onImmediateUnlockChange: handleImmediateUnlock,
    onFailureCooldownChange: handleFailureCooldown,
  };

  return (
    <div className="app-shell">
      <AppHeader active={section} onNavigate={setSection} />
      {section === "overview" ? (
        <main className="overview-page">
          <HeroStatus
            status={status}
            error={error}
            loading={loading}
            onLock={() => void runOperation("lock", lockComputer, "电脑已锁定")}
            onManageDevice={() => setSection("devices")}
          />
          <div className="dashboard-grid">
            <SignalChart
              points={signalPoints}
              current={status?.median_rssi}
              unlockThreshold={status?.high_sensitivity
                ? status.high_sensitivity_rssi ?? -55
                : status?.unlock_rssi ?? -65}
              thresholdLabel={status?.high_sensitivity ? "高灵敏触发" : "解锁阈值"}
              onCalibrate={() => setCalibrationOpen(true)}
            />
            <UnlockSettings {...settingsProps} compact onMore={() => setSection("settings")} />
          </div>
          <SecurityTimeline events={events} onOpenLogs={() => setSection("logs")} />
        </main>
      ) : null}
      {section === "devices" ? (
        <DevicesPage
          status={status}
          qrCode={qrCode}
          pairingExpiresAt={pairingExpiresAt}
          busy={busy}
          onStartPairing={() => void handlePairing()}
          onRevoke={() => setConfirmation("revoke")}
          onCalibrate={() => setCalibrationOpen(true)}
          onThresholdsChange={(unlockRssi, lockRssi) => void runOperation(
            "thresholds",
            () => setThresholds(unlockRssi, lockRssi),
            `普通模式距离阈值已更新：解锁 ${unlockRssi}、锁定 ${lockRssi} dBm`,
          )}
        />
      ) : null}
      {section === "logs" ? <LogsPage events={events} status={status} /> : null}
      {section === "settings" ? (
        <SettingsPage
          {...settingsProps}
          onHighSensitivityParameters={() => setParameterDialog("high-sensitivity")}
          onDopplerParameters={() => setParameterDialog("doppler")}
          systemIntegration={systemIntegration}
          onPause={(seconds) => void runOperation("pause", () => pauseService(seconds), seconds ? "已暂停 5 分钟" : "蓝牙解锁已恢复")}
          onCredentialProvider={() => void handleCredentialProvider()}
          onUpdatePassword={() => void runOperation("password", () => runSetupAction("set-password"), "Windows 密码已安全更新")}
          onUninstall={() => setConfirmation("uninstall")}
        />
      ) : null}
      {confirmation ? (
        <ConfirmDialog
          open
          {...confirmationCopy[confirmation]}
          confirmLabel={confirmationCopy[confirmation].label}
          onCancel={() => setConfirmation(null)}
          onConfirm={() => void confirmSensitiveAction()}
        />
      ) : null}
      <CalibrationDialog open={calibrationOpen} onClose={() => setCalibrationOpen(false)} onApplied={() => void refresh()} />
      <ParameterDialog
        kind={parameterDialog}
        value={parameterDialog === "doppler" ? status?.doppler_sensitivity ?? 60 : status?.high_sensitivity_rssi ?? -55}
        busy={Boolean(busy)}
        onClose={() => setParameterDialog(null)}
        onSave={(value) => {
          const kind = parameterDialog;
          setParameterDialog(null);
          if (kind === "doppler") {
            void runOperation("doppler-parameter", () => setDopplerSensitivity(value), `趋势预测灵敏度已设为 ${value}`);
          } else if (kind === "high-sensitivity") {
            void runOperation("high-sensitivity-parameter", () => setHighSensitivityThreshold(value), `高灵敏解锁阈值已设为 ${value} dBm`);
          }
        }}
      />
      {toast ? <div className="toast" role="status">{toast}</div> : null}
    </div>
  );
}
