import type { ServiceStatus, SystemIntegration, UnlockMode } from "../types";
import { Icon } from "./Icon";
import { UnlockSettings } from "./UnlockSettings";

interface SettingsPageProps {
  status: ServiceStatus | null;
  busy: string | null;
  onModeChange: (mode: UnlockMode) => void;
  onAutoLockChange: (enabled: boolean) => void;
  onHighSensitivityChange: (enabled: boolean) => void;
  onImmediateUnlockChange: (enabled: boolean) => void;
  onFailureCooldownChange: (enabled: boolean) => void;
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
          onImmediateUnlockChange={props.onImmediateUnlockChange}
          onFailureCooldownChange={props.onFailureCooldownChange}
          onMore={() => undefined}
          showMore={false}
          showFailureCooldown
        />
        <section className="panel service-control-panel">
          <div className="section-icon green"><Icon name="pause" size={24} /></div>
          <div className="service-control-copy">
            <h2>临时暂停</h2>
            <p>暂停期间不会自动解锁或锁定，系统登录方式不受影响。</p>
          </div>
          <span className={paused ? "pause-state is-paused" : "pause-state"}>{paused ? "已暂停" : "正常运行"}</span>
          <div className="pause-actions">
            <button className="secondary-button" type="button" disabled={Boolean(props.busy)} onClick={() => props.onPause(300)}>暂停 5 分钟</button>
            <button className="outline-button" type="button" disabled={!paused || Boolean(props.busy)} onClick={() => props.onPause(0)}>立即恢复</button>
          </div>
        </section>
        <section className="panel privacy-panel">
          <Icon name="shield" size={25} />
          <div><h2>隐私保护</h2><p>界面与日志不会显示密码、私钥、完整签名或可追踪手机标识。</p></div>
        </section>
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
