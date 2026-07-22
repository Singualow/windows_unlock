import { formatClock } from "../lib/format";
import type { SecurityEvent, ServiceStatus } from "../types";
import { Icon } from "./Icon";

interface LogsPageProps {
  events: SecurityEvent[];
  status: ServiceStatus | null;
}

export function LogsPage({ events, status }: LogsPageProps) {
  return (
    <main className="secondary-page">
      <div className="page-title-row"><div><h1>日志</h1><p>只记录安全状态，不记录密码、私钥或完整设备标识</p></div></div>
      <section className="panel log-table-panel">
        <div className="log-table-heading"><h2>最近安全事件</h2><span>本次运行</span></div>
        <div className="log-list" role="list">
          {[...events].reverse().map((event) => (
            <article className="log-row" key={event.id} role="listitem">
              <span className={`log-icon ${event.tone}`}><Icon name={event.icon} size={21} /></span>
              <span className="log-event-copy"><strong>{event.title}</strong><small>蓝牙解锁服务</small></span>
              <time>{formatClock(event.at)}</time>
              <span className={event.warning ? "log-result is-warning" : "log-result"}>{event.warning ? "需检查" : "正常"}</span>
            </article>
          ))}
          {!events.length ? <div className="log-empty">当前还没有可显示的安全事件</div> : null}
        </div>
      </section>
      <section className="panel diagnostics-panel">
        <div><span>BLE 后端</span><strong>{status?.ble_backend || "未知"}</strong></div>
        <div><span>会话状态</span><strong>{status?.session_active ? (status.locked ? "已锁定" : "桌面活动") : "未登录"}</strong></div>
        <div><span>凭据状态</span><strong>{status?.credential_valid ? "有效" : "需要更新"}</strong></div>
        <div><span>授权队列</span><strong>{status?.authorization.ready ? "授权待消费" : "空闲"}</strong></div>
        <div><span>失败冷却</span><strong>{status?.failure_cooldown_enabled ? "已开启" : "已关闭"}</strong></div>
      </section>
    </main>
  );
}
