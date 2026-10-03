import { useState, type FormEvent } from "react";
import { Building2, IdCard, Plus, Trash2 } from "lucide-react";
import { EmptyState, Mono, PageHeader, RespCode } from "@/components/common";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input, Select } from "@/components/ui/input";
import { Field } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useApi } from "@/hooks/useApi";
import { api, runPath } from "@/lib/api";
import { useSandbox } from "@/lib/sandbox";
import { useToast } from "@/lib/toast";
import type { Account, Card as GhanaCard } from "@/types";

const emptyCard: GhanaCard = { id_num: "", name: "", gender: "F", verified: "true", card_valid_start: "2021-01-01", card_valid_end: "2031-01-01" };

function CardDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const { runId } = useSandbox();
  const toast = useToast();
  const [card, setCard] = useState(emptyCard);
  const set = (k: keyof GhanaCard) => (e: { target: { value: string } }) => setCard({ ...card, [k]: e.target.value });

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    try {
      await api(runPath(runId, "/cards"), { method: "POST", body: card });
      toast(`Card ${card.id_num} saved`);
      setCard(emptyCard);
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
            <DialogTitle>Add Ghana Card</DialogTitle>
            <DialogDescription>/verifyID returns exactly this identity for the card number.</DialogDescription>
          </DialogHeader>
          <Field label="Card number" htmlFor="id_num">
            <Input id="id_num" required placeholder="GHA-123456789-0" pattern="GHA-\d{9}-\d" className="font-mono" value={card.id_num} onChange={set("id_num")} />
          </Field>
          <div className="grid grid-cols-[1fr_auto] gap-3">
            <Field label="Name on card" htmlFor="name">
              <Input id="name" value={card.name} onChange={set("name")} placeholder="Ama Mensah" />
            </Field>
            <Field label="Gender" htmlFor="gender">
              <Select id="gender" value={card.gender} onChange={set("gender")}>
                <option value="F">F</option>
                <option value="M">M</option>
              </Select>
            </Field>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Valid from" htmlFor="start">
              <Input id="start" value={card.card_valid_start} onChange={set("card_valid_start")} placeholder="YYYY-MM-DD" />
            </Field>
            <Field label="Valid until" htmlFor="end">
              <Input id="end" value={card.card_valid_end} onChange={set("card_valid_end")} placeholder="YYYY-MM-DD" />
            </Field>
          </div>
          <Field label="Face match" htmlFor="verified">
            <Select id="verified" value={card.verified} onChange={set("verified")}>
              <option value="true">Verified</option>
              <option value="false">Not verified</option>
            </Select>
          </Field>
          <DialogFooter>
            <Button type="submit">Save card</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

const emptyAccount: Account = { bank_code: "GCB", account_number: "", account_name: "", resp_code: "027" };

function AccountDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const { runId, info } = useSandbox();
  const toast = useToast();
  const [account, setAccount] = useState(emptyAccount);
  const set = (k: keyof Account) => (e: { target: { value: string } }) => setAccount({ ...account, [k]: e.target.value });

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    try {
      await api(runPath(runId, "/accounts"), { method: "POST", body: account });
      toast(`Account ${account.account_number} saved`);
      setAccount(emptyAccount);
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
            <DialogTitle>Add account</DialogTitle>
            <DialogDescription>Account inquiry (trans_type AII) returns this name for the bank code and number.</DialogDescription>
          </DialogHeader>
          <Field label="Bank or wallet" htmlFor="bank">
            <Select id="bank" value={account.bank_code} onChange={set("bank_code")}>
              {info?.bank_codes.map((b) => (
                <option key={b.code} value={b.code}>
                  {b.name} ({b.code})
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Account or wallet number" htmlFor="number">
            <Input id="number" required className="font-mono" value={account.account_number} onChange={set("account_number")} placeholder="0241234567" />
          </Field>
          <Field label="Result" htmlFor="result">
            <Select id="result" value={account.resp_code} onChange={set("resp_code")}>
              <option value="027">027 · Return the name below</option>
              <option value="067">067 · No record returned</option>
              <option value="009">009 · Invalid customer number</option>
              <option value="013">013 · Request could not be completed</option>
            </Select>
          </Field>
          {account.resp_code === "027" && (
            <Field label="Account name" htmlFor="account_name">
              <Input id="account_name" required value={account.account_name} onChange={set("account_name")} placeholder="Kofi Boateng" />
            </Field>
          )}
          <DialogFooter>
            <Button type="submit">Save account</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function TestDataPage() {
  const { runId } = useSandbox();
  const toast = useToast();
  const { data: cards } = useApi<GhanaCard[]>(runPath(runId, "/cards"), ["cards"]);
  const { data: accounts } = useApi<Account[]>(runPath(runId, "/accounts"), ["accounts"]);
  const [cardOpen, setCardOpen] = useState(false);
  const [accountOpen, setAccountOpen] = useState(false);

  const remove = async (path: string) => {
    try {
      await api(runPath(runId, path), { method: "DELETE" });
    } catch (e) {
      toast((e as Error).message, true);
    }
  };

  return (
    <>
      <PageHeader title="Test data" description="Pin exact responses for identity checks and account lookups. Unregistered numbers fall back to the built-in rules." />
      <div className="grid gap-5">
        <Card>
          <CardHeader>
            <div>
              <CardTitle>Ghana Cards</CardTitle>
              <CardDescription>
                Returned by <Mono>/verifyID</Mono>. Cards not listed here follow the last-digit rules on the Overview page.
              </CardDescription>
            </div>
            <Button size="sm" variant="outline" onClick={() => setCardOpen(true)}>
              <Plus /> Add card
            </Button>
          </CardHeader>
          {cards && cards.length === 0 ? (
            <EmptyState icon={IdCard} title="No cards registered" />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Card number</TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>Face match</TableHead>
                  <TableHead>Validity</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {cards?.map((c) => (
                  <TableRow key={c.id_num}>
                    <TableCell>
                      <Mono>{c.id_num}</Mono>
                    </TableCell>
                    <TableCell>
                      {c.name || <span className="text-muted-foreground">(empty)</span>} <span className="text-xs text-muted-foreground">{c.gender}</span>
                    </TableCell>
                    <TableCell>{c.verified === "true" ? <Badge tone="green">Verified</Badge> : <Badge tone="red">Not verified</Badge>}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {c.card_valid_start || "?"} → {c.card_valid_end || "?"}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button size="icon-sm" variant="ghost" title="Delete" onClick={() => remove(`/cards/${encodeURIComponent(c.id_num)}`)}>
                        <Trash2 />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Card>

        <Card>
          <CardHeader>
            <div>
              <CardTitle>Accounts</CardTitle>
              <CardDescription>
                Returned by account inquiry (<Mono>AII</Mono>). Unregistered numbers get a stable generated name; numbers ending in <Mono>000003</Mono> return 067.
              </CardDescription>
            </div>
            <Button size="sm" variant="outline" onClick={() => setAccountOpen(true)}>
              <Plus /> Add account
            </Button>
          </CardHeader>
          {accounts && accounts.length === 0 ? (
            <EmptyState icon={Building2} title="No accounts registered" />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Bank</TableHead>
                  <TableHead>Number</TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>Result</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {accounts?.map((a) => (
                  <TableRow key={`${a.bank_code}/${a.account_number}`}>
                    <TableCell>
                      <Mono>{a.bank_code}</Mono>
                    </TableCell>
                    <TableCell>
                      <Mono>{a.account_number}</Mono>
                    </TableCell>
                    <TableCell>{a.account_name || <span className="text-muted-foreground">—</span>}</TableCell>
                    <TableCell>
                      <RespCode code={a.resp_code ?? "027"} />
                    </TableCell>
                    <TableCell className="text-right">
                      <Button size="icon-sm" variant="ghost" title="Delete" onClick={() => remove(`/accounts/${encodeURIComponent(a.bank_code)}/${encodeURIComponent(a.account_number)}`)}>
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
      <CardDialog open={cardOpen} onOpenChange={setCardOpen} />
      <AccountDialog open={accountOpen} onOpenChange={setAccountOpen} />
    </>
  );
}
