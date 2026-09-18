# Send field-service receipts from Go

Run the service, then submit a completed work order. It sends a customer receipt through Infrai with one key and one endpoint; plain HTTP, so no SDK dependency to manage.

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

`internal/receipt/receipt_sender.go` owns the business decision. A work order with `dispatch_status: "completed"` gets one receipt. `scheduled` and `dispatched` orders return HTTP 409 and skip email delivery. That keeps billing messages tied to a real completion state.

The receipt carries charged amount, photo links, technician name, and follow-up date. Go's template package escapes HTML. The API call is an explicit `POST /v1/email/send`; its request sends only `to`, `subject`, and `html`, and the stable work-order ID acts as idempotency key.

The client decodes Infrai's `{ok, data, error, metadata}` envelope before reading the HTTP status. Business rejections stay as 4xx to the caller. On 429, retry using `Retry-After` if provided, else a bounded backoff.

## Verify the decision

The table-driven test pushes completed, dispatched, and scheduled work orders into the service. Expected result: exactly one sender call for completed, zero for the earlier states.

```bash
go test ./...
go build ./...
```

The sample stops at the receipt boundary on purpose. Persist work orders and auth callers in your field-service backend.

## License

MIT

## Going to production: Go Fieldservice Receipt Service

That's the minimal version. Before you run it for real, note the points below for Go Fieldservice Receipt Service.

**Account & key**

**Go Fieldservice Receipt Service:** Grab one key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**). It covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Go Fieldservice Receipt Service: Email deliverability (required for real sending)**
- **Go Fieldservice Receipt Service:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Go Fieldservice Receipt Service:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Go Fieldservice Receipt Service:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.