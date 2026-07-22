import type { ServiceStatus, UnlockMode } from "../types";
import { Icon, type IconName } from "./Icon";

interface UnlockSettingsProps {
  status: ServiceStatus | null;
  busy: string | null;
  onModeChange: (mode: UnlockMode) => void;
  onAutoLockChange: (enabled: boolean) => void;
  onHighSensitivityChange: (enabled: boolean) => void;
  onDopplerPredictionChange: (enabled: boolean) => void;
  onImmediateUnlockChange: (enabled: boolean) => void;
  onFailureCooldownChange: (enabled: boolean) => void;
  onHighSensitivityParameters?: () => void;
  onDopplerParameters?: () => void;
  onMore: () => void;
  compact?: boolean;
  grouped?: boolean;
  showMore?: boolean;
  showFailureCooldown?: boolean;
}

interface SettingRowProps {
  id: string;
  icon: IconName;
  tone: "blue" | "gray" | "green" | "violet";
  title: string;
  description: string;
  selected: boolean;
  kind: "radio" | "switch";
  warning?: boolean;
  disabled?: boolean;
  onClick: () => void;
  onParameters?: () => void;
}

function SettingRow({ id, icon, tone, title, description, selected, kind, warning, disabled, onClick, onParameters }: SettingRowProps) {
  return (
    <div className={`setting-row${selected && kind === "radio" ? " is-selected" : ""}`} id={id}>
      <button
        aria-checked={selected}
        className="setting-toggle"
        disabled={disabled}
        onClick={onClick}
        role={kind}
        type="button"
      >
        <span className={`setting-icon ${tone}`}><Icon name={icon} size={22} /></span>
        <span className="setting-copy">
          <strong>{title}</strong>
          <small className={warning ? "warning-copy" : ""}>{description}</small>
        </span>
        {kind === "radio" ? (
          <span className={selected ? "radio-control is-on" : "radio-control"}>
            {selected ? <Icon name="check" size={15} /> : null}
          </span>
        ) : (
          <span className={selected ? "switch-control is-on" : "switch-control"}><span /></span>
        )}
      </button>
      {onParameters ? (
        <button className="setting-parameter-button" disabled={disabled} onClick={onParameters} type="button">
          参数 <Icon name="arrow" size={15} />
        </button>
      ) : null}
    </div>
  );
}

interface OverviewModeSelectorProps {
  mode: UnlockMode;
  disabled: boolean;
  onModeChange: (mode: UnlockMode) => void;
}

function OverviewModeSelector({ mode, disabled, onModeChange }: OverviewModeSelectorProps) {
  return (
    <div className="overview-mode-selector" role="radiogroup" aria-label="解锁模式">
      <button
        aria-checked={mode === "strict"}
        className={mode === "strict" ? "is-selected" : ""}
        disabled={disabled}
        onClick={() => onModeChange("strict")}
        role="radio"
        type="button"
      >
        <span className="setting-icon blue"><Icon name="shield" size={20} /></span>
        <span><strong>安全模式</strong><small>手机解锁后认证</small></span>
      </button>
      <button
        aria-checked={mode === "convenience"}
        className={mode === "convenience" ? "is-selected" : ""}
        disabled={disabled}
        onClick={() => onModeChange("convenience")}
        role="radio"
        type="button"
      >
        <span className="setting-icon gray"><Icon name="hand" size={20} /></span>
        <span><strong>便捷模式</strong><small>手机锁屏时认证</small></span>
      </button>
    </div>
  );
}

export function UnlockSettings({
  status,
  busy,
  onModeChange,
  onAutoLockChange,
  onHighSensitivityChange,
  onDopplerPredictionChange,
  onImmediateUnlockChange,
  onFailureCooldownChange,
  onHighSensitivityParameters,
  onDopplerParameters,
  onMore,
  compact = false,
  grouped = false,
  showMore = true,
  showFailureCooldown = false,
}: UnlockSettingsProps) {
  const mode = status?.mode ?? "strict";
  const disabled = !status || Boolean(busy);
  const modeRows = (
    <>
      <SettingRow
        id="strict-mode"
        icon="shield"
        tone="blue"
        title="安全模式"
        description="手机解锁后才可认证"
        selected={mode === "strict"}
        kind="radio"
        disabled={disabled}
        onClick={() => onModeChange("strict")}
      />
      <SettingRow
        id="convenience-mode"
        icon="hand"
        tone="gray"
        title="便捷模式"
        description="手机锁屏时仍可认证"
        selected={mode === "convenience"}
        kind="radio"
        disabled={disabled}
        onClick={() => onModeChange("convenience")}
      />
    </>
  );
  const autoLockRow = (
    <SettingRow
      id="auto-lock"
      icon="lock"
      tone="green"
      title="自动锁定"
      description="手机离开后自动锁定电脑"
      selected={Boolean(status?.auto_lock)}
      kind="switch"
      disabled={disabled}
      onClick={() => onAutoLockChange(!status?.auto_lock)}
    />
  );
  const highSensitivityRow = (
    <SettingRow
      id="high-sensitivity"
      icon="signal"
      tone="blue"
      title="高灵敏模式"
      description={status?.high_sensitivity
        ? "快速检测离开和返回，短暂波动可能误锁"
        : "缩短检测周期，提升锁定与解锁响应"}
      selected={Boolean(status?.high_sensitivity)}
      kind="switch"
      warning={Boolean(status?.high_sensitivity)}
      disabled={disabled}
      onClick={() => onHighSensitivityChange(!status?.high_sensitivity)}
      onParameters={onHighSensitivityParameters}
    />
  );
  const dopplerRow = (
    <SettingRow
      id="doppler-prediction"
      icon="signal"
      tone="violet"
      title="多普勒预测"
      description={status?.doppler_prediction
        ? `RSSI 趋势预测已启用 · 灵敏度 ${status.doppler_sensitivity ?? 60}`
        : "预测信号增强趋势，提前发起安全认证"}
      selected={Boolean(status?.doppler_prediction)}
      kind="switch"
      warning={Boolean(status?.doppler_prediction)}
      disabled={disabled}
      onClick={() => onDopplerPredictionChange(!status?.doppler_prediction)}
      onParameters={onDopplerParameters}
    />
  );
  const advancedRows = (
    <>
      {autoLockRow}
      {highSensitivityRow}
      {dopplerRow}
      <SettingRow
        id="immediate-unlock"
        icon="lock"
        tone="gray"
        title="锁屏后立即解锁"
        description="开启后会降低安全性"
        selected={Boolean(status?.immediate_unlock)}
        kind="switch"
        warning
        disabled={disabled}
        onClick={() => onImmediateUnlockChange(!status?.immediate_unlock)}
      />
      {showFailureCooldown ? (
        <SettingRow
          id="failure-cooldown"
          icon="shield"
          tone="gray"
          title="认证失败冷却"
          description={status?.failure_cooldown_enabled
            ? "安全校验连续失败后暂停 5 分钟"
            : "已关闭，失败后会继续尝试（降低安全性）"}
          selected={Boolean(status?.failure_cooldown_enabled)}
          kind="switch"
          warning={!status?.failure_cooldown_enabled}
          disabled={disabled}
          onClick={() => onFailureCooldownChange(!status?.failure_cooldown_enabled)}
        />
      ) : null}
    </>
  );
  return (
    <section className={`panel settings-panel${compact ? " is-compact" : ""}${grouped ? " is-grouped" : ""}`} aria-labelledby="unlock-settings-heading">
      <div className="settings-heading">
        <h2 id="unlock-settings-heading">解锁设置</h2>
        {busy ? <span className="saving-label">正在保存…</span> : null}
      </div>
      <div className={`setting-list${compact ? " is-compact" : ""}${grouped ? " is-grouped" : ""}`} role="group" aria-label="解锁模式与锁定选项">
        {compact ? (
          <>
            <OverviewModeSelector mode={mode} disabled={disabled} onModeChange={onModeChange} />
            {autoLockRow}
            {highSensitivityRow}
            {dopplerRow}
          </>
        ) : grouped ? (
          <>
            <div className="settings-group">
              <span className="settings-group-title">身份验证</span>
              {modeRows}
            </div>
            <div className="settings-group">
              <span className="settings-group-title">自动化与响应</span>
              {advancedRows}
            </div>
          </>
        ) : (
          <>{modeRows}{advancedRows}</>
        )}
      </div>
      {showMore ? <button className="panel-link" type="button" onClick={onMore}>
        更多安全设置 <Icon name="arrow" size={17} />
      </button> : null}
    </section>
  );
}
