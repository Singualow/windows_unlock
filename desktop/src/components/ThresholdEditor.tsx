import { useEffect, useState, type FormEvent } from "react";

interface ThresholdEditorProps {
  unlockRssi: number;
  lockRssi: number;
  highSensitivityRssi: number;
  highSensitivityEnabled: boolean;
  busy: boolean;
  onApply: (unlockRssi: number, lockRssi: number, highSensitivityRssi: number) => void;
}

function validateThresholds(unlockRssi: number, lockRssi: number, highSensitivityRssi: number): string | null {
  if (!Number.isInteger(unlockRssi) || !Number.isInteger(lockRssi) || !Number.isInteger(highSensitivityRssi)) return "请输入整数 dBm 数值";
  if (unlockRssi < -90 || unlockRssi > -20) return "解锁阈值范围为 -90 至 -20 dBm";
  if (lockRssi < -120 || lockRssi > -28) return "锁定阈值范围为 -120 至 -28 dBm";
  if (unlockRssi - lockRssi < 8) return "解锁阈值必须比锁定阈值至少高 8 dB";
  if (highSensitivityRssi < -90 || highSensitivityRssi > -20) return "高灵敏触发阈值范围为 -90 至 -20 dBm";
  return null;
}

export function ThresholdEditor({
  unlockRssi,
  lockRssi,
  highSensitivityRssi,
  highSensitivityEnabled,
  busy,
  onApply,
}: ThresholdEditorProps) {
  const [unlockValue, setUnlockValue] = useState(String(unlockRssi));
  const [lockValue, setLockValue] = useState(String(lockRssi));
  const [highSensitivityValue, setHighSensitivityValue] = useState(String(highSensitivityRssi));

  useEffect(() => setUnlockValue(String(unlockRssi)), [unlockRssi]);
  useEffect(() => setLockValue(String(lockRssi)), [lockRssi]);
  useEffect(() => setHighSensitivityValue(String(highSensitivityRssi)), [highSensitivityRssi]);

  const parsedUnlock = Number(unlockValue);
  const parsedLock = Number(lockValue);
  const parsedHighSensitivity = Number(highSensitivityValue);
  const error = validateThresholds(parsedUnlock, parsedLock, parsedHighSensitivity);
  const changed = parsedUnlock !== unlockRssi
    || parsedLock !== lockRssi
    || parsedHighSensitivity !== highSensitivityRssi;

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (error || !changed || busy) return;
    onApply(parsedUnlock, parsedLock, parsedHighSensitivity);
  }

  return (
    <form className="threshold-editor" onSubmit={submit}>
      <label className="threshold-field">
        <span>解锁阈值</span>
        <span className="threshold-input-wrap">
          <input
            aria-label="解锁阈值"
            inputMode="numeric"
            max={-20}
            min={-90}
            onChange={(event) => setUnlockValue(event.target.value)}
            step={1}
            type="number"
            value={unlockValue}
          />
          <small>dBm</small>
        </span>
      </label>
      <label className="threshold-field">
        <span>锁定阈值</span>
        <span className="threshold-input-wrap">
          <input
            aria-label="锁定阈值"
            inputMode="numeric"
            max={-28}
            min={-120}
            onChange={(event) => setLockValue(event.target.value)}
            step={1}
            type="number"
            value={lockValue}
          />
          <small>dBm</small>
        </span>
      </label>
      <label className="threshold-field high-sensitivity-threshold">
        <span>高灵敏触发阈值{highSensitivityEnabled ? " · 使用中" : ""}</span>
        <span className="threshold-input-wrap">
          <input
            aria-label="高灵敏触发阈值"
            inputMode="numeric"
            max={-20}
            min={-90}
            onChange={(event) => setHighSensitivityValue(event.target.value)}
            step={1}
            type="number"
            value={highSensitivityValue}
          />
          <small>dBm</small>
        </span>
      </label>
      <button className="secondary-button" type="submit" disabled={Boolean(error) || !changed || busy}>应用</button>
      <span className={error ? "threshold-help is-error" : "threshold-help"} role={error ? "alert" : undefined}>
        {error ?? `高灵敏模式靠近达到 ${parsedHighSensitivity} dBm 即认证，离开保护线自动设为 ${parsedHighSensitivity - 8} dBm`}
      </span>
    </form>
  );
}
