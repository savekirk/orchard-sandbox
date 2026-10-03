import { useEffect, useMemo, useState } from "react";
import { ArrowLeftRight, Check, ScrollText, Search, Webhook, X } from "lucide-react";
import { CallbackState, CopyButton, Details, EmptyState, Mono, PageHeader, Segmented, TxStatus, TypeBadge } from "@/components/common";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog, DialogDescription, DialogTitle, SheetContent } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useApi } from "@/hooks/useApi";
import { api, runPath } from "@/lib/api";
import { useRoute } from "@/lib/router";
import { useSandbox } from "@/lib/sandbox";
import { useToast } from "@/lib/toast";
import { localTime, money, timeAgo } from "@/lib/utils";
import type { Callback, Transaction } from "@/types";

type Filter = "all" | "PENDING" | "SUCCESSFUL" | "FAILED";

const channelLabel: Record<string, string> = { api: "API", checkout: "Hosted checkout", ghipss: "GHIPSS card", auto_debit: "Auto debit" };

function Countdown({ at }: { at: number }) {
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 500);
    return () => window.clearInterval(id);
  }, []);
  const seconds = Math.max(0, Math.ceil((at - now) / 1000));
  return <>{seconds > 0 ? `Settles automatically in ${seconds}s` : "Settling…"}</>;
}

function TransactionSheet({ txn, onClose }: { txn: Transaction | undefined; onClose: () => void }) {
  const { runId } = useSandbox();
  const { navigate } = useRoute();
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const { data: callbacks } = useApi<Callback[]>(txn ? runPath(runId, "/callbacks") : null, ["callbacks"]);
  const related = callbacks?.filter((c) => c.exttrid === txn?.exttrid) ?? [];

  const resolve = async (status: "SUCCESSFUL" | "FAILED") => {
    if (!txn) return;
    setBusy(true);
    try {
      await api(runPath(runId, `/transactions/${encodeURIComponent(txn.exttrid)}/resolve`), {
        method: "POST",
        body: { status, message: status === "FAILED" ? "FAILED: Declined from the sandbox dashboard" : "SUCCESS" },
      });
      toast(status === "SUCCESSFUL" ? "Payment approved" : "Payment failed");
    } catch (e) {
      toast((e as Error).message, true);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={!!txn} onOpenChange={(open) => !open && onClose()}>
      <SheetContent>
        {txn && (
          <>
            <div className="border-b px-6 pb-5 pt-6">
              <div className="flex items-center gap-2">
                <TypeBadge type={txn.trans_type} />
                <TxStatus status={txn.status} />
              </div>
              <DialogTitle className="tabular mt-3 text-2xl font-semibold tracking-tight">
                <span className="mr-1.5 text-base font-medium text-muted-foreground">GHS</span>
                {money(txn.amount)}
              </DialogTitle>
              <DialogDescription className="mt-1 flex items-center gap-1">
                <Mono>{txn.exttrid}</Mono>
                <CopyButton value={txn.exttrid} />
              </DialogDescription>
            </div>

            <div className="flex-1 space-y-6 overflow-y-auto px-6 py-5">
              {txn.status === "PENDING" && (
                <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-4">
                  <p className="text-sm font-medium">Waiting for the customer</p>
                  <p className="mt-0.5 text-[13px] text-muted-foreground">
                    {txn.settle_at ? <Countdown at={txn.settle_at} /> : "This payment settles only when you resolve it."} Resolving sends the callback.
                  </p>
                  <div className="mt-3 flex gap-2">
                    <Button size="sm" disabled={busy} onClick={() => resolve("SUCCESSFUL")}>
                      <Check /> Approve
                    </Button>
                    <Button size="sm" variant="outline" disabled={busy} onClick={() => resolve("FAILED")}>
                      <X /> Fail
                    </Button>
                  </div>
                </div>
              )}

              <Details
                items={[
                  ["Orchard status", <Mono>{`${txn.trans_status} · ${txn.message}`}</Mono>],
                  ["Transaction ID", <span className="flex items-center gap-1"><Mono>{txn.trans_id}</Mono><CopyButton value={txn.trans_id} /></span>],
                  ["Channel", channelLabel[txn.channel] ?? txn.channel],
                  ["Customer", txn.customer_number && <Mono>{txn.customer_number}</Mono>],
                  ["Network", txn.nw],
                  ["Reference", txn.reference],
                  ["Callback URL", txn.callback_url && <Mono className="break-all">{txn.callback_url}</Mono>],
                  ["Scenario", txn.scenario],
                  ...Object.entries(txn.meta ?? {}).map(([k, v]) => [k.replace(/_/g, " "), v] as [string, string]),
                  ["Created", localTime(txn.created_at)],
                  ["Updated", localTime(txn.updated_at)],
                ]}
              />

              <div>
                <div className="mb-2 flex items-center justify-between">
                  <h4 className="text-sm font-medium">Callbacks</h4>
                  <Button variant="ghost" size="xs" onClick={() => navigate("requests", { ref: txn.exttrid })}>
                    <ScrollText /> API calls
                  </Button>
                </div>
                {related.length === 0 ? (
                  <p className="text-[13px] text-muted-foreground">{txn.callback_url ? "None sent yet." : "No callback URL on this transaction."}</p>
                ) : (
                  <div className="divide-y rounded-lg border">
                    {related.map((c) => (
                      <button key={c.id} onClick={() => navigate("callbacks", { id: c.id })} className="flex w-full items-center gap-3 px-3 py-2.5 text-left text-sm hover:bg-muted/50">
                        <Webhook className="h-4 w-4 text-muted-foreground" />
                        <span className="flex-1 truncate text-[13px] text-muted-foreground">
                          {c.attempts}/{c.max_attempts} attempts · {timeAgo(c.created_at)}
                        </span>
                        <CallbackState state={c.state} />
                      </button>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </>
        )}
      </SheetContent>
    </Dialog>
  );
}

export function TransactionsPage() {
  const { runId } = useSandbox();
  const { params, navigate } = useRoute();
  const { data: txns } = useApi<Transaction[]>(runPath(runId, "/transactions"), ["transactions"]);
  const [filter, setFilter] = useState<Filter>("all");
  const [query, setQuery] = useState("");
  const selectedRef = params.get("ref");

  const counts = useMemo(() => {
    const c = { all: 0, PENDING: 0, SUCCESSFUL: 0, FAILED: 0 };
    for (const t of txns ?? []) {
      c.all++;
      c[t.status]++;
    }
    return c;
  }, [txns]);

  const q = query.trim().toLowerCase();
  const rows = (txns ?? []).filter(
    (t) => (filter === "all" || t.status === filter) && (!q || [t.exttrid, t.trans_id, t.customer_number, t.reference].some((v) => v?.toLowerCase().includes(q))),
  );
  const selected = txns?.find((t) => t.exttrid === selectedRef);

  return (
    <>
      <PageHeader title="Transactions" description="Payments accepted through /sendRequest, hosted checkout and auto debit. Pending payments can be approved or failed by hand." />
      <Card>
        <div className="flex flex-col gap-3 border-b p-3 sm:flex-row sm:items-center sm:justify-between">
          <Segmented<Filter>
            value={filter}
            onChange={setFilter}
            options={[
              { value: "all", label: "All", count: counts.all },
              { value: "PENDING", label: "Pending", count: counts.PENDING },
              { value: "SUCCESSFUL", label: "Successful", count: counts.SUCCESSFUL },
              { value: "FAILED", label: "Failed", count: counts.FAILED },
            ]}
          />
          <div className="relative sm:w-72">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input placeholder="Search reference, customer, ID" className="pl-8" value={query} onChange={(e) => setQuery(e.target.value)} />
          </div>
        </div>
        {txns && rows.length === 0 ? (
          <EmptyState icon={ArrowLeftRight} title={txns.length ? "No matching transactions" : "No transactions yet"} description={txns.length ? undefined : "Collections, payouts, airtime, bills and checkouts appear here as they arrive."} />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Reference</TableHead>
                <TableHead>Type</TableHead>
                <TableHead className="hidden md:table-cell">Customer</TableHead>
                <TableHead className="text-right">Amount</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="hidden text-right sm:table-cell">Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((t) => (
                <TableRow key={t.exttrid} onClick={() => navigate("transactions", { ref: t.exttrid })}>
                  <TableCell>
                    <Mono>{t.exttrid}</Mono>
                  </TableCell>
                  <TableCell>
                    <TypeBadge type={t.trans_type} />
                  </TableCell>
                  <TableCell className="hidden md:table-cell">
                    <Mono className="text-muted-foreground">{t.customer_number || "—"}</Mono>
                    {t.nw && <span className="ml-1.5 text-xs text-muted-foreground">{t.nw}</span>}
                  </TableCell>
                  <TableCell className="tabular text-right font-medium">{money(t.amount)}</TableCell>
                  <TableCell>
                    <TxStatus status={t.status} />
                  </TableCell>
                  <TableCell className="hidden text-right text-muted-foreground sm:table-cell">{timeAgo(t.created_at)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Card>
      <TransactionSheet txn={selected} onClose={() => navigate("transactions")} />
    </>
  );
}
