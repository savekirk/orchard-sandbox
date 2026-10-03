import { useState } from "react";
import { Activity, ArrowLeftRight, ArrowRight, Eye, EyeOff, FlaskConical, MessageSquare, Webhook, type LucideIcon } from "lucide-react";
import { CodeBlock, CopyButton, EmptyState, Mono, PageHeader, RespCode, Segmented, TxStatus, TypeBadge } from "@/components/common";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useApi } from "@/hooks/useApi";
import { runPath } from "@/lib/api";
import { useRoute } from "@/lib/router";
import { useSandbox } from "@/lib/sandbox";
import { cn, money, timeAgo } from "@/lib/utils";
import type { Overview, RequestLog, Run, Transaction } from "@/types";

function Credential({ label, value, secret }: { label: string; value: string; secret?: boolean }) {
  const [shown, setShown] = useState(!secret);
  return (
    <div className="min-w-0 rounded-lg border bg-muted/30 px-3 py-2.5">
      <div className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">{label}</div>
      <div className="mt-1 flex items-center gap-1">
        <Mono className="min-w-0 flex-1 truncate text-[13px]">{shown ? value : "•".repeat(Math.min(value.length, 24))}</Mono>
        {secret && (
          <button onClick={() => setShown(!shown)} title={shown ? "Hide" : "Reveal"} className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground">
            {shown ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
          </button>
        )}
        <CopyButton value={value} />
      </div>
    </div>
  );
}

type Lang = "curl" | "node" | "python";

function snippet(lang: Lang, base: string, run: Run) {
  const sid = /^\d+$/.test(run.service_id) ? run.service_id : JSON.stringify(run.service_id);
  switch (lang) {
    case "curl":
      return `BODY='{"service_id":${sid},"trans_type":"BLC","ts":"'"$(date -u '+%Y-%m-%d %H:%M:%S')"'"}'
SIG=$(printf '%s' "$BODY" | openssl dgst -sha256 -hmac '${run.secret_key}' | sed 's/^.* //')

curl -X POST ${base}/check_wallet_balance \\
  -H "Content-Type: application/json" \\
  -H "Authorization: ${run.client_key}:$SIG" \\
  -d "$BODY"`;
    case "node":
      return `import crypto from "node:crypto";

const body = JSON.stringify({
  service_id: ${sid},
  trans_type: "BLC",
  ts: new Date().toISOString().slice(0, 19).replace("T", " "),
});
const signature = crypto.createHmac("sha256", "${run.secret_key}").update(body).digest("hex");

const res = await fetch("${base}/check_wallet_balance", {
  method: "POST",
  headers: { "Content-Type": "application/json", Authorization: \`${run.client_key}:\${signature}\` },
  body,
});
console.log(await res.json());`;
    case "python":
      return `import hashlib, hmac, json, requests
from datetime import datetime, timezone

body = json.dumps({
    "service_id": ${sid},
    "trans_type": "BLC",
    "ts": datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S"),
})
signature = hmac.new(b"${run.secret_key}", body.encode(), hashlib.sha256).hexdigest()

res = requests.post("${base}/check_wallet_balance", data=body, headers={
    "Content-Type": "application/json",
    "Authorization": f"${run.client_key}:{signature}",
})
print(res.json())`;
  }
}

function ConnectCard({ run, base }: { run: Run; base: string }) {
  const [lang, setLang] = useState<Lang>("curl");
  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>Connect your app</CardTitle>
          <CardDescription>Swap your Orchard base URL and keys for these. Request and response formats are unchanged.</CardDescription>
        </div>
        {run.settings.require_auth ? <Badge tone="green">HMAC required</Badge> : <Badge tone="amber">Signatures not checked</Badge>}
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-2.5 sm:grid-cols-2">
          <Credential label="Base URL" value={base} />
          <Credential label="Service ID" value={run.service_id} />
          <Credential label="Client key" value={run.client_key} />
          <Credential label="Secret key" value={run.secret_key} secret />
        </div>
        <div>
          <div className="mb-2 flex items-center justify-between gap-2">
            <p className="text-[13px] text-muted-foreground">
              Sign the exact body: <Mono>Authorization: CLIENT_KEY:hex(HMAC-SHA256(body, SECRET_KEY))</Mono>
            </p>
            <Segmented<Lang>
              value={lang}
              onChange={setLang}
              options={[
                { value: "curl", label: "cURL" },
                { value: "node", label: "Node" },
                { value: "python", label: "Python" },
              ]}
            />
          </div>
          <CodeBlock code={snippet(lang, base, run)} />
        </div>
      </CardContent>
    </Card>
  );
}

function Stat({ label, value, hint, icon: Icon, tone, onClick }: { label: string; value: number | string; hint?: string; icon: LucideIcon; tone: string; onClick?: () => void }) {
  return (
    <button onClick={onClick} className="group rounded-xl border bg-card p-4 text-left shadow-sm transition-colors hover:border-foreground/20">
      <div className="flex items-center justify-between">
        <span className="text-[13px] text-muted-foreground">{label}</span>
        <span className={cn("rounded-md p-1.5", tone)}>
          <Icon className="h-3.5 w-3.5" />
        </span>
      </div>
      <div className="tabular mt-2 text-2xl font-semibold tracking-tight">{value}</div>
      {hint && <div className="mt-0.5 text-xs text-muted-foreground">{hint}</div>}
    </button>
  );
}

function BalanceTile({ label, value, unit = "GHS" }: { label: string; value: number; unit?: string }) {
  return (
    <div className="rounded-lg border bg-muted/30 px-3.5 py-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="tabular mt-1 text-[17px] font-semibold tracking-tight">
        {unit === "GHS" ? (
          <>
            <span className="mr-1 text-xs font-medium text-muted-foreground">GHS</span>
            {money(value)}
          </>
        ) : (
          <>
            {value.toLocaleString()} <span className="text-xs font-medium text-muted-foreground">{unit}</span>
          </>
        )}
      </div>
    </div>
  );
}

export function OverviewPage() {
  const { runId, run, info } = useSandbox();
  const { navigate } = useRoute();
  const { data: overview } = useApi<Overview>(runPath(runId, "/overview"), ["transactions", "balances", "callbacks", "requests", "sms", "settings"]);
  const { data: txns } = useApi<Transaction[]>(runPath(runId, "/transactions"), ["transactions"]);
  const { data: logs } = useApi<RequestLog[]>(runPath(runId, "/requests"), ["requests"]);

  const current = overview?.run ?? run;
  const counts = overview?.counts;
  const b = overview?.balances;

  return (
    <>
      <PageHeader
        title="Overview"
        description="A local stand-in for the Orchard API. Point your client here and watch requests, payments and callbacks flow."
        actions={
          <Button variant="outline" size="sm" onClick={() => navigate("playground")}>
            <FlaskConical /> Try a request
          </Button>
        }
      />

      <div className="grid gap-5">
        {current && info && <ConnectCard run={current} base={info.base_url} />}

        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <Stat label="API requests" value={counts?.requests ?? "–"} icon={Activity} tone="bg-sky-500/10 text-sky-600 dark:text-sky-400" onClick={() => navigate("requests")} />
          <Stat
            label="Transactions"
            value={counts?.transactions ?? "–"}
            hint={counts?.pending ? `${counts.pending} pending` : "none pending"}
            icon={ArrowLeftRight}
            tone="bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
            onClick={() => navigate("transactions")}
          />
          <Stat
            label="Failed callbacks"
            value={counts?.callbacks_failed ?? "–"}
            icon={Webhook}
            tone={counts?.callbacks_failed ? "bg-red-500/10 text-red-600 dark:text-red-400" : "bg-muted text-muted-foreground"}
            onClick={() => navigate("callbacks")}
          />
          <Stat label="SMS sent" value={counts?.sms ?? "–"} icon={MessageSquare} tone="bg-violet-500/10 text-violet-600 dark:text-violet-400" onClick={() => navigate("messages")} />
        </div>

        <Card>
          <CardHeader>
            <div>
              <CardTitle>Wallet balances</CardTitle>
              <CardDescription>
                What <Mono>/check_wallet_balance</Mono> returns. Payouts reserve funds when accepted; collections credit when they succeed.
              </CardDescription>
            </div>
            <Button variant="ghost" size="sm" onClick={() => navigate("settings")}>
              Adjust
            </Button>
          </CardHeader>
          <CardContent>
            {b && (
              <div className="grid grid-cols-2 gap-2.5 md:grid-cols-3 lg:grid-cols-6">
                <BalanceTile label="Available collect" value={b.available_collect_bal} />
                <BalanceTile label="Actual collect" value={b.actual_collect_bal} />
                <BalanceTile label="Payout" value={b.payout_bal} />
                <BalanceTile label="Bill pay" value={b.billpay_bal} />
                <BalanceTile label="Airtime" value={b.airtime_bal} />
                <BalanceTile label="SMS" value={b.sms_bal} unit="units" />
              </div>
            )}
          </CardContent>
        </Card>

        <div className="grid gap-5 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>Recent transactions</CardTitle>
              <Button variant="ghost" size="xs" onClick={() => navigate("transactions")}>
                View all <ArrowRight />
              </Button>
            </CardHeader>
            <CardContent className="px-0 pb-2">
              {txns && txns.length === 0 && <EmptyState icon={ArrowLeftRight} title="No transactions yet" description="Send a CTM or MTC request to /sendRequest." />}
              {txns?.slice(0, 6).map((t) => (
                <button key={t.exttrid} onClick={() => navigate("transactions", { ref: t.exttrid })} className="flex w-full items-center gap-3 px-5 py-2 text-left hover:bg-muted/50">
                  <TypeBadge type={t.trans_type} />
                  <div className="min-w-0 flex-1">
                    <Mono className="block truncate">{t.exttrid}</Mono>
                    <div className="text-xs text-muted-foreground">{timeAgo(t.created_at)}</div>
                  </div>
                  <span className="tabular text-sm font-medium">{money(t.amount)}</span>
                  <TxStatus status={t.status} />
                </button>
              ))}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Recent API calls</CardTitle>
              <Button variant="ghost" size="xs" onClick={() => navigate("requests")}>
                View all <ArrowRight />
              </Button>
            </CardHeader>
            <CardContent className="px-0 pb-2">
              {logs && logs.length === 0 && <EmptyState icon={Activity} title="No requests yet" description="Calls to any Orchard endpoint show up here instantly." />}
              {logs?.slice(0, 6).map((l) => (
                <button key={l.id} onClick={() => navigate("requests", { id: String(l.id) })} className="flex w-full items-center gap-3 px-5 py-2 text-left hover:bg-muted/50">
                  <div className="min-w-0 flex-1">
                    <Mono className="block truncate">{l.operation}</Mono>
                    <div className="truncate text-xs text-muted-foreground">
                      {l.ref || "—"} · {timeAgo(l.created_at)}
                    </div>
                  </div>
                  <RespCode code={l.resp_code} status={l.status} />
                </button>
              ))}
            </CardContent>
          </Card>
        </div>

        {info && (
          <Card>
            <CardHeader>
              <div>
                <CardTitle>Trigger outcomes with test numbers</CardTitle>
                <CardDescription>The last digits of a customer, wallet or card number choose what happens. Anything else succeeds.</CardDescription>
              </div>
            </CardHeader>
            <CardContent className="grid gap-6 md:grid-cols-2">
              <div className="space-y-1.5">
                <div className="text-xs font-medium text-muted-foreground">Payments · account inquiry</div>
                {info.scenarios.map((s) => (
                  <div key={s.key} className="flex items-center gap-3 text-sm">
                    <Mono className="w-28 shrink-0 text-muted-foreground">…{s.suffix}</Mono>
                    <span>{s.description}</span>
                  </div>
                ))}
              </div>
              <div className="space-y-1.5">
                <div className="text-xs font-medium text-muted-foreground">Ghana Card · last digit</div>
                {info.card_scenarios.map((s) => (
                  <div key={s.digit} className="flex items-center gap-3 text-sm">
                    <Mono className="w-28 shrink-0 text-muted-foreground">{s.digit === "other" ? "any other" : `GHA-…-${s.digit}`}</Mono>
                    <span>{s.description}</span>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        )}
      </div>
    </>
  );
}
