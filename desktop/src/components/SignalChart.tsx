import { formatClock } from "../lib/format";
import type { SignalPoint } from "../types";
import { Icon } from "./Icon";

interface SignalChartProps {
  points: SignalPoint[];
  current?: number;
  unlockThreshold?: number;
  thresholdLabel?: string;
  onCalibrate: () => void;
}

const bounds = { left: 68, right: 814, top: 18, bottom: 176 };
const historyMilliseconds = 10 * 60 * 1_000;

function yFor(value: number) {
  const clamped = Math.max(-85, Math.min(-35, value));
  return bounds.top + ((-35 - clamped) / 50) * (bounds.bottom - bounds.top);
}

function chartCoordinates(points: SignalPoint[], now: number) {
  if (!points.length) return [];
  const start = now - historyMilliseconds;
  return points.map((point, index) => ({
    x: bounds.left + Math.max(0, Math.min(1, (point.at - start) / historyMilliseconds)) * (bounds.right - bounds.left),
    y: yFor(point.value),
    at: point.at,
    value: point.value,
    index,
  }));
}

function smoothCoordinates(points: ReturnType<typeof chartCoordinates>) {
  if (points.length < 3) return points;

  const kernel = [1, 4, 6, 4, 1];
  const radius = Math.floor(kernel.length / 2);
  const smoothed = points.map((point, index) => {
    let weightedY = 0;
    let totalWeight = 0;
    for (let offset = -radius; offset <= radius; offset += 1) {
      const neighbor = points[index + offset];
      if (!neighbor) continue;
      const weight = kernel[offset + radius];
      weightedY += neighbor.y * weight;
      totalWeight += weight;
    }
    return { ...point, y: weightedY / totalWeight };
  });

  const anchorEndpoint = (atStart: boolean) => {
    const anchorIndex = atStart ? 0 : smoothed.length - 1;
    const correction = points[anchorIndex].y - smoothed[anchorIndex].y;
    const anchorCount = Math.min(6, smoothed.length);
    for (let step = 0; step < anchorCount; step += 1) {
      const progress = anchorCount === 1 ? 1 : step / (anchorCount - 1);
      const eased = progress * progress * (3 - 2 * progress);
      const index = atStart ? step : smoothed.length - anchorCount + step;
      const influence = atStart ? 1 - eased : eased;
      smoothed[index] = {
        ...smoothed[index],
        y: smoothed[index].y + correction * influence,
      };
    }
  };

  anchorEndpoint(true);
  anchorEndpoint(false);
  return smoothed;
}

function smoothPath(points: ReturnType<typeof chartCoordinates>) {
  if (!points.length) return "";
  if (points.length === 1) return `M ${points[0].x} ${points[0].y}`;

  let path = `M ${points[0].x.toFixed(1)} ${points[0].y.toFixed(1)}`;
  for (let index = 0; index < points.length - 1; index += 1) {
    const before = points[index - 1] ?? points[index];
    const start = points[index];
    const end = points[index + 1];
    const after = points[index + 2] ?? end;
    const width = end.x - start.x;
    if (width <= 0) {
      path += ` L ${end.x.toFixed(1)} ${end.y.toFixed(1)}`;
      continue;
    }

    const controlOffset = width / 3;
    const startSpan = end.x - before.x;
    const endSpan = after.x - start.x;
    const startSlope = startSpan > 0 ? (end.y - before.y) / startSpan : 0;
    const endSlope = endSpan > 0 ? (after.y - start.y) / endSpan : 0;
    const localMinimum = Math.min(before.y, start.y, end.y, after.y);
    const localMaximum = Math.max(before.y, start.y, end.y, after.y);
    const control1X = start.x + controlOffset;
    const control1Y = Math.max(localMinimum, Math.min(localMaximum, start.y + startSlope * controlOffset));
    const control2X = end.x - controlOffset;
    const control2Y = Math.max(localMinimum, Math.min(localMaximum, end.y - endSlope * controlOffset));
    path += ` C ${control1X.toFixed(1)} ${control1Y.toFixed(1)}, ${control2X.toFixed(1)} ${control2Y.toFixed(1)}, ${end.x.toFixed(1)} ${end.y.toFixed(1)}`;
  }
  return path;
}

export function SignalChart({ points, current, unlockThreshold = -65, thresholdLabel = "解锁阈值", onCalibrate }: SignalChartProps) {
  const now = Date.now();
  const coordinates = smoothCoordinates(chartCoordinates(points, now));
  const line = smoothPath(coordinates);
  const area = line ? `${line} L ${bounds.right} ${bounds.bottom} L ${bounds.left} ${bounds.bottom} Z` : "";
  const last = coordinates.at(-1);
  const timeLabels = [now - historyMilliseconds, now - historyMilliseconds / 2, now];
  const chartWidth = 900;
  const chartHeight = 220;
  const topPercent = (value: number) => `${(value / chartHeight) * 100}%`;
  const leftPercent = (value: number) => `${(value / chartWidth) * 100}%`;

  return (
    <section className="panel signal-panel" aria-labelledby="signal-heading">
      <div className="panel-heading signal-heading-row">
        <div>
          <h2 id="signal-heading">距离趋势</h2>
          <p>滚动 10 分钟 · 每 2 秒采样</p>
        </div>
        {typeof current === "number" ? <span className="current-signal">{current} dBm</span> : null}
      </div>
      <div className="chart-wrap">
        <svg className="signal-chart" viewBox="0 0 900 220" preserveAspectRatio="none" role="img" aria-label="手机蓝牙信号强度趋势">
          <defs>
            <linearGradient id="signalArea" x1="0" y1="0" x2="0" y2="1">
              <stop stopColor="#2d7fff" stopOpacity=".19" />
              <stop offset="1" stopColor="#2d7fff" stopOpacity=".02" />
            </linearGradient>
          </defs>
          {[-40, -50, -60, -70, -80].map((value) => {
            const y = yFor(value);
            return (
              <g key={value}>
                <line x1={bounds.left} y1={y} x2={bounds.right} y2={y} className="grid-line" />
              </g>
            );
          })}
          <line x1={bounds.left} y1={yFor(unlockThreshold)} x2={bounds.right} y2={yFor(unlockThreshold)} className="threshold-line" />
          {area ? <path d={area} fill="url(#signalArea)" /> : null}
          {line ? <path d={line} className="signal-line" /> : null}
        </svg>
        {last ? (
          <span
            className="signal-current-marker"
            style={{ left: leftPercent(last.x), top: topPercent(last.y) }}
            aria-hidden="true"
          />
        ) : null}
        <div className="chart-label-layer" aria-hidden="true">
          {[-40, -50, -60, -70, -80].map((value) => (
            <span className="chart-label axis-overlay" key={value} style={{ top: topPercent(yFor(value)) }}>{value} dBm</span>
          ))}
          <span
            className="chart-label threshold-overlay"
            style={{ left: leftPercent(bounds.right + 14), top: topPercent(yFor(unlockThreshold)) }}
          >
            <strong>{thresholdLabel}</strong>
            <span>{unlockThreshold} dBm</span>
          </span>
          {timeLabels.map((value, index) => (
            <span
              className={`chart-label time-overlay time-${index === 0 ? "start" : index === 1 ? "middle" : "end"}`}
              key={index}
              style={{ left: leftPercent(index === 0 ? bounds.left : index === 1 ? (bounds.left + bounds.right) / 2 : bounds.right) }}
            >
              {formatClock(value)}
            </span>
          ))}
        </div>
        {!points.length ? <div className="chart-empty">正在收集蓝牙信号样本…</div> : null}
      </div>
      <div className="distance-controls">
        <div className="distance-scale" aria-label="当前距离估计">
          <div className="distance-track">
            <span className="distance-near" />
            <span className="distance-medium" />
            <span className="distance-far" />
            <span className="distance-marker" style={{ left: `${Math.max(3, Math.min(97, ((-35 - (current ?? -65)) / 50) * 100))}%` }} />
          </div>
          <div className="distance-labels"><span>近</span><span>适中</span><span>远</span></div>
        </div>
        <button className="outline-button" type="button" onClick={onCalibrate}>
          <Icon name="calibrate" size={19} />
          重新校准
        </button>
      </div>
    </section>
  );
}
