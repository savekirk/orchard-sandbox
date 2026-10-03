import { useState, type ReactNode } from "react";
import { Check, Copy, type LucideIcon } from "lucide-react";
import { Badge, type Tone } from "@/components/ui/badge";
import { cn, prettyJSON } from "@/lib/utils";

export function PageHeader({ title, description, actions }: { title: string; description?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
        {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}

export function CopyButton({ value, className, label }: { value: string; className?: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await navigator.clipboard.writeText(value);
    } catch {
      const el = document.createElement("textarea");
      el.value = value;
      document.body.appendChild(el);
      el.select();
      document.execCommand("copy");
      el.remove();
    }
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };
  const Icon = copied ? Check : Copy;
  return (
    <button
      type="button"
      onClick={copy}
      title={copied ? "Copied" : `Copy${label ? ` ${label}` : ""}`}
      className={cn("inline-flex shrink-0 items-center gap-1 rounded-md p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground", className)}
    >
      <Icon className={cn("h-3.5 w-3.5", copied && "text-emerald-500")} />
      {label && <span className="text-xs">{copied ? "Copied" : label}</span>}
    </button>
  );
}

export function Mono({ children, className }: { children: ReactNode; className?: string }) {
  return <span className={cn("font-mono text-[12.5px]", className)}>{children}</span>;
}

const txTone: Record<string, Tone> = { PENDING: "amber", SUCCESSFUL: "green", FAILED: "red" };
const txLabel: Record<string, string> = { PENDING: "Pending", SUCCESSFUL: "Successful", FAILED: "Failed" };

export function TxStatus({ status }: { status: string }) {
  return (
    <Badge tone={txTone[status] ?? "neutral"} dot>
      {txLabel[status] ?? status}
    </Badge>
  );
}

const cbTone: Record<string, Tone> = { QUEUED: "amber", DELIVERING: "blue", DELIVERED: "green", FAILED: "red" };

export function CallbackState({ state }: { state: string }) {
  return (
    <Badge tone={cbTone[state] ?? "neutral"} dot>
      {state.charAt(0) + state.slice(1).toLowerCase()}
    </Badge>
  );
}

/** Colours an Orchard response code: success, accepted, or error. */
export function RespCode({ code, status }: { code: string; status?: number }) {
  if (!code) {
    // Some replies (e.g. wallet balances) carry no resp_code; judge them by HTTP status.
    if (status && status < 300) return <Badge tone="green">OK</Badge>;
    return <Badge tone="red">{status ? `HTTP ${status}` : "dropped"}</Badge>;
  }
  const tone: Tone = ["000", "000/01", "027", "082", "083"].includes(code) ? "green" : code === "015" ? "blue" : code.startsWith("10") ? "red" : "amber";
  return (
    <Badge tone={tone} className="font-mono">
      {code}
    </Badge>
  );
}

export function TypeBadge({ type }: { type: string }) {
  const tone: Record<string, Tone> = { CTM: "green", AUD: "green", MTC: "violet", RMT: "violet", ATP: "blue", BLP: "blue" };
  return (
    <Badge tone={tone[type] ?? "neutral"} className="font-mono">
      {type}
    </Badge>
  );
}

export function CodeBlock({ code, className, json }: { code: string; className?: string; json?: boolean }) {
  const text = json ? prettyJSON(code) : code;
  return (
    <div className={cn("group relative rounded-lg border bg-muted/40", className)}>
      <CopyButton value={text} className="absolute right-2 top-2 bg-card/80 opacity-0 transition-opacity group-hover:opacity-100" />
      <pre className="max-h-[420px] overflow-auto whitespace-pre-wrap p-3.5 font-mono text-[12.5px] leading-relaxed [overflow-wrap:anywhere]">{text || <span className="text-muted-foreground">(empty)</span>}</pre>
    </div>
  );
}

export function EmptyState({ icon: Icon, title, description, children }: { icon: LucideIcon; title: string; description?: ReactNode; children?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center px-6 py-14 text-center">
      <div className="mb-3 rounded-xl border bg-muted/50 p-3">
        <Icon className="h-5 w-5 text-muted-foreground" />
      </div>
      <p className="text-sm font-medium">{title}</p>
      {description && <p className="mt-1 max-w-sm text-[13px] text-muted-foreground">{description}</p>}
      {children && <div className="mt-4">{children}</div>}
    </div>
  );
}

export function Details({ items }: { items: [string, ReactNode][] }) {
  return (
    <dl className="grid grid-cols-[minmax(110px,auto)_1fr] gap-x-6 gap-y-2.5 text-sm">
      {items
        .filter(([, v]) => v !== undefined && v !== null && v !== "")
        .map(([k, v]) => (
          <div key={k} className="contents">
            <dt className="text-muted-foreground">{k}</dt>
            <dd className="min-w-0 break-words">{v}</dd>
          </div>
        ))}
    </dl>
  );
}

export function ErrorNote({ message }: { message?: string }) {
  if (!message) return null;
  return <div className="rounded-lg border border-red-500/30 bg-red-500/5 px-4 py-3 text-sm text-red-600 dark:text-red-400">{message}</div>;
}

/** Segmented filter chips. */
export function Segmented<T extends string>({ value, onChange, options }: { value: T; onChange: (v: T) => void; options: { value: T; label: string; count?: number }[] }) {
  return (
    <div className="inline-flex rounded-lg border bg-muted/50 p-0.5">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          onClick={() => onChange(o.value)}
          className={cn(
            "inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-[13px] font-medium text-muted-foreground transition-colors hover:text-foreground",
            value === o.value && "bg-card text-foreground shadow-sm",
          )}
        >
          {o.label}
          {o.count !== undefined && <span className="tabular text-xs text-muted-foreground">{o.count}</span>}
        </button>
      ))}
    </div>
  );
}
