import type { SecurityEvent, ServiceLogEntry, ServiceStatus } from "../types";

const emptyTime = /^0001-01-01/;

export function hasTime(value?: string): value is string {
  return Boolean(value && !emptyTime.test(value) && !Number.isNaN(Date.parse(value)));
}

export function formatClock(value: Date | string | number): string {
  const date = value instanceof Date ? value : new Date(value);
  return date.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false });
}

export function formatLogTime(value: Date | string | number): string {
  const date = value instanceof Date ? value : new Date(value);
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
}

export function relativeAuthentication(value?: string): string {
  if (!hasTime(value)) return "等待认证";
  const elapsed = Date.now() - Date.parse(value);
  if (elapsed < 60_000) return "刚刚认证";
  if (elapsed < 3_600_000) return `${Math.max(1, Math.floor(elapsed / 60_000))} 分钟前认证`;
  return `${formatClock(value)} 认证`;
}

export function buildSecurityEvents(status: ServiceStatus | null): SecurityEvent[] {
  if (!status) return [];
  if (status.recent_events) {
    return status.recent_events
      .filter((event) => hasTime(event.at))
      .slice(-100)
      .map((event) => {
        const presentation = eventPresentation(event.kind, event.warning);
        return {
          id: `service-${event.id}`,
          at: new Date(event.at),
          title: event.message,
          detail: event.detail || `事件代码：${event.code}`,
          result: event.result,
          tone: presentation.tone,
          icon: presentation.icon,
          warning: event.warning,
        };
      });
  }

  const events: SecurityEvent[] = [];
  const push = (
    id: string,
    value: string | undefined,
    title: string,
    tone: SecurityEvent["tone"],
    icon: SecurityEvent["icon"],
    warning = false,
    detail?: string,
    result?: string,
  ) => {
    if (hasTime(value)) events.push({ id, at: new Date(value), title, tone, icon, warning, detail, result });
  };
  push(
    "auth-failure",
    status.last_authentication_failure_at,
    `认证失败：${status.last_authentication_failure_reason || status.last_authentication_failure_code || "原因未知"}`,
    "violet",
    "shield",
    true,
    status.last_authentication_failure_code
      ? `认证代码：${status.last_authentication_failure_code}`
      : "手机认证没有完成",
    "失败",
  );
  push("proof", status.last_authenticated, "手机认证通过", "green", "shield", false, "已验证手机签名", "成功");
  push("grant", status.authorization.last_granted_at, "授权已就绪", "blue", "key", false, "一次性解锁授权已生成", "成功");
  push("consume", status.authorization.last_consume_at, "凭据安全提交", "violet", "credential", false, "凭据提供程序已消费授权", "成功");
  push(
    "signal",
    status.authorization.last_signal_at,
    status.authorization.last_signal_error ? "凭据通知需要检查" : "锁屏通知已送达",
    status.authorization.last_signal_error ? "violet" : "signal",
    "signal",
    Boolean(status.authorization.last_signal_error),
    status.authorization.last_signal_error || "已通知 Windows 锁屏界面刷新凭据",
    status.authorization.last_signal_error ? "失败" : "成功",
  );
  return events.sort((left, right) => left.at.getTime() - right.at.getTime());
}

function eventPresentation(
  kind: ServiceLogEntry["kind"],
  warning: boolean,
): Pick<SecurityEvent, "tone" | "icon"> {
  switch (kind) {
    case "authentication":
      return { tone: warning ? "violet" : "green", icon: "shield" };
    case "authorization":
      return { tone: "blue", icon: "key" };
    case "credential":
      return { tone: warning ? "violet" : "green", icon: "credential" };
    case "session":
      return { tone: "signal", icon: "signal" };
    case "configuration":
    case "pairing":
    case "service":
    default:
      return { tone: warning ? "violet" : "blue", icon: "signal" };
  }
}

export function describeError(error: unknown): string {
  if (typeof error === "string") return error;
  if (error instanceof Error) return error.message;
  return "操作没有完成，请稍后重试";
}
