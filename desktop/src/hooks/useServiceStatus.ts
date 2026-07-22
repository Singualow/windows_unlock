import { useCallback, useEffect, useRef, useState } from "react";
import { getStatus, isDesktopRuntime } from "../lib/backend";
import { describeError } from "../lib/format";
import type { ServiceStatus, SignalPoint } from "../types";

const previewValues = [-58, -56, -57, -54, -55, -52, -53, -50, -52, -51, -48, -55, -52, -51, -46, -53, -49, -48];

function previewHistory(): SignalPoint[] {
  if (isDesktopRuntime) return [];
  const current = Date.now();
  return previewValues.map((value, index) => ({
    at: current - (previewValues.length - index - 1) * 32_000,
    value,
  }));
}

export function useServiceStatus() {
  const [status, setStatus] = useState<ServiceStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [signalPoints, setSignalPoints] = useState<SignalPoint[]>(previewHistory);
  const alive = useRef(true);

  const refresh = useCallback(async () => {
    try {
      const next = await getStatus();
      if (!alive.current) return;
      setStatus(next);
      setError(null);
      if (next.has_rssi && typeof next.median_rssi === "number") {
        setSignalPoints((current) => {
          const last = current.at(-1);
          const point = { at: Date.now(), value: next.median_rssi as number };
          if (last && point.at - last.at < 1_500) return current;
          return [...current, point].slice(-60);
        });
      }
    } catch (nextError) {
      if (alive.current) setError(describeError(nextError));
    } finally {
      if (alive.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    alive.current = true;
    let timer = 0;
    const poll = async () => {
      await refresh();
      if (alive.current) timer = window.setTimeout(poll, 2_000);
    };
    void poll();
    return () => {
      alive.current = false;
      window.clearTimeout(timer);
    };
  }, [refresh]);

  return { status, error, loading, signalPoints, refresh };
}
