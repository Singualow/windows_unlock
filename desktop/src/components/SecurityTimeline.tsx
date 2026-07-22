import { formatClock } from "../lib/format";
import type { SecurityEvent } from "../types";
import { Icon } from "./Icon";

interface SecurityTimelineProps {
  events: SecurityEvent[];
  onOpenLogs: () => void;
}

export function SecurityTimeline({ events, onOpenLogs }: SecurityTimelineProps) {
  const visibleEvents = events.slice(-4);

  return (
    <section className="panel timeline-panel" aria-labelledby="events-heading">
      <div className="timeline-heading">
        <h2 id="events-heading">今天的安全事件</h2>
        <button className="panel-link" type="button" onClick={onOpenLogs}>打开日志 <Icon name="arrow" size={17} /></button>
      </div>
      {visibleEvents.length ? (
        <div className="timeline-track">
          {visibleEvents.map((event, index) => (
            <article className={`timeline-event ${event.tone}`} key={event.id}>
              <span className="event-icon"><Icon name={event.icon} size={22} /></span>
              <span className="event-copy"><time>{formatClock(event.at)}</time><strong>{event.title}</strong></span>
              {index < visibleEvents.length - 1 ? <span className="timeline-connector"><i /></span> : null}
            </article>
          ))}
        </div>
      ) : (
        <div className="timeline-empty">新的认证、授权和凭据提交事件会显示在这里</div>
      )}
    </section>
  );
}
