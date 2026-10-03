# Reference

Details behind the [README](../README.md). The Orchard request and response formats come from the [official documentation](https://docs.anmgw.com/docs-page.html).

- [Orchard endpoints](#orchard-endpoints)
- [Payment lifecycle and callbacks](#payment-lifecycle-and-callbacks)
- [Admin API](#admin-api)
- [Failure rules](#failure-rules)
- [Response codes](#response-codes)

## Orchard endpoints

All endpoints take a JSON `POST`, answer HTTP 200, and report the outcome in `resp_code`. Every request needs `service_id`, and the headers below when signatures are required.

```http
Content-Type: application/json
Authorization: <client_key>:<hex HMAC-SHA256 of the exact body, keyed with secret_key>
```

| Endpoint | `trans_type` / `operation` | What the sandbox does |
|---|---|---|
| `/sendRequest` | `CTM` collect from a wallet or card | `015`, then settles and calls back. Credits collections on success. |
| | `MTC` pay out to a wallet or bank (`nw: BNK` + `bank_code` + `recipient_name`) | `015`, reserves the payout balance at once, refunds it if the payout fails. `038` when funds are short. |
| | `ATP` airtime, `BLP` bill payment (`account_number`) | Like `MTC`, drawing on the airtime (`064`) or bill pay (`086`) wallet. Airtime minimum is GHS 0.20. |
| | `RMT` remittance | Like `MTC`. Sender and recipient fields are required (`026`). |
| | `AUD` debit an auto-debit customer | Like `CTM`, but only for a customer with an active mandate (`044` otherwise). |
| | `AII` account name lookup (`nw: BNK`, `bank_code`) | `027` with `name`. Registered accounts return their name or code; others get a stable generated name. |
| | `CTM` with `landing_page` (GHIPSS) | `000` with `form_url` and `form_details`. Posting the form opens the card page. |
| `/checkTransaction` | `TSC` | `trans_status`, `trans_ref`, `trans_id`, `message`. Pending payments report `015`. |
| `/check_wallet_balance` | `BLC` | All six wallet balances in GHS (`sms_bal` in units). |
| `/sendSms` | `SMS` | `082`. Charges one unit per 160 characters (`076` when empty). |
| `/verifyID` | `AII`, `id_type: GCA` | `027` with identity `data`. Registered cards return their profile; others follow the last-digit rules. |
| `/third_party_request` | `payment_mode: CRD, MOM or CRM` | `000` with `redirect_url` to a hosted checkout page. The customer's choice settles the payment, calls back, and redirects to `landing_page`. |
| `/autoDebit` | `SUB`, `OTP`, `OTR`, `SUS`, `RES`, `CAN`, `STA` | Full mandate lifecycle. `SUB` texts an OTP to the customer (see Messages); `OTP` activates and confirms on `return_url`. |

References (`exttrid`, `unique_id`, `uniq_ref_id`) must be unique per sandbox: reuse returns `021`. Requests rejected by validation do not use up the reference. `ts` must be `YYYY-MM-DD HH:MM:SS` (UTC) and is required on `/sendRequest`, `/check_wallet_balance` and `/third_party_request`.

Any other `POST` path returns `051 Unknown request`.

## Payment lifecycle and callbacks

```
POST /sendRequest ──► 015 PENDING ──(settle delay or manual resolve)──► 000/01 SUCCESSFUL ─┐
                                                                       001/02 FAILED ─────┴─► POST callback_url
```

The callback body has exactly Orchard's four fields:

```json
{ "trans_id": "21870173572", "trans_ref": "ORDER-1001", "trans_status": "000/01", "message": "SUCCESS" }
```

A delivery succeeds on any 2xx. Otherwise it is retried after 2 s, 4 s, 8 s … (capped at one minute) until `callback_attempts` is reached. Every attempt is recorded and can be re-sent from the dashboard.

Settlement mode:

- `auto` (default): payments settle `settle_delay_ms` after they are accepted. The customer number picks the outcome (see README).
- `manual`: payments stay pending until you resolve them from the dashboard or with `POST /mock/runs/{run}/transactions/{exttrid}/resolve`.

Hosted checkout and GHIPSS payments always wait for the customer on the checkout page.

## Admin API

Everything the dashboard does is available over HTTP under `/mock`. Paths take a sandbox ID (`default` is always present). Errors return `{"error": "..."}` with a 4xx status.

| Method and path | Purpose |
|---|---|
| `GET /mock/runs` | List sandboxes |
| `POST /mock/runs` | Create a sandbox: `{"run_id", "name", "client_key", "secret_key", "service_id", "settings"}`, all optional. Missing keys are generated. |
| `GET` / `DELETE /mock/runs/{run}` | Read or delete a sandbox |
| `POST /mock/runs/{run}/reset` | Clear activity and restore opening balances |
| `GET /mock/runs/{run}/overview` | Sandbox, balances and counts |
| `PUT /mock/runs/{run}/settings` | Update any of `require_auth`, `settle_mode`, `settle_delay_ms`, `latency_ms`, `callback_attempts`, `callback_timeout_ms` |
| `GET` / `PUT /mock/runs/{run}/balances` | Read or set balances, e.g. `{"payout_bal": 0}` |
| `GET /mock/runs/{run}/ledger` | Every balance movement |
| `GET /mock/runs/{run}/transactions[/{exttrid}]` | Payments |
| `POST /mock/runs/{run}/transactions/{exttrid}/resolve` | `{"status": "SUCCESSFUL" or "FAILED", "message": "..."}` |
| `GET /mock/runs/{run}/requests?ref=` | Request log, optionally for one reference |
| `GET /mock/runs/{run}/callbacks` | Callback deliveries |
| `GET /mock/runs/{run}/callbacks/{id}/attempts` | Delivery attempts |
| `POST /mock/runs/{run}/callbacks/{id}/retry` | Send a callback again |
| `GET /mock/runs/{run}/sms` | Messages |
| `GET` / `POST /mock/runs/{run}/cards`, `DELETE …/cards/{id_num}` | Ghana Card profiles |
| `GET` / `POST /mock/runs/{run}/accounts`, `DELETE …/accounts/{bank_code}/{number}` | Account inquiry profiles (`resp_code` other than `027` makes the lookup fail) |
| `GET /mock/runs/{run}/subscriptions` | Auto debit mandates, including the pending OTP |
| `POST /mock/runs/{run}/subscriptions/{ref}/debit` | Run one billing cycle now |
| `GET` / `POST /mock/runs/{run}/failures`, `DELETE …/failures/{id}` | Failure rules |
| `GET /mock/runs/{run}/holds`, `POST …/holds/{id}/release` | Requests paused by a hold rule |
| `POST /mock/runs/{run}/try` | `{"path": "/sendRequest", "body": {...}}`: sign with the sandbox's keys and send |
| `GET /mock/events?run=` | Server-sent events whenever data changes |

Requests reach a sandbox through its client key. When signatures are off, send `X-Sandbox-Run: <run_id>` instead; without either, requests go to `default`.

## Failure rules

A rule matches on `operation` (`*`, an endpoint such as `verifyID`, `sendRequest`, or `sendRequest:CTM`), `exttrid` (`*` or one reference) and `attempt` (`0` for all, `1` for the first try, and so on). Rules run after authentication; the first match wins.

| `action` | Extra fields | Effect |
|---|---|---|
| `http_status` | `http_status`, `response_body` | Non-200 response (429 adds `Retry-After`) |
| `resp_code` | `resp_code`, `response_body` | HTTP 200 with that Orchard code, e.g. `100` IP not whitelisted |
| `delayed_response` | `delay_ms` or `hold: true` | Wait, then answer normally. Held requests wait until released. |
| `connection_failure` | | Close the connection without a response |
| `connection_close_after_accept` | | Process the request, then close the connection |
| `malformed_json`, `missing_fields`, `wrong_types` | | Broken response bodies |
| `oversized_response` | | A 1 MB body |
| `redirect` | | `302` to an unexpected location |

## Response codes

The sandbox returns these codes from Orchard's published table.

| Code | Meaning |
|---|---|
| `000`, `000/01` | Passed / payment successful |
| `001/02` | Payment failed (callback and status check) |
| `015` | Accepted, pending |
| `027` | Completed (AII, verifyID) |
| `082` / `083` | SMS queued / OTP resent |
| `101` / `102` / `103` | No `Authorization` header / unknown client key / bad signature |
| `006` / `011` | Missing / wrong `service_id` |
| `008`, `009`, `010`, `014`, `018`, `026`, `039`, `041`, `053`, `069`, `077`, `079`, `081`, `088` | Missing or invalid required field |
| `021` | Duplicate reference |
| `022` / `023` | Invalid JSON / invalid `ts` |
| `025` / `085` / `087` | Reference too long (20 / 30 / 40 characters) |
| `028` / `046` / `072` | Invalid amount / more than two decimals / amount too low |
| `029` / `090` / `091` | Network not allowed / unknown network / unknown bank code |
| `033` / `067` | Transaction not found / no record |
| `038` / `064` / `076` / `086` | Insufficient payout / airtime / SMS / bill pay balance |
| `044` | No active auto debit mandate for the customer |
| `045` | `callback_url` is not an absolute http(s) URL |
| `051` | Unknown endpoint |
| `078` | `sender_id` longer than 9 characters |
| `089` | Unknown `trans_type` or operation |
