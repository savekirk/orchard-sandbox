import { useEffect, useMemo, useState } from "react";
import { ExternalLink, FlaskConical, RefreshCw, Send } from "lucide-react";
import { CodeBlock, CopyButton, Mono, PageHeader, RespCode } from "@/components/common";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Select, Textarea } from "@/components/ui/input";
import { Field } from "@/components/ui/label";
import { api, runPath } from "@/lib/api";
import { useSandbox } from "@/lib/sandbox";
import { orchardTimestamp, randomRef } from "@/lib/utils";
import type { TryResult } from "@/types";

interface Template {
  label: string;
  path: string;
  body: (ctx: { sid: number | string; callback: string; landing: string }) => Record<string, unknown>;
}

const templates: Template[] = [
  { label: "Collect from a wallet (CTM)", path: "/sendRequest", body: ({ sid, callback }) => ({ service_id: sid, trans_type: "CTM", exttrid: randomRef("CTM"), amount: "10.00", customer_number: "0241234567", nw: "MTN", reference: "Order 1001", callback_url: callback, ts: orchardTimestamp() }) },
  { label: "Pay out to a wallet (MTC)", path: "/sendRequest", body: ({ sid, callback }) => ({ service_id: sid, trans_type: "MTC", exttrid: randomRef("MTC"), amount: "25.00", customer_number: "0201234567", nw: "VOD", reference: "Payout", callback_url: callback, ts: orchardTimestamp() }) },
  { label: "Pay out to a bank account (MTC)", path: "/sendRequest", body: ({ sid, callback }) => ({ service_id: sid, trans_type: "MTC", exttrid: randomRef("BNK"), amount: "150.00", customer_number: "1020304050", nw: "BNK", bank_code: "GCB", recipient_name: "Kofi Boateng", reference: "Salary", callback_url: callback, ts: orchardTimestamp() }) },
  { label: "Look up an account name (AII)", path: "/sendRequest", body: ({ sid }) => ({ service_id: sid, trans_type: "AII", exttrid: randomRef("AII"), customer_number: "0241234567", nw: "BNK", bank_code: "MTN", ts: orchardTimestamp() }) },
  { label: "Buy airtime (ATP)", path: "/sendRequest", body: ({ sid, callback }) => ({ service_id: sid, trans_type: "ATP", exttrid: randomRef("ATP"), amount: "5.00", customer_number: "0241234567", nw: "MTN", reference: "Airtime", callback_url: callback, ts: orchardTimestamp() }) },
  { label: "Pay a bill (BLP)", path: "/sendRequest", body: ({ sid, callback }) => ({ service_id: sid, trans_type: "BLP", exttrid: randomRef("BLP"), amount: "120.00", account_number: "123456789012", nw: "DST", callback_url: callback, ts: orchardTimestamp() }) },
  {
    label: "Receive a remittance (RMT)",
    path: "/sendRequest",
    body: ({ sid, callback }) => ({
      service_id: sid, trans_type: "RMT", exttrid: randomRef("RMT"), amount: "100.00", transf_amount: "6.25", customer_number: "0241234567", nw: "MTN",
      reference: "Family", sender_name: "John Smith", sender_number: "447700900123", sender_gender: "M", recipient_name: "Ama Mensah",
      recipient_address: "Accra", recipient_gender: "F", ctry_origin_code: "GBR", transf_curr_code: "GBP", transf_purpose: "Support", callback_url: callback, ts: orchardTimestamp(),
    }),
  },
  { label: "Hosted checkout", path: "/third_party_request", body: ({ sid, callback, landing }) => ({ service_id: sid, exttrid: randomRef("CHK"), amount: "49.99", reference: "Basket", callback_url: callback, landing_page: landing, payment_mode: "CRM", nickname: "Demo Shop", ts: orchardTimestamp() }) },
  { label: "Check a transaction (TSC)", path: "/checkTransaction", body: ({ sid }) => ({ service_id: sid, trans_type: "TSC", exttrid: "PASTE-AN-EXTTRID" }) },
  { label: "Check wallet balance (BLC)", path: "/check_wallet_balance", body: ({ sid }) => ({ service_id: sid, trans_type: "BLC", ts: orchardTimestamp() }) },
  { label: "Send an SMS", path: "/sendSms", body: ({ sid }) => ({ service_id: sid, trans_type: "SMS", msg_type: "T", sender_id: "DEMO", recipient_number: "233241234567", msg_body: "Your one-time password is 72755", unique_id: randomRef("SMS") }) },
  { label: "Verify a Ghana Card", path: "/verifyID", body: ({ sid }) => ({ service_id: sid, trans_type: "AII", id_type: "GCA", id_num: "GHA-123456789-0", exttrid: randomRef("ID"), image: "aGVsbG8gc2FuZGJveA==", ts: orchardTimestamp() }) },
  {
    label: "Subscribe to auto debit (SUB)",
    path: "/autoDebit",
    body: ({ sid, callback }) => ({ service_id: sid, operation: "SUB", uniq_ref_id: randomRef("SUB"), customer_number: "233241234567", amount: "5.00", nw: "MTN", cycle: "MON", start_date: orchardTimestamp().slice(0, 10), reference: "Loan", return_url: callback, resumable: "Y", cycle_skip: "N", ts: orchardTimestamp() }),
  },
];

export function PlaygroundPage() {
  const { runId, run, info } = useSandbox();
  const [index, setIndex] = useState(0);
  const [body, setBody] = useState("");
  const [result, setResult] = useState<TryResult>();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  const ctx = useMemo(() => {
    const base = info?.base_url ?? window.location.origin;
    const sid = run && /^\d+$/.test(run.service_id) ? Number(run.service_id) : run?.service_id ?? 1234;
    return { sid, callback: `${base}/mock/sink`, landing: `${base}/dashboard/transactions` };
  }, [info, run]);

  const regenerate = (i = index) => setBody(JSON.stringify(templates[i].body(ctx), null, 2));
  useEffect(() => regenerate(), [ctx]);

  const send = async () => {
    setBusy(true);
    setError(undefined);
    try {
      JSON.parse(body);
    } catch {
      setError("The body is not valid JSON.");
      setBusy(false);
      return;
    }
    try {
      setResult(await api<TryResult>(runPath(runId, "/try"), { method: "POST", body: { path: templates[index].path, body: JSON.parse(body) } }));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  let parsed: Record<string, unknown> | undefined;
  try {
    parsed = result ? JSON.parse(result.body) : undefined;
  } catch {
    parsed = undefined;
  }
  const code = parsed && (typeof parsed.resp_code === "string" ? parsed.resp_code : typeof parsed.trans_status === "string" ? parsed.trans_status : "");
  const redirect = typeof parsed?.redirect_url === "string" ? parsed.redirect_url : undefined;

  return (
    <>
      <PageHeader title="Playground" description="Send real Orchard requests without writing code. The sandbox signs each one with this sandbox's keys and logs it like any other call." />
      <div className="grid gap-5 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <div>
              <CardTitle>Request</CardTitle>
              <CardDescription>
                POST <Mono>{templates[index].path}</Mono>
              </CardDescription>
            </div>
            <Button variant="ghost" size="xs" onClick={() => regenerate()} title="New references and timestamp">
              <RefreshCw /> Refresh refs
            </Button>
          </CardHeader>
          <CardContent className="space-y-4">
            <Field label="Example" htmlFor="template">
              <Select
                id="template"
                value={index}
                onChange={(e) => {
                  const i = Number(e.target.value);
                  setIndex(i);
                  regenerate(i);
                  setResult(undefined);
                }}
              >
                {templates.map((t, i) => (
                  <option key={t.label} value={i}>
                    {t.label}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="JSON body" htmlFor="body" hint="Edit freely: try a test number like 0240000001, remove a field, or reuse an exttrid.">
              <Textarea id="body" spellCheck={false} className="min-h-[340px] font-mono text-[12.5px] leading-relaxed" value={body} onChange={(e) => setBody(e.target.value)} />
            </Field>
            <Button onClick={send} disabled={busy} className="w-full">
              <Send /> {busy ? "Sending…" : "Send request"}
            </Button>
          </CardContent>
        </Card>

        <Card className="lg:sticky lg:top-6 lg:self-start">
          <CardHeader>
            <div>
              <CardTitle>Response</CardTitle>
              <CardDescription>{result ? `${result.duration_ms} ms` : "Send a request to see the reply."}</CardDescription>
            </div>
            {result && (
              <div className="flex items-center gap-1.5">
                {code && <RespCode code={code} />}
                <Badge tone={result.status < 300 ? "neutral" : "red"} className="font-mono">
                  HTTP {result.status}
                </Badge>
              </div>
            )}
          </CardHeader>
          <CardContent className="space-y-4">
            {error && <div className="rounded-lg border border-red-500/30 bg-red-500/5 px-4 py-3 text-sm text-red-600 dark:text-red-400">{error}</div>}
            {!result && !error && (
              <div className="flex flex-col items-center rounded-lg border border-dashed py-16 text-center text-sm text-muted-foreground">
                <FlaskConical className="mb-2 h-5 w-5" />
                Nothing sent yet
              </div>
            )}
            {result && (
              <>
                <CodeBlock code={result.body || "(empty response)"} json />
                {redirect && (
                  <Button asChild variant="outline" size="sm">
                    <a href={redirect} target="_blank" rel="noreferrer">
                      <ExternalLink /> Open checkout page
                    </a>
                  </Button>
                )}
                <div>
                  <div className="mb-1.5 flex items-center justify-between text-xs font-medium text-muted-foreground">
                    Authorization header sent
                    <CopyButton value={result.request.authorization} />
                  </div>
                  <Mono className="block break-all rounded-lg border bg-muted/40 px-3 py-2 text-muted-foreground">{result.request.authorization}</Mono>
                </div>
              </>
            )}
          </CardContent>
        </Card>
      </div>
    </>
  );
}
