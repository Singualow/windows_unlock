import type { ServiceStatus, UnlockMode } from "../types";
import { Icon, type IconName } from "./Icon";

interface UnlockSettingsProps {
  status: ServiceStatus | null;
  busy: string | null;
  onModeChange: (mode: UnlockMode) => void;
  onAutoLockChange: (enabled: boolean) => void;
  onImmediateUnlockChange: (enabled: boolean) => void;
  onFailureCooldownChange: (enabled: boolean) => void;
  onMore: () => void;
  showMore?: boolean;
  showFailureCooldown?: boolean;
}

interface SettingRowProps {
  id: string;
  icon: IconName;
  tone: "blue" | "gray" | "green";
  title: string;
  description: string;
  selected: boolean;
  kind: "radio" | "switch";
  warning?: boolean;
  disabled?: boolean;
  onClick: () => void;
}

function SettingRow({ id, icon, tone, title, description, selected, kind, warning, disabled, onClick }: SettingRowProps) {
  return (
    <button
      aria-checked={selected}
      className={`setting-row${selected && kind === "radio" ? " is-selected" : ""}`}
      disabled={disabled}
      id={id}
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
  );
}

export function UnlockSettings({
  status,
  busy,
  onModeChange,
  onAutoLockChange,
  onImmediateUnlockChange,
  onFailureCooldownChange,
  onMore,
  showMore = true,
  showFailureCooldown = false,
}: UnlockSettingsProps) {
  const mode = status?.mode ?? "strict";
  const disabled = !status || Boolean(busy);
  return (
    <section className="panel settings-panel" aria-labelledby="unlock-settings-heading">
      <div className="settings-heading">
        <h2 id="unlock-settings-heading">解锁设置</h2>
        {busy ? <span className="saving-label">正在保存…</span> : null}
      </div>
      <div className="setting-list" role="radiogroup" aria-label="解锁模式与锁定选项">
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
      </div>
      {showMore ? <button className="panel-link" type="button" onClick={onMore}>
        更多安全设置 <Icon name="arrow" size={17} />
      </button> : null}
    </section>
  );
}
