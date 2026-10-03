import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useTopicsVersion } from "@/lib/live";

interface ApiState<T> {
  data: T | undefined;
  error: string | undefined;
  loading: boolean;
}

/** Fetches an admin endpoint and refetches whenever one of the live topics changes. */
export function useApi<T>(path: string | null, topics: string[] = []) {
  const version = useTopicsVersion(topics);
  const [nonce, setNonce] = useState(0);
  const [state, setState] = useState<ApiState<T>>({ data: undefined, error: undefined, loading: true });

  useEffect(() => {
    setState({ data: undefined, error: undefined, loading: true });
  }, [path]);

  useEffect(() => {
    if (!path) return;
    let cancelled = false;
    api<T>(path)
      .then((data) => !cancelled && setState({ data, error: undefined, loading: false }))
      .catch((e: Error) => !cancelled && setState((s) => ({ ...s, error: e.message, loading: false })));
    return () => {
      cancelled = true;
    };
  }, [path, version, nonce]);

  const reload = useCallback(() => setNonce((n) => n + 1), []);
  return { ...state, reload };
}
