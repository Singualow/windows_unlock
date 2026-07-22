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

function yFor(value: number) {
  const clamped = Math.max(-85, Math.min(-35, value));
  return bounds.top + ((-35 - clamped) / 50) * (bounds.bottom - bounds.top);
}

function chartCoordinates(points: SignalPoint[]) {
  if (!points.length) return [];
  const first = points[0].at;
  const last = points.at(-1)?.at ?? first;
  const span = Math.max(1, last - first);
  return points.map((point, index) => ({
    x:
      points.length === 1
        ? bounds.right
        : bounds.left + ((point.at - first) / span) * (bounds.right - bounds.left),
    y: yFor(point.value),
    at: point.at,
    value: point.value,
    index,
  }));
}

function smoothPath(points: ReturnType<typeof chartCoordinates>) {
  if (!points.length) return "";
  if (points.length === 1) return `M ${points[0].x} ${points[0].y}`;
  let path = `M ${points[0].x.toFixed(1)} ${points[0].y.toFixed(1)}`;
  for (let index = 1; index < points.length; index += 1) {
    const previous = points[index - 1];
    const current = points[index];
    const middle = (previous.x + current.x) / 2;
    path += ` C ${middle.toFixed(1)} ${previous.y.toFixed(1)}, ${middle.toFixed(1)} ${current.y.toFixed(1)}, ${current.x.toFixed(1)} ${current.y.toFixed(1)}`;
  }
  return path;
}

export function SignalChart({ points, current, unlockThreshold = -65, thresholdLabel = "解锁阈值", onCalibrate }: SignalChartProps) {
  const coordinates = chartCoordinates(points);
  const line = smoothPath(coordinates);
  const area = line ? `${line} L ${bounds.right} ${bounds.bottom} L ${bounds.left} ${bounds.bottom} Z` : "";
  const last = coordinates.at(-1);
  const timeLabels = points.length
    ? [points[0], points[Math.floor((points.length - 1) / 2)], points.at(-1) as SignalPoint]
    : [];

  return (
    <section className="panel signal-panel" aria-labelledby="signal-heading">
      <div className="panel-heading signal-heading-row">
        <div>
          <h2 id="signal-heading">距离趋势</h2>
          <p>最近 10 分钟</p>
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
                <text x="0" y={y + 4} className="axis-label">{value} dBm</text>
              </g>
            );
          })}
          <line x1={bounds.left} y1={yFor(unlockThreshold)} x2={bounds.right} y2={yFor(unlockThreshold)} className="threshold-line" />
          <text x={bounds.right + 14} y={yFor(unlockThreshold) - 3} className="threshold-label">{thresholdLabel}</text>
          <text x={bounds.right + 14} y={yFor(unlockThreshold) + 15} className="threshold-value">{unlockThreshold} dBm</text>
          {area ? <path d={area} fill="url(#signalArea)" /> : null}
          {line ? <path d={line} className="signal-line" /> : null}
          {last ? (
            <>
              <circle cx={last.x} cy={last.y} r="8" className="signal-dot-halo" />
              <circle cx={last.x} cy={last.y} r="4.5" className="signal-dot" />
            </>
          ) : null}
          {timeLabels.map((point, index) => (
            <text
              className="time-label"
              key={`${point.at}-${index}`}
              x={index === 0 ? bounds.left : index === 1 ? (bounds.left + bounds.right) / 2 : bounds.right}
              y="208"
              textAnchor={index === 0 ? "start" : index === 1 ? "middle" : "end"}
            >
              {formatClock(point.at)}
            </text>
          ))}
        </svg>
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
