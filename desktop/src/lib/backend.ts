import { invoke, isTauri } from "@tauri-apps/api/core";
import type { PairingPayload, ServiceStatus, SetupAction, SystemIntegration, UnlockMode } from "../types";

const now = Date.now();
let previewStatus: ServiceStatus = {
  configured: true,
  paired: true,
  credential_valid: true,
  mode: "strict",
  auto_lock: true,
  high_sensitivity: false,
  doppler_prediction: false,
  doppler_sensitivity: 60,
  immediate_unlock: false,
  failure_cooldown_enabled: true,
  session_active: true,
  locked: false,
  median_rssi: -48,
  has_rssi: true,
  last_authenticated: new Date(now - 42_000).toISOString(),
  should_lock: false,
  ble_backend: "Windows WinRT BLE",
  authorization: {
    ready: false,
    last_granted_at: new Date(now - 95_000).toISOString(),
    last_signal_at: new Date(now - 92_000).toISOString(),
    last_peek_at: new Date(now - 90_000).toISOString(),
    last_consume_at: new Date(now - 88_000).toISOString(),
  },
  unlock_rssi: -65,
  lock_rssi: -80,
  high_sensitivity_rssi: -55,
  recent_events: [
    { id: 1, at: new Date(now - 150_000).toISOString(), kind: "service", code: "service_started", message: "蓝牙解锁服务已启动", detail: "BLE 扫描、会话监视和认证状态机已就绪", result: "运行中", warning: false },
    { id: 2, at: new Date(now - 125_000).toISOString(), kind: "session", code: "session_locked", message: "Windows 已进入锁屏", detail: "等待手机返回并完成新鲜挑战", result: "已锁定", warning: false },
    { id: 3, at: new Date(now - 110_000).toISOString(), kind: "authentication", code: "transport_timeout", message: "认证失败：BLE 挑战响应超时", detail: "错误代码：transport_timeout；将继续认证，连续失败满 10 秒后才允许自动锁定", result: "失败", warning: true },
    { id: 4, at: new Date(now - 83_000).toISOString(), kind: "authentication", code: "authentication_recovered", message: "手机认证已恢复", detail: "电脑签名、手机签名、nonce、计数器和目标会话校验通过", result: "成功", warning: false },
    { id: 5, at: new Date(now - 80_000).toISOString(), kind: "authorization", code: "authorization_granted", message: "一次性解锁授权已就绪", detail: "授权将在短时间内过期且只能消费一次", result: "待消费", warning: false },
    { id: 6, at: new Date(now - 75_000).toISOString(), kind: "credential", code: "credential_consumed", message: "自动解锁凭据已安全提交", detail: "仅向当前锁定会话提交一次，授权消费后立即失效", result: "已提交", warning: false },
    { id: 7, at: new Date(now - 70_000).toISOString(), kind: "credential", code: "unlock_success", message: "Windows 已接受自动解锁凭据", detail: "一次性授权已消费并清除", result: "成功", warning: false },
  ],
};

export const isDesktopRuntime = isTauri();

function withPreviewSignal(): ServiceStatus {
  const wave = Math.round(Math.sin(Date.now() / 8_000) * 3);
  return { ...previewStatus, median_rssi: -49 + wave };
}

export async function getStatus(): Promise<ServiceStatus> {
  if (!isDesktopRuntime) return withPreviewSignal();
  return invoke<ServiceStatus>("get_status");
}

export async function setMode(mode: UnlockMode): Promise<void> {
  if (!isDesktopRuntime) {
    previewStatus = { ...previewStatus, mode };
    return;
  }
  await invoke("set_mode", { mode });
}

export async function setAutoLock(enabled: boolean): Promise<void> {
  if (!isDesktopRuntime) {
    previewStatus = { ...previewStatus, auto_lock: enabled };
    return;
  }
  await invoke("set_auto_lock", { enabled });
}

export async function setHighSensitivity(enabled: boolean): Promise<void> {
  if (!isDesktopRuntime) {
    previewStatus = { ...previewStatus, high_sensitivity: enabled };
    return;
  }
  await invoke("set_high_sensitivity", { enabled });
}

export async function setDopplerPrediction(enabled: boolean): Promise<void> {
  if (!isDesktopRuntime) {
    previewStatus = { ...previewStatus, doppler_prediction: enabled };
    return;
  }
  await invoke("set_doppler_prediction", { enabled });
}

export async function setDopplerSensitivity(sensitivity: number): Promise<void> {
  if (!isDesktopRuntime) {
    previewStatus = { ...previewStatus, doppler_sensitivity: sensitivity };
    return;
  }
  await invoke("set_doppler_sensitivity", { sensitivity });
}

export async function setHighSensitivityThreshold(rssi: number): Promise<void> {
  if (!isDesktopRuntime) {
    previewStatus = { ...previewStatus, high_sensitivity_rssi: rssi };
    return;
  }
  await invoke("set_high_sensitivity_threshold", { rssi });
}

export async function setImmediateUnlock(enabled: boolean): Promise<void> {
  if (!isDesktopRuntime) {
    previewStatus = { ...previewStatus, immediate_unlock: enabled };
    return;
  }
  await invoke("set_immediate_unlock", { enabled });
}

export async function setFailureCooldown(enabled: boolean): Promise<void> {
  if (!isDesktopRuntime) {
    previewStatus = { ...previewStatus, failure_cooldown_enabled: enabled };
    return;
  }
  await invoke("set_failure_cooldown", { enabled });
}

export async function setThresholds(
  unlockRssi: number,
  lockRssi: number,
  highSensitivityRssi?: number,
): Promise<void> {
  if (!isDesktopRuntime) {
    previewStatus = {
      ...previewStatus,
      unlock_rssi: unlockRssi,
      lock_rssi: lockRssi,
      high_sensitivity_rssi: highSensitivityRssi ?? previewStatus.high_sensitivity_rssi,
    };
    return;
  }
  await invoke("set_thresholds", { unlockRssi, lockRssi, highSensitivityRssi });
}

export async function pauseService(seconds: number): Promise<void> {
  if (!isDesktopRuntime) {
    previewStatus = {
      ...previewStatus,
      paused_until: seconds ? new Date(Date.now() + seconds * 1_000).toISOString() : undefined,
    };
    return;
  }
  await invoke("pause", { seconds });
}

export async function lockComputer(): Promise<void> {
  if (!isDesktopRuntime) return;
  await invoke("lock_workstation");
}

export async function startPairing(): Promise<PairingPayload> {
  if (!isDesktopRuntime) {
    throw new Error("请在蓝牙解锁桌面程序中开始配对");
  }
  return invoke<PairingPayload>("start_pairing");
}

export async function revokePairing(): Promise<void> {
  if (!isDesktopRuntime) {
    previewStatus = { ...previewStatus, paired: false, has_rssi: false };
    return;
  }
  await invoke("revoke_pairing");
}

export async function getSystemIntegration(): Promise<SystemIntegration> {
  if (!isDesktopRuntime) return { credential_provider_registered: true };
  return invoke<SystemIntegration>("get_system_integration");
}

export async function runSetupAction(action: SetupAction): Promise<void> {
  if (!isDesktopRuntime) return;
  await invoke("run_setup_action", { action });
}
