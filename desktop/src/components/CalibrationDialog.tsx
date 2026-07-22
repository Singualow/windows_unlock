import { useEffect, useRef, useState } from "react";
import { getStatus, setThresholds } from "../lib/backend";
import { describeError } from "../lib/format";
import { Icon } from "./Icon";

type Step = "intro" | "near" | "move" | "far" | "result";

interface CalibrationDialogProps {
  open: boolean;
  onClose: () => void;
  onApplied: () => void;
}

function median(values: number[]) {
  const sorted = [...values].sort((left, right) => left - right);
  return sorted[Math.floor(sorted.length / 2)];
}

function clamp(value: number, minimum: number, maximum: number) {
  return Math.max(minimum, Math.min(maximum, value));
}

export function CalibrationDialog({ open, onClose, onApplied }: CalibrationDialogProps) {
  const [step, setStep] = useState<Step>("intro");
  const [progress, setProgress] = useState(0);
  const [near, setNear] = useState<number | null>(null);
  const [far, setFar] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const generation = useRef(0);

  useEffect(() => {
    if (open) {
      setStep("intro");
      setProgress(0);
      setNear(null);
      setFar(null);
      setError(null);
    } else {
      generation.current += 1;
    }
  }, [open]);

  async function collect(target: "near" | "far") {
    const token = ++generation.current;
    setStep(target);
    setProgress(0);
    setError(null);
    const samples: number[] = [];
    for (let index = 0; index < 16; index += 1) {
      if (generation.current !== token) return;
      try {
        const status = await getStatus();
        if (status.has_rssi && typeof status.median_rssi === "number") samples.push(status.median_rssi);
      } catch {
        // Keep collecting; a short transient service error should not abort the full sample window.
      }
      setProgress(Math.round(((index + 1) / 16) * 100));
      await new Promise((resolve) => window.setTimeout(resolve, 500));
    }
    if (samples.length < 5) {
      setError("有效蓝牙样本不足，请保持手机端蓝牙钥匙运行后重试。 ");
      setStep(target === "near" ? "intro" : "move");
      return;
    }
    const value = median(samples);
    if (target === "near") {
      setNear(value);
      setStep("move");
      return;
    }
    setFar(value);
    if (near === null || near - value < 8) {
      setError(`近、远样本只相差 ${near === null ? 0 : near - value} dB，请把手机移得更远后重试。`);
      setStep("move");
      return;
    }
    setStep("result");
  }

  async function applyResult() {
    if (near === null || far === null) return;
    const unlock = clamp(near - 2, -90, -35);
    const lock = clamp(far + 2, -110, unlock - 8);
    try {
      setError(null);
      await setThresholds(unlock, lock);
      onApplied();
      onClose();
    } catch (nextError) {
      setError(describeError(nextError));
    }
  }

  if (!open) return null;
  const unlock = near === null ? -65 : clamp(near - 2, -90, -35);
  const lock = far === null ? -80 : clamp(far + 2, -110, unlock - 8);
  const sampling = step === "near" || step === "far";
  return (
    <div className="dialog-backdrop">
      <section className="calibration-dialog" role="dialog" aria-modal="true" aria-labelledby="calibration-title">
        <div className="calibration-header">
          <span className="dialog-icon blue"><Icon name="calibrate" size={25} /></span>
          <div><h2 id="calibration-title">距离校准</h2><p>RSSI 只用于改善便利性，不能防止蓝牙中继攻击。</p></div>
          <button className="icon-close" type="button" disabled={sampling} onClick={onClose}><Icon name="close" size={20} /></button>
        </div>
        <div className="calibration-body">
          {step === "intro" ? <><h3>设置解锁距离</h3><p>把手机放在希望能够解锁电脑的最远位置，然后开始采集近距离信号。</p></> : null}
          {step === "move" ? <><h3>设置锁定距离</h3><p>近距离样本为 <strong>{near} dBm</strong>。现在把手机移到应触发自动锁定的位置，等待信号稳定。</p></> : null}
          {sampling ? <><h3>{step === "near" ? "正在采集近距离信号" : "正在采集远距离信号"}</h3><p>请保持手机位置不动，采集大约需要 8 秒。</p><div className="progress-track"><span style={{ width: `${progress}%` }} /></div><strong className="progress-label">{progress}%</strong></> : null}
          {step === "result" ? <><h3>建议阈值已生成</h3><p>近、远样本之间保留了安全滞回，避免在边界反复锁定。</p><div className="calibration-result"><span><small>解锁</small><strong>{unlock} dBm</strong></span><span><small>锁定</small><strong>{lock} dBm</strong></span></div></> : null}
          {error ? <p className="inline-error">{error}</p> : null}
        </div>
        <div className="dialog-actions">
          <button className="secondary-button" type="button" disabled={sampling} onClick={onClose}>取消</button>
          {step === "intro" ? <button className="primary-button" type="button" onClick={() => void collect("near")}>采集近距离</button> : null}
          {step === "move" ? <button className="primary-button" type="button" onClick={() => void collect("far")}>采集远距离</button> : null}
          {step === "result" ? <button className="primary-button" type="button" onClick={() => void applyResult()}>应用阈值</button> : null}
        </div>
      </section>
    </div>
  );
}
