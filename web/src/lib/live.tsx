import { createContext, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";

type Versions = Record<string, number>;

interface LiveState {
  connected: boolean;
  versions: Versions;
}

const LiveContext = createContext<LiveState>({ connected: false, versions: {} });

/**
 * Subscribes to the sandbox event stream for one run and counts changes per topic.
 * Bursts of events are batched so pages refetch at most a few times per second.
 */
export function LiveProvider({ runId, children }: { runId: string; children: ReactNode }) {
  const [state, setState] = useState<LiveState>({ connected: false, versions: {} });
  const pending = useRef(new Set<string>());

  useEffect(() => {
    const source = new EventSource(`/mock/events?run=${encodeURIComponent(runId)}`);
    const flush = window.setInterval(() => {
      if (pending.current.size === 0) return;
      const topics = [...pending.current];
      pending.current.clear();
      setState((s) => {
        const versions = { ...s.versions };
        for (const t of topics) versions[t] = (versions[t] ?? 0) + 1;
        return { ...s, versions };
      });
    }, 250);

    source.onopen = () => setState((s) => ({ ...s, connected: true }));
    source.onerror = () => setState((s) => ({ ...s, connected: false }));
    source.onmessage = (e) => {
      try {
        const { type } = JSON.parse(e.data) as { type: string };
        pending.current.add(type);
      } catch {
        // Ignore malformed events.
      }
    };
    return () => {
      window.clearInterval(flush);
      source.close();
    };
  }, [runId]);

  return <LiveContext.Provider value={state}>{children}</LiveContext.Provider>;
}

export function useLiveConnected() {
  return useContext(LiveContext).connected;
}

/** Returns a number that changes whenever any of the topics (or a reset) changes. */
export function useTopicsVersion(topics: string[]) {
  const { versions } = useContext(LiveContext);
  const key = topics.join(",");
  return useMemo(() => {
    let sum = versions.reset ?? 0;
    for (const t of key.split(",")) sum += versions[t] ?? 0;
    return sum;
  }, [versions, key]);
}
