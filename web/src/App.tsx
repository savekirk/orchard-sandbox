import { useEffect, useState, type ReactNode } from "react";
import {
  ArrowLeftRight,
  BookOpen,
  FlaskConical,
  IdCard,
  LayoutDashboard,
  Menu,
  MessageSquare,
  Moon,
  Repeat,
  ScrollText,
  Settings as SettingsIcon,
  Sun,
  Webhook,
  X,
  Zap,
  type LucideIcon,
} from "lucide-react";
import { Select } from "@/components/ui/input";
import { useApi } from "@/hooks/useApi";
import { runPath } from "@/lib/api";
import { LiveProvider, useLiveConnected, useTopicsVersion } from "@/lib/live";
import { RouterProvider, useRoute, type Page } from "@/lib/router";
import { SandboxProvider, useSandbox } from "@/lib/sandbox";
import { ToastProvider } from "@/lib/toast";
import { cn, storageSet } from "@/lib/utils";
import type { Overview } from "@/types";
import { OverviewPage } from "@/pages/Overview";
import { TransactionsPage } from "@/pages/Transactions";
import { CallbacksPage } from "@/pages/Callbacks";
import { RequestsPage } from "@/pages/Requests";
import { MessagesPage } from "@/pages/Messages";
import { PlaygroundPage } from "@/pages/Playground";
import { AutoDebitPage } from "@/pages/AutoDebit";
import { TestDataPage } from "@/pages/TestData";
import { FailuresPage } from "@/pages/Failures";
import { SettingsPage } from "@/pages/Settings";

const nav: { group: string; items: { page: Page; label: string; icon: LucideIcon; count?: keyof Overview["counts"] }[] }[] = [
  {
    group: "Monitor",
    items: [
      { page: "overview", label: "Overview", icon: LayoutDashboard },
      { page: "transactions", label: "Transactions", icon: ArrowLeftRight, count: "pending" },
      { page: "callbacks", label: "Callbacks", icon: Webhook, count: "callbacks_failed" },
      { page: "requests", label: "API log", icon: ScrollText },
      { page: "messages", label: "Messages", icon: MessageSquare },
    ],
  },
  {
    group: "Simulate",
    items: [
      { page: "playground", label: "Playground", icon: FlaskConical },
      { page: "auto-debit", label: "Auto debit", icon: Repeat },
      { page: "test-data", label: "Test data", icon: IdCard },
      { page: "failures", label: "Failure rules", icon: Zap },
    ],
  },
  { group: "Configure", items: [{ page: "settings", label: "Settings", icon: SettingsIcon }] },
];

const pages: Record<Page, () => ReactNode> = {
  overview: OverviewPage,
  transactions: TransactionsPage,
  callbacks: CallbacksPage,
  requests: RequestsPage,
  messages: MessagesPage,
  playground: PlaygroundPage,
  "auto-debit": AutoDebitPage,
  "test-data": TestDataPage,
  failures: FailuresPage,
  settings: SettingsPage,
};

function Logo() {
  return (
    <div className="flex items-center gap-2.5">
      <div className="grid h-8 w-8 place-items-center rounded-lg bg-gradient-to-br from-emerald-500 to-emerald-700 shadow-sm ring-1 ring-inset ring-white/20">
        <svg viewBox="0 0 24 24" className="h-[18px] w-[18px]" fill="none" stroke="white" strokeWidth="2.2" strokeLinecap="round">
          <path d="M6 17c0-6.5 4.5-11 11.5-11 0 7.5-4.5 12-11 12" />
          <path d="M5 19l6.5-6.5" />
        </svg>
      </div>
      <div className="leading-tight">
        <div className="text-[15px] font-semibold tracking-tight">Orchard Sandbox</div>
        <div className="text-[11px] text-muted-foreground">Mock payments API</div>
      </div>
    </div>
  );
}

function ThemeToggle() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains("dark"));
  const toggle = () => {
    document.documentElement.classList.toggle("dark", !dark);
    storageSet("theme", dark ? "light" : "dark");
    setDark(!dark);
  };
  return (
    <button onClick={toggle} title="Toggle theme" className="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground">
      {dark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
    </button>
  );
}

function Sidebar({ onNavigate }: { onNavigate?: () => void }) {
  const { page, navigate } = useRoute();
  const { runId, runs, selectRun, reloadRuns } = useSandbox();
  const connected = useLiveConnected();
  const { data: overview } = useApi<Overview>(runPath(runId, "/overview"), ["transactions", "callbacks", "requests"]);

  const runsVersion = useTopicsVersion(["runs", "settings"]);
  useEffect(() => {
    if (runsVersion > 0) reloadRuns().catch(() => {});
  }, [runsVersion, reloadRuns]);

  return (
    <div className="flex h-full flex-col">
      <div className="px-4 pb-4 pt-5">
        <Logo />
      </div>
      <div className="px-3 pb-3">
        <label className="mb-1 block px-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">Sandbox</label>
        <Select value={runId} onChange={(e) => selectRun(e.target.value)} className="h-8 text-[13px]">
          {runs.length === 0 && <option value={runId}>{runId}</option>}
          {runs.map((r) => (
            <option key={r.run_id} value={r.run_id}>
              {r.name === r.run_id ? r.run_id : `${r.name} (${r.run_id})`}
            </option>
          ))}
        </Select>
      </div>
      <nav className="flex-1 space-y-5 overflow-y-auto px-3 py-2">
        {nav.map((section) => (
          <div key={section.group}>
            <div className="mb-1 px-2 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">{section.group}</div>
            {section.items.map(({ page: p, label, icon: Icon, count }) => {
              const n = count ? overview?.counts[count] : 0;
              return (
                <button
                  key={p}
                  onClick={() => {
                    navigate(p);
                    onNavigate?.();
                  }}
                  className={cn(
                    "flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-[13.5px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground",
                    page === p && "bg-accent font-medium text-foreground",
                  )}
                >
                  <Icon className={cn("h-4 w-4", page === p && "text-primary")} />
                  <span className="flex-1 text-left">{label}</span>
                  {!!n && (
                    <span className={cn("tabular rounded-full px-1.5 text-[11px] font-medium", count === "callbacks_failed" ? "bg-red-500/15 text-red-600 dark:text-red-400" : "bg-amber-500/15 text-amber-700 dark:text-amber-400")}>
                      {n}
                    </span>
                  )}
                </button>
              );
            })}
          </div>
        ))}
      </nav>
      <div className="flex items-center justify-between border-t px-4 py-3">
        <div className="flex items-center gap-2 text-xs text-muted-foreground" title={connected ? "Receiving live updates" : "Reconnecting to live updates"}>
          <span className={cn("relative flex h-2 w-2")}>
            {connected && <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-60" />}
            <span className={cn("relative inline-flex h-2 w-2 rounded-full", connected ? "bg-emerald-500" : "bg-amber-500")} />
          </span>
          {connected ? "Live" : "Reconnecting"}
        </div>
        <div className="flex items-center gap-0.5">
          <a
            href="https://docs.anmgw.com/docs-page.html"
            target="_blank"
            rel="noreferrer"
            title="Orchard API documentation"
            className="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          >
            <BookOpen className="h-4 w-4" />
          </a>
          <ThemeToggle />
        </div>
      </div>
    </div>
  );
}

function Shell() {
  const { page } = useRoute();
  const { runId } = useSandbox();
  const [menuOpen, setMenuOpen] = useState(false);
  const Current = pages[page] ?? OverviewPage;

  return (
    <LiveProvider runId={runId}>
      <div className="min-h-screen lg:pl-60">
        <aside className="fixed inset-y-0 left-0 z-30 hidden w-60 border-r bg-sidebar lg:block">
          <Sidebar />
        </aside>

        <header className="sticky top-0 z-20 flex h-14 items-center justify-between border-b bg-background/80 px-4 backdrop-blur lg:hidden">
          <Logo />
          <button onClick={() => setMenuOpen(true)} className="rounded-md p-2 hover:bg-accent" aria-label="Open menu">
            <Menu className="h-5 w-5" />
          </button>
        </header>
        {menuOpen && (
          <div className="fixed inset-0 z-50 lg:hidden">
            <div className="absolute inset-0 bg-black/40" onClick={() => setMenuOpen(false)} />
            <aside className="absolute inset-y-0 left-0 w-64 border-r bg-sidebar shadow-xl animate-in slide-in-from-left">
              <button onClick={() => setMenuOpen(false)} className="absolute right-3 top-5 rounded-md p-1 hover:bg-accent" aria-label="Close menu">
                <X className="h-4 w-4" />
              </button>
              <Sidebar onNavigate={() => setMenuOpen(false)} />
            </aside>
          </div>
        )}

        <main key={runId} className="mx-auto max-w-6xl px-4 py-6 sm:px-8 sm:py-8">
          <Current />
        </main>
      </div>
    </LiveProvider>
  );
}

export function App() {
  return (
    <ToastProvider>
      <RouterProvider>
        <SandboxProvider>
          <Shell />
        </SandboxProvider>
      </RouterProvider>
    </ToastProvider>
  );
}
