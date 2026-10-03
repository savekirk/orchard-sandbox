import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

const ghs = new Intl.NumberFormat("en-GH", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

/** Formats a GHS amount, e.g. 75010.5 → "75,010.50". */
export function money(value: number | string) {
  return ghs.format(typeof value === "string" ? Number(value) : value);
}

/** Server timestamps are UTC "YYYY-MM-DD HH:MM:SS". */
export function parseTime(ts: string) {
  return new Date(ts.replace(" ", "T") + "Z");
}

export function timeAgo(ts: string) {
  if (!ts) return "";
  const seconds = Math.round((Date.now() - parseTime(ts).getTime()) / 1000);
  if (seconds < 5) return "just now";
  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  return parseTime(ts).toLocaleDateString();
}

export function localTime(ts: string) {
  return ts ? parseTime(ts).toLocaleString() : "";
}

/** Pretty-prints JSON text, returning the input unchanged if it is not JSON. */
export function prettyJSON(raw: string) {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

/** Orchard timestamp for "now": UTC "YYYY-MM-DD HH:MM:SS". */
export function orchardTimestamp() {
  return new Date().toISOString().slice(0, 19).replace("T", " ");
}

export function randomRef(prefix: string) {
  return `${prefix}-${Date.now().toString(36).toUpperCase()}`;
}

export function storageGet(key: string) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

export function storageSet(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // Storage can be unavailable (private mode); the dashboard still works without it.
  }
}
