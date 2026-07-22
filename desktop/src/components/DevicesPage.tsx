import type { ServiceStatus } from "../types";
import { Icon } from "./Icon";

interface DevicesPageProps {
  status: ServiceStatus | null;
  qrCode: string | null;
  pairingExpiresAt: string | null;
  busy: string | null;
  onStartPairing: () => void;
  onRevoke: () => void;
  onCalibrate: () => void;
}

export function DevicesPage({ status, qrCode, pairingExpiresAt, busy, onStartPairing, onRevoke, onCalibrate }: DevicesPageProps) {
  return (
    <main className="secondary-page">
      <div className="page-title-row">
        <div><h1>设备</h1><p>管理已配对手机、蓝牙连接和距离阈值</p></div>
        <span className={status?.paired ? "page-status is-online" : "page-status"}>
          <i />{status?.paired ? "手机已配对" : "等待配对"}
        </span>
      </div>
      <div className="device-grid">
        <section className="panel paired-device-card">
          <div className="device-illustration"><Icon name="device" size={38} /><span><Icon name="bluetooth" size={22} /></span></div>
          <div className="device-main-copy">
            <h2>{status?.paired ? "Android 手机" : "尚未添加手机"}</h2>
            <p>{status?.paired ? "已保存两种模式的不可导出公钥" : "使用手机端蓝牙解锁应用扫描配对二维码"}</p>
          </div>
          <dl className="device-facts">
            <div><dt>蓝牙后端</dt><dd>{status?.ble_backend || "等待服务"}</dd></div>
            <div><dt>当前信号</dt><dd>{status?.has_rssi ? `${status.median_rssi} dBm` : "未发现"}</dd></div>
            <div><dt>认证状态</dt><dd>{status?.credential_valid ? "可用" : "需要更新 Windows 密码"}</dd></div>
          </dl>
          <div className="card-actions">
            <button className="primary-button" type="button" disabled={Boolean(busy)} onClick={onStartPairing}>
              <Icon name="device" size={19} />{status?.paired ? "重新配对" : "添加手机"}
            </button>
            {status?.paired ? <button className="danger-text-button" type="button" disabled={Boolean(busy)} onClick={onRevoke}>撤销设备</button> : null}
          </div>
        </section>
        <section className="panel pairing-card">
          {qrCode ? (
            <>
              <div className="qr-frame"><img src={qrCode} alt="两分钟有效的手机配对二维码" /></div>
              <h2>用手机扫描二维码</h2>
              <p>请保持手机解锁并靠近电脑。二维码只在本机显示，不会上传。</p>
              <span className="expiry-label">{pairingExpiresAt ? `${new Date(pairingExpiresAt).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })} 前有效` : "两分钟内有效"}</span>
            </>
          ) : (
            <div className="pairing-empty">
              <span className="pairing-empty-icon"><Icon name="shield" size={30} /></span>
              <h2>端到端安全配对</h2>
              <p>配对二维码包含一次性密钥和电脑公钥，两分钟后自动失效。</p>
            </div>
          )}
        </section>
        <section className="panel calibration-card">
          <div className="section-icon blue"><Icon name="calibrate" size={24} /></div>
          <div><h2>距离校准</h2><p>按你的房间和设备环境重新计算解锁、锁定阈值。</p></div>
          <div className="threshold-summary">
            <span><small>解锁阈值</small><strong>{status?.unlock_rssi ?? -65} dBm</strong></span>
            <span><small>锁定阈值</small><strong>{status?.lock_rssi ?? -80} dBm</strong></span>
          </div>
          <button className="outline-button" type="button" disabled={!status?.has_rssi || Boolean(busy)} onClick={onCalibrate}>
            <Icon name="calibrate" size={18} />开始校准
          </button>
        </section>
      </div>
    </main>
  );
}
