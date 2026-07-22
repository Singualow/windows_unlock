import { useEffect, useState, type FormEvent } from "react";
import { Icon } from "./Icon";

export type ParameterDialogKind = "high-sensitivity" | "doppler";

interface ParameterDialogProps {
  kind: ParameterDialogKind | null;
  value: number;
  busy: boolean;
  onClose: () => void;
  onSave: (value: number) => void;
}

const specifications = {
  "high-sensitivity": {
    title: "高灵敏参数",
    label: "解锁触发阈值（RSSI）",
    min: -90,
    max: -20,
    unit: "dBm",
    scale: ["远", "适中", "近"],
    note: "锁定保护线会自动设置为触发阈值低 8 dB，避免在边界反复锁定。",
  },
  doppler: {
    title: "多普勒预测参数",
    label: "预测灵敏度",
    min: 1,
    max: 100,
    unit: "",
    scale: ["稳健", "平衡", "灵敏"],
    note: "灵敏度越高，越容易根据增强趋势提前发起认证；仍需手机签名才能解锁。",
  },
} as const;

export function ParameterDialog({ kind, value, busy, onClose, onSave }: ParameterDialogProps) {
  const [draft, setDraft] = useState(value);

  useEffect(() => setDraft(value), [kind, value]);
  if (!kind) return null;

  const specification = specifications[kind];
  const valid = Number.isInteger(draft) && draft >= specification.min && draft <= specification.max;

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (valid && !busy) onSave(draft);
  }

  return (
    <div className="dialog-backdrop" role="presentation">
      <form className="parameter-dialog" aria-labelledby="parameter-dialog-title" onSubmit={submit} role="dialog" aria-modal="true">
        <div className="parameter-dialog-heading">
          <div>
            <h2 id="parameter-dialog-title">{specification.title}</h2>
            <p>{kind === "doppler" ? "调整信号增强趋势的预测时机" : "独立调整高灵敏模式的近距离触发线"}</p>
          </div>
          <button className="dialog-close" type="button" aria-label="关闭参数设置" onClick={onClose}>
            <Icon name="close" size={18} />
          </button>
        </div>

        <label className="parameter-value-field">
          <span>{specification.label}</span>
          <span className="parameter-number-wrap">
            <input
              aria-label={specification.label}
              max={specification.max}
              min={specification.min}
              onChange={(event) => setDraft(event.currentTarget.valueAsNumber)}
              step={1}
              type="number"
              value={Number.isNaN(draft) ? "" : draft}
            />
            {specification.unit ? <small>{specification.unit}</small> : null}
          </span>
        </label>

        <input
          aria-label={`${specification.label}滑块`}
          className="parameter-slider"
          max={specification.max}
          min={specification.min}
          onChange={(event) => setDraft(event.currentTarget.valueAsNumber)}
          step={1}
          type="range"
          value={Number.isNaN(draft) ? specification.min : draft}
        />
        <div className="parameter-scale" aria-hidden="true">
          {specification.scale.map((label) => <span key={label}>{label}</span>)}
        </div>

        <div className="parameter-safety-note">
          <Icon name="shield" size={19} />
          <span>{specification.note}</span>
        </div>
        {kind === "doppler" ? (
          <p className="parameter-technical-note">基于最近 RSSI 样本的增强斜率进行预测，不是物理层射频多普勒测量。</p>
        ) : null}

        <div className="dialog-actions">
          <button className="secondary-button" type="button" onClick={onClose}>取消</button>
          <button className="primary-button" type="submit" disabled={!valid || busy}>保存</button>
        </div>
      </form>
    </div>
  );
}
