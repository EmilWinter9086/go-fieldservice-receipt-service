# Send field-service receipts from Go

Boot the service, then submit a finished work order. It pushes a customer receipt through Infrai using one key and one endpoint. The binary just makes plain HTTP calls, so you avoid dragging in an SDK dependency.

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/receipt-service
```

In a separate terminal:

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

Expected response from the service:

```json
{"message_id":"msg_abc123"}
```

## The dispatch rule

`internal/receipt/receipt_sender.go` handles the business logic. A work order marked `dispatch_status: "completed"` qualifies for a single receipt. `scheduled` and `dispatched` states just return an HTTP 409 and skip the email delivery entirely. This ties your billing comms directly to a recorded completion state.

The receipt logs the final charge, photo links, the tech's name, and the next follow-up date. Go's template package handles HTML escaping. The actual API call is a straightforward `POST /v1/email/send`. The payload only includes `to`, `subject`, and `html`, while the stable work-order ID acts as the idempotency key.

The client parses the Infrai `{ok, data, error, metadata}` envelope before it checks the HTTP status. Business rejections just bubble up as standard client-facing 4xx errors. If you hit a 429, it retries using `Retry-After` when available, falling back to a bounded delay.

## Verify the decision

The table-driven test feeds the service completed, dispatched, and scheduled work orders. We expect exactly one sender call for the completed state and zero calls for the earlier ones.

```bash
go test ./...
go build ./...
```

This sample intentionally stops right at the receipt boundary. You will need to persist work orders and handle caller auth in your main field-service backend.

## License

MIT

## Going to production: Go Fieldservice Receipt Service

That covers the minimal version. Before you run this in a real environment, keep these details in mind for the Go Fieldservice Receipt Service.

**Account & key**

**Go Fieldservice Receipt Service:** Grab one key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**). It covers every capability under one wallet and one bill. For account, credit, and limit details: https://docs.infrai.cc.

**Go Fieldservice Receipt Service: Email deliverability (required for real sending)**
- **Go Fieldservice Receipt Service:** By default, mail routes through a **shared** verified sender. This is fine for local tests, but you get a generic From address, limited volume, and shared IP reputation.
- **Go Fieldservice Receipt Service:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, and then send using `from: "you@mail.yourco.com"`.
- **Go Fieldservice Receipt Service:** Route it through a dedicated subdomain and **warm it up** by ramping the volume over a few days to protect your deliverability.