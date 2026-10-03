import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { api } from "@/lib/api";
import { storageGet, storageSet } from "@/lib/utils";
import type { Info, Run } from "@/types";

interface SandboxState {
  runId: string;
  run: Run | undefined;
  runs: Run[];
  info: Info | undefined;
  selectRun: (id: string) => void;
  reloadRuns: () => Promise<void>;
}

const SandboxContext = createContext<SandboxState | null>(null);

/** Holds the selected sandbox run, the run list, and static reference data. */
export function SandboxProvider({ children }: { children: ReactNode }) {
  const [runId, setRunId] = useState(() => storageGet("run") ?? "default");
  const [runs, setRuns] = useState<Run[]>([]);
  const [info, setInfo] = useState<Info>();

  const reloadRuns = useCallback(async () => {
    const list = await api<Run[]>("/mock/runs");
    setRuns(list);
    setRunId((current) => (list.some((r) => r.run_id === current) ? current : "default"));
  }, []);

  useEffect(() => {
    reloadRuns().catch(() => {});
    api<Info>("/mock/info").then(setInfo).catch(() => {});
  }, [reloadRuns]);

  const selectRun = useCallback((id: string) => {
    setRunId(id);
    storageSet("run", id);
  }, []);

  const run = runs.find((r) => r.run_id === runId);
  return <SandboxContext.Provider value={{ runId, run, runs, info, selectRun, reloadRuns }}>{children}</SandboxContext.Provider>;
}

export function useSandbox() {
  const state = useContext(SandboxContext);
  if (!state) throw new Error("useSandbox outside SandboxProvider");
  return state;
}
