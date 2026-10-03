import { MessageSquare } from "lucide-react";
import { EmptyState, Mono, PageHeader } from "@/components/common";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { useApi } from "@/hooks/useApi";
import { runPath } from "@/lib/api";
import { useSandbox } from "@/lib/sandbox";
import { localTime, timeAgo } from "@/lib/utils";
import type { SMS } from "@/types";

export function MessagesPage() {
  const { runId } = useSandbox();
  const { data: messages } = useApi<SMS[]>(runPath(runId, "/sms"), ["sms"]);

  return (
    <>
      <PageHeader title="Messages" description="Texts accepted by /sendSms, plus auto debit activation codes the sandbox sends to customers. Nothing leaves this machine." />
      {messages && messages.length === 0 ? (
        <Card>
          <EmptyState icon={MessageSquare} title="No messages yet" description="Each 160 characters costs one SMS unit, just like Orchard." />
        </Card>
      ) : (
        <div className="grid gap-3 md:grid-cols-2">
          {messages?.map((m) => (
            <Card key={m.id} className="p-4">
              <div className="flex items-center justify-between gap-3">
                <div className="flex min-w-0 items-center gap-2">
                  <div className="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-violet-500/10 text-xs font-semibold text-violet-600 dark:text-violet-400">
                    {m.sender_id.slice(0, 2).toUpperCase()}
                  </div>
                  <div className="min-w-0">
                    <div className="truncate text-sm font-medium">{m.sender_id}</div>
                    <Mono className="text-muted-foreground">to {m.recipient}</Mono>
                  </div>
                </div>
                <span className="shrink-0 text-xs text-muted-foreground" title={localTime(m.created_at)}>
                  {timeAgo(m.created_at)}
                </span>
              </div>
              <p className="mt-3 whitespace-pre-wrap rounded-lg rounded-tl-sm bg-muted/60 px-3.5 py-2.5 text-sm">{m.body}</p>
              <div className="mt-2.5 flex items-center gap-2 text-xs text-muted-foreground">
                <Mono>{m.unique_id}</Mono>
                <span>·</span>
                <span>
                  {m.pages} {m.pages === 1 ? "unit" : "units"}
                </span>
                {m.msg_type === "F" && <Badge tone="amber">Flash</Badge>}
                {m.sender_id === "ORCHARD" && <Badge tone="violet">OTP</Badge>}
              </div>
            </Card>
          ))}
        </div>
      )}
    </>
  );
}
