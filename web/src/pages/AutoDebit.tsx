import { Repeat, Zap } from "lucide-react";
import { CopyButton, EmptyState, Mono, PageHeader } from "@/components/common";
import { Badge, type Tone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useApi } from "@/hooks/useApi";
import { api, runPath } from "@/lib/api";
import { useRoute } from "@/lib/router";
import { useSandbox } from "@/lib/sandbox";
import { useToast } from "@/lib/toast";
import { money, timeAgo } from "@/lib/utils";
import type { Subscription, Transaction } from "@/types";

const tone: Record<string, Tone> = { Pending: "amber", Active: "green", Suspended: "neutral", Cancelled: "red" };
const cycles: Record<string, string> = { DLY: "Daily", WKL: "Weekly", MON: "Monthly" };

export function AutoDebitPage() {
  const { runId } = useSandbox();
  const { navigate } = useRoute();
  const toast = useToast();
  const { data: subs } = useApi<Subscription[]>(runPath(runId, "/subscriptions"), ["subscriptions"]);

  const debit = async (ref: string) => {
    try {
      const t = await api<Transaction>(runPath(runId, `/subscriptions/${encodeURIComponent(ref)}/debit`), { method: "POST" });
      toast(`Debit ${t.exttrid} created`);
    } catch (e) {
      toast((e as Error).message, true);
    }
  };

  return (
    <>
      <PageHeader
        title="Auto debit"
        description={
          <>
            Mandates created with <Mono>/autoDebit</Mono> SUB. Activate them with the OTP shown here, then run a billing cycle on demand instead of waiting a day.
          </>
        }
      />
      <Card>
        {subs && subs.length === 0 ? (
          <EmptyState icon={Repeat} title="No mandates yet" description="Send operation SUB to /autoDebit. The activation code appears here and in Messages." />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Mandate</TableHead>
                <TableHead>Customer</TableHead>
                <TableHead className="text-right">Amount</TableHead>
                <TableHead>Cycle</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>OTP</TableHead>
                <TableHead className="text-right" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {subs?.map((s) => (
                <TableRow key={s.uniq_ref_id}>
                  <TableCell>
                    <Mono>{s.uniq_ref_id}</Mono>
                    <div className="text-xs text-muted-foreground">
                      {s.reference} · {timeAgo(s.subscribed_at)}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Mono className="text-muted-foreground">{s.customer_number}</Mono>
                    <span className="ml-1.5 text-xs text-muted-foreground">{s.nw}</span>
                  </TableCell>
                  <TableCell className="tabular text-right font-medium">{money(s.amount)}</TableCell>
                  <TableCell className="text-muted-foreground">{cycles[s.cycle] ?? s.cycle}</TableCell>
                  <TableCell>
                    <Badge tone={tone[s.status]} dot>
                      {s.status}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    {s.status === "Pending" ? (
                      <span className="inline-flex items-center gap-1">
                        <Mono className="rounded bg-amber-500/10 px-1.5 py-0.5 font-semibold tracking-widest text-amber-700 dark:text-amber-400">{s.otp}</Mono>
                        <CopyButton value={s.otp} />
                      </span>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-1">
                      <Button size="xs" variant="ghost" onClick={() => navigate("transactions")}>
                        Debits
                      </Button>
                      <Button size="xs" variant="outline" disabled={s.status !== "Active"} onClick={() => debit(s.uniq_ref_id)} title="Charge one cycle now">
                        <Zap /> Debit now
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Card>
    </>
  );
}
