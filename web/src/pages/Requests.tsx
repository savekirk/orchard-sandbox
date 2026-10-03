import { useState } from "react";
import { ScrollText, Search, X } from "lucide-react";
import { CodeBlock, CopyButton, EmptyState, Mono, PageHeader, RespCode } from "@/components/common";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Dialog, DialogDescription, DialogTitle, SheetContent } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useApi } from "@/hooks/useApi";
import { runPath } from "@/lib/api";
import { useRoute } from "@/lib/router";
import { useSandbox } from "@/lib/sandbox";
import { localTime, timeAgo } from "@/lib/utils";
import type { RequestLog } from "@/types";

function curl(base: string, log: RequestLog) {
  const headers = ["content-type", "authorization", "x-sandbox-run"]
    .filter((h) => log.headers?.[h])
    .map((h) => `  -H '${h}: ${log.headers[h]}' \\\n`)
    .join("");
  return `curl -X ${log.method} ${base}${log.path} \\\n${headers}  -d '${log.body.replace(/'/g, "'\\''")}'`;
}

function RequestSheet({ log, onClose }: { log: RequestLog | undefined; onClose: () => void }) {
  const { info } = useSandbox();
  return (
    <Dialog open={!!log} onOpenChange={(open) => !open && onClose()}>
      <SheetContent>
        {log && (
          <>
            <div className="border-b px-6 pb-5 pt-6">
              <div className="flex items-center gap-2">
                <RespCode code={log.resp_code} status={log.status} />
                <Badge tone={log.status < 300 ? "neutral" : "red"} className="font-mono">
                  HTTP {log.status}
                </Badge>
                <span className="text-xs text-muted-foreground">{log.duration_ms} ms</span>
              </div>
              <DialogTitle className="mt-3 font-mono text-[15px]">
                {log.method} {log.path}
              </DialogTitle>
              <DialogDescription className="mt-1">
                {log.operation} · {localTime(log.created_at)}
              </DialogDescription>
            </div>
            <div className="flex-1 space-y-6 overflow-y-auto px-6 py-5">
              <div>
                <div className="mb-2 flex items-center justify-between">
                  <h4 className="text-sm font-medium">Request body</h4>
                  {info && <CopyButton value={curl(info.base_url, log)} label="cURL" />}
                </div>
                <CodeBlock code={log.body} json />
              </div>
              <div>
                <h4 className="mb-2 text-sm font-medium">Response</h4>
                <CodeBlock code={log.response_body || (log.status === 0 ? "(connection dropped)" : "")} json />
              </div>
              <div>
                <h4 className="mb-2 text-sm font-medium">Headers</h4>
                <div className="divide-y rounded-lg border text-[12.5px]">
                  {Object.entries(log.headers ?? {}).map(([k, v]) => (
                    <div key={k} className="flex gap-3 px-3 py-1.5">
                      <Mono className="w-40 shrink-0 text-muted-foreground">{k}</Mono>
                      <Mono className="min-w-0 break-all">{v}</Mono>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </>
        )}
      </SheetContent>
    </Dialog>
  );
}

export function RequestsPage() {
  const { runId } = useSandbox();
  const { params, navigate } = useRoute();
  const ref = params.get("ref") ?? "";
  const { data: logs } = useApi<RequestLog[]>(runPath(runId, `/requests${ref ? `?ref=${encodeURIComponent(ref)}` : ""}`), ["requests"]);
  const [query, setQuery] = useState("");
  const selected = logs?.find((l) => String(l.id) === params.get("id"));

  const q = query.trim().toLowerCase();
  const rows = (logs ?? []).filter((l) => !q || [l.operation, l.ref, l.resp_code, l.body].some((v) => v?.toLowerCase().includes(q)));
  const open = (l: RequestLog) => navigate("requests", ref ? { ref, id: String(l.id) } : { id: String(l.id) });

  return (
    <>
      <PageHeader title="API log" description="Every call to an Orchard endpoint, with the exact body, headers and the sandbox's reply." />
      <Card>
        <div className="flex flex-col gap-3 border-b p-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-2">
            {ref ? (
              <Badge tone="blue" className="py-1 pl-2 pr-1">
                Reference <Mono>{ref}</Mono>
                <button onClick={() => navigate("requests")} className="rounded p-0.5 hover:bg-sky-500/20" title="Clear filter">
                  <X className="h-3 w-3" />
                </button>
              </Badge>
            ) : (
              <span className="px-1 text-[13px] text-muted-foreground">{logs ? `${logs.length} requests` : ""}</span>
            )}
          </div>
          <div className="relative sm:w-72">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input placeholder="Search endpoint, reference, code" className="pl-8" value={query} onChange={(e) => setQuery(e.target.value)} />
          </div>
        </div>
        {logs && rows.length === 0 ? (
          <EmptyState icon={ScrollText} title={logs.length ? "No matching requests" : "No requests yet"} description={logs.length ? undefined : "Send a request to any Orchard endpoint, or use the Playground."} />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Endpoint</TableHead>
                <TableHead className="hidden md:table-cell">Reference</TableHead>
                <TableHead>Result</TableHead>
                <TableHead className="hidden text-right sm:table-cell">Duration</TableHead>
                <TableHead className="text-right">Time</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((l) => (
                <TableRow key={l.id} onClick={() => open(l)}>
                  <TableCell>
                    <Mono>{l.operation}</Mono>
                  </TableCell>
                  <TableCell className="hidden md:table-cell">
                    <Mono className="text-muted-foreground">{l.ref || "—"}</Mono>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1.5">
                      <RespCode code={l.resp_code} status={l.status} />
                      {l.status !== 200 && l.resp_code && <span className="font-mono text-xs text-muted-foreground">HTTP {l.status}</span>}
                    </div>
                  </TableCell>
                  <TableCell className="tabular hidden text-right text-muted-foreground sm:table-cell">{l.duration_ms} ms</TableCell>
                  <TableCell className="text-right text-muted-foreground">{timeAgo(l.created_at)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Card>
      <RequestSheet log={selected} onClose={() => navigate("requests", ref ? { ref } : undefined)} />
    </>
  );
}
