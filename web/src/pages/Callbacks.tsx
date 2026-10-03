import { RotateCw, Webhook } from "lucide-react";
import { CallbackState, CodeBlock, Details, EmptyState, Mono, PageHeader } from "@/components/common";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog, DialogDescription, DialogTitle, SheetContent } from "@/components/ui/dialog";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useApi } from "@/hooks/useApi";
import { api, runPath } from "@/lib/api";
import { useRoute } from "@/lib/router";
import { useSandbox } from "@/lib/sandbox";
import { useToast } from "@/lib/toast";
import { localTime, timeAgo } from "@/lib/utils";
import type { Callback, CallbackAttempt } from "@/types";

function HttpStatus({ status, error }: { status: number; error?: string }) {
  if (!status) return <Badge tone={error ? "red" : "neutral"}>{error ? "error" : "—"}</Badge>;
  return (
    <Badge tone={status < 300 ? "green" : "red"} className="font-mono">
      {status}
    </Badge>
  );
}

function CallbackSheet({ cb, onClose }: { cb: Callback | undefined; onClose: () => void }) {
  const { runId } = useSandbox();
  const toast = useToast();
  const { data: attempts } = useApi<CallbackAttempt[]>(cb ? runPath(runId, `/callbacks/${cb.id}/attempts`) : null, ["callbacks"]);

  const retry = async () => {
    if (!cb) return;
    try {
      await api(runPath(runId, `/callbacks/${cb.id}/retry`), { method: "POST" });
      toast("Callback queued for another attempt");
    } catch (e) {
      toast((e as Error).message, true);
    }
  };

  return (
    <Dialog open={!!cb} onOpenChange={(open) => !open && onClose()}>
      <SheetContent>
        {cb && (
          <>
            <div className="border-b px-6 pb-5 pt-6">
              <CallbackState state={cb.state} />
              <DialogTitle className="mt-3">Callback for {cb.exttrid}</DialogTitle>
              <DialogDescription className="mt-1 break-all font-mono text-[12.5px]">POST {cb.url}</DialogDescription>
            </div>
            <div className="flex-1 space-y-6 overflow-y-auto px-6 py-5">
              <Details
                items={[
                  ["Attempts", `${cb.attempts} of ${cb.max_attempts}`],
                  ["Next attempt", cb.state === "QUEUED" && cb.next_attempt_at ? new Date(cb.next_attempt_at).toLocaleTimeString() : ""],
                  ["Delivered", localTime(cb.delivered_at ?? "")],
                  ["Created", localTime(cb.created_at)],
                ]}
              />
              <div>
                <h4 className="mb-2 text-sm font-medium">Payload</h4>
                <CodeBlock code={cb.payload} json />
              </div>
              <div>
                <h4 className="mb-2 text-sm font-medium">Delivery attempts</h4>
                {attempts?.length === 0 && <p className="text-[13px] text-muted-foreground">Not attempted yet.</p>}
                <ol className="space-y-2">
                  {attempts?.map((a, i) => (
                    <li key={a.id} className="rounded-lg border p-3">
                      <div className="flex items-center gap-2 text-sm">
                        <span className="text-muted-foreground">#{i + 1}</span>
                        <HttpStatus status={a.http_status} error={a.error} />
                        <span className="text-xs text-muted-foreground">{a.duration_ms} ms</span>
                        <span className="ml-auto text-xs text-muted-foreground">{localTime(a.attempted_at)}</span>
                      </div>
                      {(a.error || a.response_body) && <Mono className="mt-2 block break-all text-muted-foreground">{a.error || a.response_body}</Mono>}
                    </li>
                  ))}
                </ol>
              </div>
            </div>
            {cb.state !== "DELIVERING" && (
              <div className="border-t px-6 py-3">
                <Button size="sm" variant="outline" onClick={retry}>
                  <RotateCw /> Send again
                </Button>
              </div>
            )}
          </>
        )}
      </SheetContent>
    </Dialog>
  );
}

export function CallbacksPage() {
  const { runId } = useSandbox();
  const { params, navigate } = useRoute();
  const { data: callbacks } = useApi<Callback[]>(runPath(runId, "/callbacks"), ["callbacks"]);
  const selected = callbacks?.find((c) => c.id === params.get("id"));

  return (
    <>
      <PageHeader
        title="Callbacks"
        description="Webhooks the sandbox POSTs to your callback_url when a payment settles. Failed deliveries retry with backoff; any 2xx counts as delivered."
      />
      <Card>
        {callbacks && callbacks.length === 0 ? (
          <EmptyState icon={Webhook} title="No callbacks yet" description="They are queued as soon as a payment with a callback_url settles." />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Reference</TableHead>
                <TableHead className="hidden md:table-cell">Endpoint</TableHead>
                <TableHead>State</TableHead>
                <TableHead>Attempts</TableHead>
                <TableHead>Last response</TableHead>
                <TableHead className="text-right">Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {callbacks?.map((c) => (
                <TableRow key={c.id} onClick={() => navigate("callbacks", { id: c.id })}>
                  <TableCell>
                    <Mono>{c.exttrid}</Mono>
                  </TableCell>
                  <TableCell className="hidden max-w-[260px] truncate md:table-cell">
                    <Mono className="text-muted-foreground">{c.url}</Mono>
                  </TableCell>
                  <TableCell>
                    <CallbackState state={c.state} />
                  </TableCell>
                  <TableCell className="tabular text-muted-foreground">
                    {c.attempts}/{c.max_attempts}
                  </TableCell>
                  <TableCell>
                    <HttpStatus status={c.last_status} error={c.last_error} />
                  </TableCell>
                  <TableCell className="text-right text-muted-foreground">{timeAgo(c.created_at)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Card>
      <CallbackSheet cb={selected} onClose={() => navigate("callbacks")} />
    </>
  );
}
