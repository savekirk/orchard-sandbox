export interface Settings {
  require_auth: boolean;
  settle_mode: "auto" | "manual";
  settle_delay_ms: number;
  latency_ms: number;
  callback_attempts: number;
  callback_timeout_ms: number;
}

export interface Run {
  run_id: string;
  name: string;
  client_key: string;
  secret_key: string;
  service_id: string;
  settings: Settings;
  created_at: string;
}

export interface Balances {
  sms_bal: number;
  payout_bal: number;
  billpay_bal: number;
  available_collect_bal: number;
  airtime_bal: number;
  actual_collect_bal: number;
}

export interface Counts {
  requests: number;
  transactions: number;
  pending: number;
  callbacks_failed: number;
  sms: number;
  subscriptions: number;
}

export interface Overview {
  run: Run;
  balances: Balances;
  counts: Counts;
}

export interface Transaction {
  run_id: string;
  exttrid: string;
  trans_id: string;
  trans_type: string;
  channel: string;
  customer_number: string;
  nw: string;
  amount_pesewas: number;
  amount: string;
  reference: string;
  callback_url: string;
  status: "PENDING" | "SUCCESSFUL" | "FAILED";
  trans_status: string;
  message: string;
  scenario: string;
  settle_at?: number;
  meta?: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export interface RequestLog {
  id: number;
  method: string;
  path: string;
  operation: string;
  ref: string;
  headers: Record<string, string>;
  body: string;
  status: number;
  response_body: string;
  resp_code: string;
  duration_ms: number;
  created_at: string;
}

export interface Callback {
  id: string;
  exttrid: string;
  url: string;
  payload: string;
  state: "QUEUED" | "DELIVERING" | "DELIVERED" | "FAILED";
  attempts: number;
  max_attempts: number;
  next_attempt_at?: number;
  last_status: number;
  last_error?: string;
  created_at: string;
  delivered_at?: string;
}

export interface CallbackAttempt {
  id: number;
  http_status: number;
  response_body: string;
  error?: string;
  duration_ms: number;
  attempted_at: string;
}

export interface SMS {
  id: number;
  unique_id: string;
  sender_id: string;
  recipient: string;
  body: string;
  msg_type: string;
  pages: number;
  created_at: string;
}

export interface Card {
  id_num: string;
  name: string;
  gender: string;
  verified: string;
  card_valid_start: string;
  card_valid_end: string;
}

export interface Account {
  bank_code: string;
  account_number: string;
  account_name: string;
  resp_code?: string;
}

export interface Subscription {
  uniq_ref_id: string;
  customer_number: string;
  nw: string;
  amount: string;
  cycle: string;
  reference: string;
  return_url: string;
  resumable: string;
  status: "Pending" | "Active" | "Suspended" | "Cancelled";
  otp: string;
  subscribed_at: string;
  activated_at: string;
  cancelled_at: string;
}

export interface FailureRule {
  id: string;
  operation: string;
  exttrid: string;
  attempt: number;
  action: string;
  http_status?: number;
  resp_code?: string;
  delay_ms?: number;
  hold?: boolean;
  response_body?: string;
  created_at: string;
}

export interface Hold {
  id: string;
  operation: string;
  exttrid: string;
  attempt: number;
  since: string;
}

export interface LedgerEntry {
  id: number;
  account: string;
  direction: "CREDIT" | "DEBIT";
  amount: number;
  balance_after: number;
  exttrid: string;
  note: string;
  created_at: string;
}

export interface Scenario {
  key: string;
  suffix: string;
  description: string;
}

export interface Info {
  base_url: string;
  scenarios: Scenario[];
  card_scenarios: { digit: string; description: string }[];
  bank_codes: { code: string; name: string }[];
  failure_actions: string[];
  endpoints: string[];
}

export interface TryResult {
  request: { method: string; path: string; authorization: string; body: string };
  status: number;
  body: string;
  duration_ms: number;
}
