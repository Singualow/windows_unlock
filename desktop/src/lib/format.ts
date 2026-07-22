import type { SecurityEvent, ServiceStatus } from "../types";

const emptyTime = /^0001-01-01/;

export function hasTime(value?: string): value is string {
  return Boolean(value && !emptyTime.test(value) && !Number.isNaN(Date.parse(value)));
}

export function formatClock(value: Date | string | number): string {
  const date = value instanceof Date ? value : new Date(value);
  return date.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false });
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
  const events: SecurityEvent[] = [];
  const push = (
    id: string,
    value: string | undefined,
    title: string,
    tone: SecurityEvent["tone"],
    icon: SecurityEvent["icon"],
    warning = false,
  ) => {
    if (hasTime(value)) events.push({ id, at: new Date(value), title, tone, icon, warning });
  };
  push(
    "auth-failure",
    status.last_authentication_failure_at,
    `认证失败：${status.last_authentication_failure_reason || status.last_authentication_failure_code || "原因未知"}`,
    "violet",
    "shield",
    true,
  );
  push("proof", status.last_authenticated, "手机认证通过", "green", "shield");
  push("grant", status.authorization.last_granted_at, "授权已就绪", "blue", "key");
  push("consume", status.authorization.last_consume_at, "凭据安全提交", "violet", "credential");
  push(
    "signal",
    status.authorization.last_signal_at,
    status.authorization.last_signal_error ? "凭据通知需要检查" : "锁屏通知已送达",
    status.authorization.last_signal_error ? "violet" : "signal",
    "signal",
  );
  return events.sort((left, right) => left.at.getTime() - right.at.getTime()).slice(-4);
}

export function describeError(error: unknown): string {
  if (typeof error === "string") return error;
  if (error instanceof Error) return error.message;
  return "操作没有完成，请稍后重试";
}
