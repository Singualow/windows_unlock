import type { SVGProps } from "react";

export type IconName =
  | "arrow"
  | "bluetooth"
  | "calibrate"
  | "check"
  | "close"
  | "credential"
  | "device"
  | "hand"
  | "help"
  | "inbox"
  | "key"
  | "lock"
  | "maximize"
  | "minimize"
  | "pause"
  | "settings"
  | "shield"
  | "signal";

interface IconProps extends SVGProps<SVGSVGElement> {
  name: IconName;
  size?: number;
}

const paths: Record<IconName, React.ReactNode> = {
  arrow: <path d="m9 18 6-6-6-6" />,
  bluetooth: <path d="m12 3 5 5-5 4 5 4-5 5V3Zm0 9L7 8m5 4-5 4" />,
  calibrate: (
    <>
      <circle cx="12" cy="12" r="7" />
      <path d="M12 2v3m0 14v3M2 12h3m14 0h3" />
      <circle cx="12" cy="12" r="2" />
    </>
  ),
  check: <path d="m5 12 4 4L19 6" />,
  close: <path d="m6 6 12 12M18 6 6 18" />,
  credential: (
    <>
      <rect x="4" y="3" width="13" height="18" rx="2" />
      <path d="M8 8h5m-5 4h4m7 2v6m-3-3 3 3 3-3" />
    </>
  ),
  device: (
    <>
      <rect x="5" y="3" width="14" height="18" rx="3" />
      <path d="M9 6h6m-4 12h2" />
    </>
  ),
  hand: <path d="M7 11V7a1.5 1.5 0 0 1 3 0v3-5a1.5 1.5 0 0 1 3 0v5-4a1.5 1.5 0 0 1 3 0v5-2a1.5 1.5 0 0 1 3 0v5c0 4-2.5 7-7 7-3 0-5-1.5-7-4l-2-3a1.6 1.6 0 0 1 2.5-2L7 13" />,
  help: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M9.7 9a2.5 2.5 0 1 1 3.6 2.2c-.8.4-1.3 1-1.3 1.8v.5m0 3.5h.01" />
    </>
  ),
  inbox: (
    <>
      <path d="M5 5h14l2 8v6H3v-6l2-8Z" />
      <path d="M3.5 13H8l1.5 2h5L16 13h4.5" />
    </>
  ),
  key: (
    <>
      <circle cx="8" cy="15" r="4" />
      <path d="m11 12 8-8m-3 3 3 3m-6 0 2 2" />
    </>
  ),
  lock: (
    <>
      <rect x="5" y="10" width="14" height="11" rx="2" />
      <path d="M8 10V7a4 4 0 0 1 8 0v3m-4 4v3" />
    </>
  ),
  maximize: <rect x="5" y="5" width="14" height="14" rx="1" />,
  minimize: <path d="M6 12h12" />,
  pause: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M10 9v6m4-6v6" />
    </>
  ),
  settings: (
    <>
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.8 2.8-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6v.2h-4V21a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1L4.2 17l.1-.1a1.7 1.7 0 0 0 .3-1.9A1.7 1.7 0 0 0 3 14H2.8v-4H3a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9L4.2 7 7 4.2l.1.1A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-1.6v-.2h4V3a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1L19.8 7l-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.6 1h.2v4H21a1.7 1.7 0 0 0-1.6 1Z" />
    </>
  ),
  shield: <path d="M12 3 5 6v5c0 4.5 2.8 8.2 7 10 4.2-1.8 7-5.5 7-10V6l-7-3Zm-3 9 2 2 4-5" />,
  signal: <path d="M5 19v-3m4 3v-6m4 6V9m4 10V6m4 13V3" />,
};

export function Icon({ name, size = 24, ...props }: IconProps) {
  return (
    <svg
      aria-hidden="true"
      fill="none"
      height={size}
      viewBox="0 0 24 24"
      width={size}
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.8"
      {...props}
    >
      {paths[name]}
    </svg>
  );
}
