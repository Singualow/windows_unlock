import { getCurrentWindow } from "@tauri-apps/api/window";
import { isDesktopRuntime } from "../lib/backend";
import type { AppSection } from "../types";
import { Icon } from "./Icon";

const navItems: Array<{ id: AppSection; label: string }> = [
  { id: "overview", label: "概览" },
  { id: "devices", label: "设备" },
  { id: "logs", label: "日志" },
  { id: "settings", label: "设置" },
];

interface AppHeaderProps {
  active: AppSection;
  onNavigate: (section: AppSection) => void;
}

async function windowAction(action: "minimize" | "maximize" | "close") {
  if (!isDesktopRuntime) return;
  const window = getCurrentWindow();
  if (action === "minimize") await window.minimize();
  if (action === "maximize") await window.toggleMaximize();
  if (action === "close") await window.close();
}

export function AppHeader({ active, onNavigate }: AppHeaderProps) {
  return (
    <header className="app-header" data-tauri-drag-region>
      <div className="brand" data-tauri-drag-region>
        <img className="brand-icon" src="/brand.svg" alt="" />
        <span className="brand-title">蓝牙解锁</span>
      </div>
      <nav className="primary-nav" aria-label="主导航">
        {navItems.map((item) => (
          <button
            className={active === item.id ? "nav-item is-active" : "nav-item"}
            key={item.id}
            onClick={() => onNavigate(item.id)}
            type="button"
          >
            {item.label}
          </button>
        ))}
      </nav>
      <div className="window-actions">
        <button className="window-button" onClick={() => void windowAction("minimize")} type="button" aria-label="最小化">
          <Icon name="minimize" size={18} />
        </button>
        <button className="window-button" onClick={() => void windowAction("maximize")} type="button" aria-label="最大化">
          <Icon name="maximize" size={16} />
        </button>
        <button className="window-button close-button" onClick={() => void windowAction("close")} type="button" aria-label="关闭到托盘">
          <Icon name="close" size={18} />
        </button>
      </div>
    </header>
  );
}
