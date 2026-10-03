export class ApiError extends Error {}

/** Calls the sandbox admin API and returns parsed JSON. Non-2xx responses throw ApiError. */
export async function api<T>(path: string, init?: { method?: string; body?: unknown }): Promise<T> {
  const res = await fetch(path, {
    method: init?.method ?? "GET",
    headers: init?.body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: init?.body !== undefined ? JSON.stringify(init.body) : undefined,
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) throw new ApiError(data?.error ?? `${res.status} ${res.statusText}`);
  return data as T;
}

/** Path helper for run-scoped admin endpoints. */
export function runPath(runId: string, suffix = "") {
  return `/mock/runs/${encodeURIComponent(runId)}${suffix}`;
}
