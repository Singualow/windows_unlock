import type { ServiceStatus, SystemIntegration, UnlockMode } from "../types";
import { Icon } from "./Icon";
import { UnlockSettings } from "./UnlockSettings";

interface SettingsPageProps {
  status: ServiceStatus | null;
  busy: string | null;
  onModeChange: (mode: UnlockMode) => void;
  onAutoLockChange: (enabled: boolean) => void;
  onHighSensitivityChange: (enabled: boolean) => void;
  onDopplerPredictionChange: (enabled: boolean) => void;
  onImmediateUnlockChange: (enabled: boolean) => void;
  onFailureCooldownChange: (enabled: boolean) => void;
  onHighSensitivityParameters: () => void;
  onDopplerParameters: () => void;
  onPause: (seconds: number) => void;
  systemIntegration: SystemIntegration | null;
  onCredentialProvider: () => void;
  onUpdatePassword: () => void;
  onUninstall: () => void;
}

export function SettingsPage(props: SettingsPageProps) {
  const paused = Boolean(props.status?.paused_until && new Date(props.status.paused_until) > new Date());
  return (
    <main className="secondary-page">
      <div className="page-title-row"><div><h1>设置</h1><p>调整解锁安全性和自动锁定行为</p></div></div>
      <div className="settings-page-grid">
        <UnlockSettings
          status={props.status}
          busy={props.busy}
          onModeChange={props.onModeChange}
          onAutoLockChange={props.onAutoLockChange}
          onHighSensitivityChange={props.onHighSensitivityChange}
          onDopplerPredictionChange={props.onDopplerPredictionChange}
          onImmediateUnlockChange={props.onImmediateUnlockChange}
          onFailureCooldownChange={props.onFailureCooldownChange}
          onHighSensitivityParameters={props.onHighSensitivityParameters}
          onDopplerParameters={props.onDopplerParameters}
          onMore={() => undefined}
          grouped
          showMore={false}
          showFailureCooldown
        />
        <div className="settings-side-stack">
          <section className="panel compact-setting-card pause-card">
            <div className="compact-card-heading">
              <span className="section-icon green"><Icon name="pause" size={22} /></span>
              <span><strong>临时暂停</strong><small>暂停自动解锁与锁定</small></span>
              <span className={paused ? "pause-state is-paused" : "pause-state"}>{paused ? "已暂停" : "正常运行"}</span>
            </div>
            <p>系统 PIN、密码和 Windows Hello 不受影响。</p>
            <div className="compact-card-actions">
              <button className="secondary-button" type="button" disabled={Boolean(props.busy)} onClick={() => props.onPause(300)}>暂停 5 分钟</button>
              <button className="outline-button" type="button" disabled={!paused || Boolean(props.busy)} onClick={() => props.onPause(0)}>立即恢复</button>
            </div>
          </section>
          <section className="panel compact-setting-card runtime-card">
            <div className="compact-card-heading">
              <span className="section-icon blue"><Icon name="signal" size={22} /></span>
              <span><strong>运行状态</strong><small>当前蓝牙与认证状态</small></span>
            </div>
            <dl className="runtime-facts">
              <div><dt>服务</dt><dd>{props.status ? "运行中" : "连接中"}</dd></div>
              <div><dt>手机信号</dt><dd>{props.status?.has_rssi ? `${props.status.median_rssi} dBm` : "未发现"}</dd></div>
              <div><dt>认证</dt><dd>{props.status?.credential_valid ? "凭据有效" : "需要更新"}</dd></div>
            </dl>
          </section>
          <section className="panel compact-setting-card privacy-card">
            <div className="compact-card-heading">
              <span className="section-icon violet"><Icon name="shield" size={22} /></span>
              <span><strong>隐私保护</strong><small>敏感信息不进入界面和日志</small></span>
            </div>
            <p>不显示密码、私钥、完整签名或可追踪手机标识。</p>
          </section>
        </div>
        <section className="panel maintenance-panel">
          <div className="maintenance-heading">
            <div className="section-icon blue"><Icon name="settings" size={23} /></div>
            <div><h2>系统维护</h2><p>需要管理员确认的操作会通过 Windows 安全对话框完成。</p></div>
          </div>
          <div className="maintenance-actions">
            <div>
              <span><strong>Windows 锁屏自动解锁组件</strong><small>PIN、密码和 Windows Hello 始终保留</small></span>
              <button className="outline-button" type="button" disabled={Boolean(props.busy)} onClick={props.onCredentialProvider}>
                {props.systemIntegration?.credential_provider_registered ? "停用组件" : "启用组件"}
              </button>
            </div>
            <div>
              <span><strong>Windows 账户密码</strong><small>密码仅保存到 LSA 私密数据</small></span>
              <button className="secondary-button" type="button" disabled={Boolean(props.busy)} onClick={props.onUpdatePassword}>更新密码</button>
            </div>
            <div className="danger-maintenance-row">
              <span><strong>卸载蓝牙解锁</strong><small>移除服务、配对、密钥和启动项</small></span>
              <button className="danger-text-button" type="button" disabled={Boolean(props.busy)} onClick={props.onUninstall}>卸载软件</button>
            </div>
          </div>
        </section>
      </div>
    </main>
  );
}
