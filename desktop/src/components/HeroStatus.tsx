import { relativeAuthentication } from "../lib/format";
import type { ServiceStatus } from "../types";
import { ConnectionVisual } from "./ConnectionVisual";
import { Icon } from "./Icon";

interface HeroStatusProps {
  status: ServiceStatus | null;
  error: string | null;
  loading: boolean;
  onLock: () => void;
  onManageDevice: () => void;
}

function heroCopy(status: ServiceStatus | null, error: string | null) {
  if (error) return { title: "服务暂时不可用", detail: "请确认蓝牙解锁服务正在运行" };
  if (!status) return { title: "正在连接蓝牙解锁服务", detail: "正在读取安全状态" };
  if (!status.paired) return { title: "还没有配对手机", detail: "请打开设备页完成首次配对" };
  if (!status.has_rssi) return { title: "正在等待手机信号", detail: "请确认手机端蓝牙钥匙正在运行" };
  return {
    title: "手机已连接，解锁准备就绪",
    detail: `Android 手机 · 信号 ${status.median_rssi ?? "--"} dBm · ${relativeAuthentication(status.last_authenticated)}`,
  };
}

export function HeroStatus({ status, error, loading, onLock, onManageDevice }: HeroStatusProps) {
  const copy = heroCopy(status, error);
  const serviceHealthy = Boolean(status && !error);
  return (
    <section className="hero-panel" aria-labelledby="connection-title">
      <div className="hero-copy">
        <h1 id="connection-title">{copy.title}</h1>
        <p className="hero-detail">{copy.detail}</p>
        <div className="status-chips" aria-label="当前解锁状态">
          <span className={serviceHealthy ? "status-chip green" : "status-chip red"}>
            <Icon name={serviceHealthy ? "shield" : "pause"} size={17} />
            {loading ? "连接中" : serviceHealthy ? "服务正常" : "服务异常"}
          </span>
          <span className="status-chip blue">
            <Icon name="shield" size={17} />
            {status?.mode === "convenience" ? "便捷模式" : "安全模式"}
          </span>
          <span className={status?.auto_lock ? "status-chip green" : "status-chip neutral"}>
            <Icon name="lock" size={17} />
            {status?.auto_lock ? "自动锁定开启" : "自动锁定关闭"}
          </span>
          {status?.high_sensitivity ? (
            <span className="status-chip blue">
              <Icon name="signal" size={17} />
              高灵敏响应
            </span>
          ) : null}
          {status?.doppler_prediction ? (
            <span className="status-chip violet">
              <Icon name="signal" size={17} />
              趋势预测
            </span>
          ) : null}
        </div>
        <div className="hero-actions">
          <button className="primary-button" type="button" onClick={onLock}>
            <Icon name="lock" size={20} />
            立即锁定
          </button>
          <button className="text-button" type="button" onClick={onManageDevice}>
            管理设备
            <Icon name="arrow" size={18} />
          </button>
        </div>
      </div>
      <div className="hero-art" aria-hidden="true">
        <ConnectionVisual />
      </div>
    </section>
  );
}
