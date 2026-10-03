import { useState, type FormEvent } from "react";
import { Pause, Play, Plus, Trash2, Zap } from "lucide-react";
import { EmptyState, Mono, PageHeader } from "@/components/common";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input, Select } from "@/components/ui/input";
import { Field } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useApi } from "@/hooks/useApi";
import { api, runPath } from "@/lib/api";
import { useSandbox } from "@/lib/sandbox";
import { useToast } from "@/lib/toast";
import { timeAgo } from "@/lib/utils";
import type { FailureRule, Hold } from "@/types";

const actions: Record<string, string> = {
  http_status: "Return an HTTP error",
  resp_code: "Return an Orchard error code",
  delayed_response: "Delay or hold the response",
  connection_failure: "Drop the connection",
  connection_close_after_accept: "Process, then drop the connection",
  malformed_json: "Return malformed JSON",
  missing_fields: "Return JSON without the expected fields",
  wrong_types: "Return fields with the wrong types",
  oversized_response: "Return a 1 MB body",
  redirect: "Redirect with 302",
};

const operations = [
  ["*", "Any endpoint"],
  ["sendRequest", "sendRequest (any type)"],
  ...["CTM", "MTC", "AII", "ATP", "BLP", "RMT", "AUD"].map((t) => [`sendRequest:${t}`, `sendRequest · ${t}`]),
  ["verifyID", "verifyID"],
  ["checkTransaction", "checkTransaction"],
  ["check_wallet_balance", "check_wallet_balance"],
  ["sendSms", "sendSms"],
  ["third_party_request", "third_party_request"],
  ["autoDebit", "autoDebit"],
];

function describe(rule: FailureRule) {
  switch (rule.action) {
    case "http_status":
      return `HTTP ${rule.http_status || 500}`;
    case "resp_code":
      return `resp_code ${rule.resp_code}`;
    case "delayed_response":
      return rule.hold ? "Hold until released" : `Wait ${rule.delay_ms ?? 0} ms`;
    default:
      return actions[rule.action] ?? rule.action;
  }
}

const emptyRule = { operation: "sendRequest:CTM", exttrid: "", attempt: 0, action: "http_status", http_status: 503, resp_code: "100", delay_ms: 5000, hold: false };

function RuleDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const { runId } = useSandbox();
  const toast = useToast();
  const [rule, setRule] = useState(emptyRule);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    try {
      await api(runPath(runId, "/failures"), { method: "POST", body: { ...rule, exttrid: rule.exttrid.trim() || "*", attempt: Number(rule.attempt) } });
      toast("Failure rule added");
      setRule(emptyRule);
      onOpenChange(false);
    } catch (err) {
      toast((err as Error).message, true);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>Add failure rule</DialogTitle>
            <DialogDescription>Matching requests are authenticated, then fail this way instead of being processed.</DialogDescription>
          </DialogHeader>
          <Field label="Endpoint" htmlFor="op">
            <Select id="op" value={rule.operation} onChange={(e) => setRule({ ...rule, operation: e.target.value })}>
              {operations.map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </Select>
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Reference" htmlFor="ref" hint="exttrid, unique_id or uniq_ref_id. Blank for all.">
              <Input id="ref" className="font-mono" value={rule.exttrid} onChange={(e) => setRule({ ...rule, exttrid: e.target.value })} placeholder="*" />
            </Field>
            <Field label="Attempt" htmlFor="attempt" hint="0 for every attempt, 1 for the first only.">
              <Input id="attempt" type="number" min={0} value={rule.attempt} onChange={(e) => setRule({ ...rule, attempt: Number(e.target.value) })} />
            </Field>
          </div>
          <Field label="Failure" htmlFor="action">
            <Select id="action" value={rule.action} onChange={(e) => setRule({ ...rule, action: e.target.value })}>
              {Object.entries(actions).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </Select>
          </Field>
          {rule.action === "http_status" && (
            <Field label="HTTP status" htmlFor="status">
              <Input id="status" type="number" min={100} max={599} value={rule.http_status} onChange={(e) => setRule({ ...rule, http_status: Number(e.target.value) })} />
            </Field>
          )}
          {rule.action === "resp_code" && (
            <Field label="Orchard resp_code" htmlFor="code" hint="For example 100 (IP not whitelisted), 013, 038 or 055.">
              <Input id="code" className="font-mono" value={rule.resp_code} onChange={(e) => setRule({ ...rule, resp_code: e.target.value })} />
            </Field>
          )}
          {rule.action === "delayed_response" && (
            <div className="grid gap-3">
              <label className="flex items-center justify-between rounded-lg border px-3 py-2.5 text-sm">
                Hold until released from this page
                <Switch checked={rule.hold} onCheckedChange={(hold) => setRule({ ...rule, hold })} />
              </label>
              {!rule.hold && (
                <Field label="Delay (ms)" htmlFor="delay">
                  <Input id="delay" type="number" min={0} value={rule.delay_ms} onChange={(e) => setRule({ ...rule, delay_ms: Number(e.target.value) })} />
                </Field>
              )}
            </div>
          )}
          <DialogFooter>
            <Button type="submit">Add rule</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function FailuresPage() {
  const { runId } = useSandbox();
  const toast = useToast();
  const { data: rules } = useApi<FailureRule[]>(runPath(runId, "/failures"), ["failures"]);
  const { data: holds } = useApi<Hold[]>(runPath(runId, "/holds"), ["holds"]);
  const [open, setOpen] = useState(false);

  const call = async (path: string, method: string, message: string) => {
    try {
      await api(runPath(runId, path), { method });
      toast(message);
    } catch (e) {
      toast((e as Error).message, true);
    }
  };

  return (
    <>
      <PageHeader
        title="Failure rules"
        description="Make specific requests fail so you can test timeouts, retries and error handling. The first matching rule wins."
        actions={
          <Button size="sm" onClick={() => setOpen(true)}>
            <Plus /> Add rule
          </Button>
        }
      />
      <div className="grid gap-5">
        {!!holds?.length && (
          <Card className="border-amber-500/40">
            <CardHeader>
              <div>
                <CardTitle className="flex items-center gap-2">
                  <Pause className="h-4 w-4 text-amber-500" /> Held requests
                </CardTitle>
                <CardDescription>Your client is waiting on these. Release one to let it finish normally.</CardDescription>
              </div>
            </CardHeader>
            <div className="divide-y border-t">
              {holds.map((h) => (
                <div key={h.id} className="flex items-center gap-3 px-5 py-2.5 text-sm">
                  <Mono>{h.operation}</Mono>
                  <Mono className="text-muted-foreground">{h.exttrid || "—"}</Mono>
                  <span className="text-xs text-muted-foreground">attempt {h.attempt} · {timeAgo(h.since)}</span>
                  <Button size="xs" variant="outline" className="ml-auto" onClick={() => call(`/holds/${h.id}/release`, "POST", "Request released")}>
                    <Play /> Release
                  </Button>
                </div>
              ))}
            </div>
          </Card>
        )}

        <Card>
          {rules && rules.length === 0 ? (
            <EmptyState icon={Zap} title="No failure rules" description="Everything behaves normally. Add a rule to simulate outages, slow responses or bad payloads.">
              <Button size="sm" variant="outline" onClick={() => setOpen(true)}>
                <Plus /> Add rule
              </Button>
            </EmptyState>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Endpoint</TableHead>
                  <TableHead>Reference</TableHead>
                  <TableHead>Attempt</TableHead>
                  <TableHead>Failure</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {rules?.map((r) => (
                  <TableRow key={r.id}>
                    <TableCell>
                      <Mono>{r.operation === "*" ? "any" : r.operation}</Mono>
                    </TableCell>
                    <TableCell>
                      <Mono className="text-muted-foreground">{r.exttrid === "*" ? "any" : r.exttrid}</Mono>
                    </TableCell>
                    <TableCell className="text-muted-foreground">{r.attempt ? `#${r.attempt}` : "every"}</TableCell>
                    <TableCell>
                      <Badge tone="red">{describe(r)}</Badge>
                    </TableCell>
                    <TableCell className="text-right">
                      <Button size="icon-sm" variant="ghost" title="Delete rule" onClick={() => call(`/failures/${r.id}`, "DELETE", "Rule removed")}>
                        <Trash2 />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Card>
      </div>
      <RuleDialog open={open} onOpenChange={setOpen} />
    </>
  );
}
