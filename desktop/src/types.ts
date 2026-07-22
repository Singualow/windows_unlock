export type UnlockMode = "strict" | "convenience";
export type AppSection = "overview" | "devices" | "logs" | "settings";

export interface AuthorizationStatus {
  ready: boolean;
  expires_at?: string;
  last_granted_at?: string;
  last_signal_at?: string;
  last_signal_error?: string;
  last_peek_at?: string;
  last_consume_at?: string;
}

export interface ServiceStatus {
  configured: boolean;
  paired: boolean;
  credential_valid: boolean;
  mode: UnlockMode;
  auto_lock: boolean;
  high_sensitivity: boolean;
  doppler_prediction: boolean;
  doppler_sensitivity: number;
  doppler_triggered?: boolean;
  predicted_rssi?: number;
  rssi_slope_db_per_sec?: number;
  immediate_unlock: boolean;
  failure_cooldown_enabled: boolean;
  paused_until?: string;
  session_active: boolean;
  locked: boolean;
  median_rssi?: number;
  has_rssi: boolean;
  last_authenticated?: string;
  should_lock: boolean;
  ble_backend: string;
  authorization: AuthorizationStatus;
  cooldown_until?: string;
  last_authentication_failure_code?: string;
  last_authentication_failure_reason?: string;
  last_authentication_failure_at?: string;
  last_credential_provider_event?: string;
  last_credential_provider_event_at?: string;
  unlock_rssi?: number;
  lock_rssi?: number;
  high_sensitivity_rssi?: number;
  recent_events?: ServiceLogEntry[];
}

export interface ServiceLogEntry {
  id: number;
  at: string;
  kind: "service" | "authentication" | "session" | "authorization" | "credential" | "configuration" | "pairing";
  code: string;
  message: string;
  detail?: string;
  result: string;
  warning: boolean;
}

export interface SignalPoint {
  at: number;
  value: number;
}

export type EventTone = "green" | "blue" | "violet" | "signal";

export interface SecurityEvent {
  id: string;
  at: Date;
  title: string;
  tone: EventTone;
  icon: "shield" | "key" | "credential" | "signal";
  warning?: boolean;
  detail?: string;
  result?: string;
}

export interface PairingPayload {
  uri: string;
  expires_at: string;
}

export interface SystemIntegration {
  credential_provider_registered: boolean;
}

export type SetupAction =
  | "enable-credential-provider"
  | "disable-credential-provider"
  | "set-password"
  | "uninstall";
