import { useEffect, useState, type FormEvent } from "react";
import { Plus, RotateCcw, Trash2 } from "lucide-react";
import { Mono, PageHeader, Segmented } from "@/components/common";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Field } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useApi } from "@/hooks/useApi";
import { api, runPath } from "@/lib/api";
import { useSandbox } from "@/lib/sandbox";
import { useToast } from "@/lib/toast";
import { timeAgo } from "@/lib/utils";
import type { Balances, Run, Settings } from "@/types";

function BehaviourCard({ run }: { run: Run }) {
  const toast = useToast();
  const { reloadRuns } = useSandbox();
  const [s, setS] = useState<Settings>(run.settings);
  useEffect(() => setS(run.settings), [run.settings]);
  const num = (k: keyof Settings) => (e: { target: { value: string } }) => setS({ ...s, [k]: Number(e.target.value) });

  const save = async (e: FormEvent) => {
    e.preventDefault();
    try {
      await api(runPath(run.run_id, "/settings"), { method: "PUT", body: s });
      await reloadRuns();
      toast("Settings saved");
    } catch (err) {
      toast((err as Error).message, true);
    }
  };

  return (
    <Card>
      <form onSubmit={save}>
        <CardHeader>
          <div>
            <CardTitle>Behaviour</CardTitle>
            <CardDescription>How this sandbox authenticates, settles payments and delivers callbacks.</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="space-y-5">
          <label className="flex items-center justify-between gap-4 rounded-lg border p-3.5">
            <div>
              <div className="text-sm font-medium">Require HMAC signatures</div>
              <p className="text-[13px] text-muted-foreground">Like Orchard, reject unsigned or badly signed requests with 101, 102 or 103.</p>
            </div>
            <Switch checked={s.require_auth} onCheckedChange={(require_auth) => setS({ ...s, require_auth })} />
          </label>

          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Settlement" hint={s.settle_mode === "auto" ? "Payments settle by themselves after the delay." : "Payments stay pending until you approve or fail them."}>
              <div>
                <Segmented<Settings["settle_mode"]>
                  value={s.settle_mode}
                  onChange={(settle_mode) => setS({ ...s, settle_mode })}
                  options={[
                    { value: "auto", label: "Automatic" },
                    { value: "manual", label: "Manual" },
                  ]}
                />
              </div>
            </Field>
            {s.settle_mode === "auto" && (
              <Field label="Settle delay (ms)" htmlFor="delay" hint="Time a customer takes to approve the prompt.">
                <Input id="delay" type="number" min={0} step={100} value={s.settle_delay_ms} onChange={num("settle_delay_ms")} />
              </Field>
            )}
            <Field label="Response latency (ms)" htmlFor="latency" hint="Added to every API response.">
              <Input id="latency" type="number" min={0} step={50} value={s.latency_ms} onChange={num("latency_ms")} />
            </Field>
            <Field label="Callback attempts" htmlFor="attempts" hint="Retries back off from 2s up to 1 minute.">
              <Input id="attempts" type="number" min={1} max={20} value={s.callback_attempts} onChange={num("callback_attempts")} />
            </Field>
            <Field label="Callback timeout (ms)" htmlFor="timeout">
              <Input id="timeout" type="number" min={100} step={500} value={s.callback_timeout_ms} onChange={num("callback_timeout_ms")} />
            </Field>
          </div>
        </CardContent>
        <CardFooter>
          <Button type="submit" size="sm">
            Save behaviour
          </Button>
        </CardFooter>
      </form>
    </Card>
  );
}

const balanceFields: [keyof Balances, string][] = [
  ["available_collect_bal", "Available collect (GHS)"],
  ["actual_collect_bal", "Actual collect (GHS)"],
  ["payout_bal", "Payout (GHS)"],
  ["billpay_bal", "Bill pay (GHS)"],
  ["airtime_bal", "Airtime (GHS)"],
  ["sms_bal", "SMS (units)"],
];

function BalancesCard({ runId }: { runId: string }) {
  const toast = useToast();
  const { data } = useApi<Balances>(runPath(runId, "/balances"), ["balances"]);
  const [values, setValues] = useState<Record<string, string>>({});
  useEffect(() => {
    if (data) setValues(Object.fromEntries(balanceFields.map(([k]) => [k, k === "sms_bal" ? String(data[k]) : data[k].toFixed(2)])));
  }, [data]);

  const save = async (e: FormEvent) => {
    e.preventDefault();
    try {
      await api(runPath(runId, "/balances"), { method: "PUT", body: Object.fromEntries(Object.entries(values).map(([k, v]) => [k, Number(v)])) });
      toast("Balances updated");
    } catch (err) {
      toast((err as Error).message, true);
    }
  };

  return (
    <Card>
      <form onSubmit={save}>
        <CardHeader>
          <div>
            <CardTitle>Wallet balances</CardTitle>
            <CardDescription>Set balances directly, for example to test insufficient funds (038). Changes are recorded in the ledger.</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="grid grid-cols-2 gap-4 md:grid-cols-3">
          {balanceFields.map(([k, label]) => (
            <Field key={k} label={label} htmlFor={k}>
              <Input id={k} type="number" min={0} step={k === "sms_bal" ? 1 : 0.01} className="tabular" value={values[k] ?? ""} onChange={(e) => setValues({ ...values, [k]: e.target.value })} />
            </Field>
          ))}
        </CardContent>
        <CardFooter>
          <Button type="submit" size="sm">
            Update balances
          </Button>
        </CardFooter>
      </form>
    </Card>
  );
}

function SandboxesCard() {
  const { runs, runId, selectRun, reloadRuns } = useSandbox();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ run_id: "", name: "" });

  const create = async (e: FormEvent) => {
    e.preventDefault();
    try {
      const run = await api<Run>("/mock/runs", { method: "POST", body: { run_id: form.run_id.trim() || undefined, name: form.name.trim() || undefined } });
      await reloadRuns();
      selectRun(run.run_id);
      setOpen(false);
      setForm({ run_id: "", name: "" });
      toast(`Sandbox ${run.run_id} created with its own keys`);
    } catch (err) {
      toast((err as Error).message, true);
    }
  };

  const remove = async (id: string) => {
    if (!window.confirm(`Delete sandbox "${id}" and all of its data?`)) return;
    try {
      await api(`/mock/runs/${encodeURIComponent(id)}`, { method: "DELETE" });
      if (id === runId) selectRun("default");
      await reloadRuns();
      toast(`Sandbox ${id} deleted`);
    } catch (err) {
      toast((err as Error).message, true);
    }
  };

  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>Sandboxes</CardTitle>
          <CardDescription>Each sandbox has its own keys, balances and data, so parallel test suites never collide. Requests are routed by client key.</CardDescription>
        </div>
        <Button size="sm" variant="outline" onClick={() => setOpen(true)}>
          <Plus /> New sandbox
        </Button>
      </CardHeader>
      <div className="divide-y border-t">
        {runs.map((r) => (
          <div key={r.run_id} className="flex items-center gap-3 px-5 py-3">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 text-sm font-medium">
                {r.name}
                {r.run_id === runId && <Badge tone="green">Selected</Badge>}
              </div>
              <div className="truncate text-xs text-muted-foreground">
                <Mono>{r.run_id}</Mono> · key <Mono>{r.client_key}</Mono> · {timeAgo(r.created_at)}
              </div>
            </div>
            {r.run_id !== runId && (
              <Button size="xs" variant="ghost" onClick={() => selectRun(r.run_id)}>
                Switch
              </Button>
            )}
            {r.run_id !== "default" && (
              <Button size="icon-sm" variant="ghost" title="Delete sandbox" onClick={() => remove(r.run_id)}>
                <Trash2 />
              </Button>
            )}
          </div>
        ))}
      </div>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <form onSubmit={create} className="grid gap-4">
            <DialogHeader>
              <DialogTitle>New sandbox</DialogTitle>
              <DialogDescription>Gets fresh client and secret keys, and copies settings and opening balances from the default sandbox.</DialogDescription>
            </DialogHeader>
            <Field label="ID" htmlFor="run_id" hint="Optional. Letters, numbers, dashes.">
              <Input id="run_id" className="font-mono" placeholder="ci-checkout" value={form.run_id} onChange={(e) => setForm({ ...form, run_id: e.target.value })} />
            </Field>
            <Field label="Name" htmlFor="run_name">
              <Input id="run_name" placeholder="Checkout CI" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            </Field>
            <DialogFooter>
              <Button type="submit">Create sandbox</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </Card>
  );
}

export function SettingsPage() {
  const { runId, run } = useSandbox();
  const toast = useToast();

  const reset = async () => {
    if (!window.confirm("Clear all transactions, requests, callbacks, messages and mandates, and restore opening balances?")) return;
    try {
      await api(runPath(runId, "/reset"), { method: "POST" });
      toast("Sandbox reset");
    } catch (err) {
      toast((err as Error).message, true);
    }
  };

  return (
    <>
      <PageHeader title="Settings" description={<>Configuring <Mono>{runId}</Mono>. Start-up defaults come from environment variables or a config file.</>} />
      <div className="grid gap-5">
        {run && <BehaviourCard run={run} />}
        <BalancesCard runId={runId} />
        <SandboxesCard />
        <Card className="border-red-500/30">
          <CardHeader>
            <div>
              <CardTitle>Reset this sandbox</CardTitle>
              <CardDescription>Clears activity and restores opening balances. Keys, settings, test data and failure rules are kept.</CardDescription>
            </div>
            <Button size="sm" variant="destructive" onClick={reset}>
              <RotateCcw /> Reset
            </Button>
          </CardHeader>
        </Card>
      </div>
    </>
  );
}
