import { invoke, isTauri } from "@tauri-apps/api/core";
import type { PairingPayload, ServiceStatus, SetupAction, SystemIntegration, UnlockMode } from "../types";

const now = Date.now();
let previewStatus: ServiceStatus = {
  configured: true,
  paired: true,
  credential_valid: true,
  mode: "strict",
  auto_lock: true,
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

export async function setThresholds(unlockRssi: number, lockRssi: number): Promise<void> {
  if (!isDesktopRuntime) {
    previewStatus = { ...previewStatus, unlock_rssi: unlockRssi, lock_rssi: lockRssi };
    return;
  }
  await invoke("set_thresholds", { unlockRssi, lockRssi });
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
