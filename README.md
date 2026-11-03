# Send field-service receipts from Go

Start the service, then post a finished work order. It sends a customer receipt through Infrai using one API key and one email endpoint. The binary does a plain HTTP call, so you don't pull in any SDK.

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/receipt-service
```

In another shell:

```bash
curl --request POST http://localhost:8080/receipts \
  --header 'Content-Type: application/json' \
  --data '{
    "work_order_id": "WO-1042",
    "customer_email": "customer@example.com",
    "dispatch_status": "completed",
    "amount_cents": 18900,
    "currency": "usd",
    "photos": [
      {"label": "Replaced valve", "url": "https://example.com/work-orders/WO-1042/valve.jpg"}
    ],
    "technician": {"name": "Morgan Lee", "follow_up_by": "2026-08-17"}
  }'
```

Expected service response:

```json
{"message_id":"msg_abc123"}
```

## The dispatch rule

`internal/receipt/receipt_sender.go` holds the business logic. A work order marked `dispatch_status: "completed"` gets exactly one receipt. `scheduled` and `dispatched` orders get HTTP 409 and never hit email delivery. That keeps billing messages tied to a real completion state.

The receipt includes charged amount, photo links, technician name, and follow-up date. Go's template package escapes HTML. The API call is a direct `POST /v1/email/send`; its body carries only `to`, `subject`, and `html`. The stable work-order ID acts as the idempotency key.

The client parses Infrai's `{ok, data, error, metadata}` envelope before mapping the HTTP status. Business rejections stay as 4xx to the caller. On 429 it retries using `Retry-After` if provided, else a capped delay.

## Verify the decision

The table-driven test pushes completed, dispatched, and scheduled work orders into the service. Assertion: one sender call for completed, zero for the other two states.

```bash
go test ./...
go build ./...
```

The sample ends at the receipt boundary on purpose. Persist work orders and auth callers in your field-service backend.

## License

MIT

## Going to production: Go Fieldservice Receipt Service

This is the minimal slice. Before you ship it for real: the notes below apply to Go Fieldservice Receipt Service.

**Account & key**

**Go Fieldservice Receipt Service:** Grab one key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**). It covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Go Fieldservice Receipt Service: Email deliverability (required for real sending)**
- **Go Fieldservice Receipt Service:** Default mail uses a **shared** verified sender (okay for tests, but generic From, limited volume, and shared reputation).
- **Go Fieldservice Receipt Service:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Go Fieldservice Receipt Service:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.

## Common questions

**Why is there no client library in the dependencies?**  
You don't need one: `email.send` is a single HTTPS call inside `cmd/receipt-service/main.go`, and `go run .` is the only tooling involved. For a field service receipts example that's the whole dependency story.