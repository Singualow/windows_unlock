import { useEffect, useState, type FormEvent } from "react";

interface ThresholdEditorProps {
  unlockRssi: number;
  lockRssi: number;
  busy: boolean;
  onApply: (unlockRssi: number, lockRssi: number) => void;
}

function validateThresholds(unlockRssi: number, lockRssi: number): string | null {
  if (!Number.isInteger(unlockRssi) || !Number.isInteger(lockRssi)) return "请输入整数 dBm 数值";
  if (unlockRssi < -90 || unlockRssi > -20) return "解锁阈值范围为 -90 至 -20 dBm";
  if (lockRssi < -120 || lockRssi > -28) return "锁定阈值范围为 -120 至 -28 dBm";
  if (unlockRssi - lockRssi < 8) return "解锁阈值必须比锁定阈值至少高 8 dB";
  return null;
}

export function ThresholdEditor({
  unlockRssi,
  lockRssi,
  busy,
  onApply,
}: ThresholdEditorProps) {
  const [unlockValue, setUnlockValue] = useState(String(unlockRssi));
  const [lockValue, setLockValue] = useState(String(lockRssi));

  useEffect(() => setUnlockValue(String(unlockRssi)), [unlockRssi]);
  useEffect(() => setLockValue(String(lockRssi)), [lockRssi]);

  const parsedUnlock = Number(unlockValue);
  const parsedLock = Number(lockValue);
  const error = validateThresholds(parsedUnlock, parsedLock);
  const changed = parsedUnlock !== unlockRssi
    || parsedLock !== lockRssi;

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (error || !changed || busy) return;
    onApply(parsedUnlock, parsedLock);
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
      <button className="secondary-button" type="submit" disabled={Boolean(error) || !changed || busy}>应用</button>
      <span className={error ? "threshold-help is-error" : "threshold-help"} role={error ? "alert" : undefined}>
        {error ?? "普通模式使用双阈值，至少保留 8 dB 防抖空间"}
      </span>
    </form>
  );
}
