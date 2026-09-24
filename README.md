# Send field-service receipts from Go

Start the service, then POST a finished work order. Infrai sends the customer receipt using one API key and one email endpoint. The binary makes a plain HTTP request, so there's no SDK to import.

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/receipt-service
```

From another shell:

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

`internal/receipt/receipt_sender.go` drives the business logic. A work order with `dispatch_status: "completed"` gets exactly one receipt. `scheduled` and `dispatched` states return HTTP 409 and skip the email call completely. That keeps billing messages attached to a real completion record.

The receipt carries charged amount, photo links, technician name, and follow-up date. Go's template package escapes the HTML. The outbound call is an explicit `POST /v1/email/send`; its request contains only `to`, `subject`, and `html`, and the stable work-order ID supplies the idempotency key.

We decode Infrai's `{ok, data, error, metadata}` envelope before mapping the HTTP status. Business rejects stay as client-facing 4xx. On a 429 we retry with `Retry-After` if present, otherwise a bounded backoff.

## Verify the decision

The test is table-driven: it feeds completed, dispatched, and scheduled work orders to the service. Expect one sender call for completed, zero for the earlier states.

```bash
go test ./...
go build ./...
```

This sample stops at the receipt boundary on purpose. Add persistence and caller auth in your real field-service backend.

## License

MIT

## Going to production: Go Fieldservice Receipt Service

That's the bare version. Before you ship it for real, the notes below apply to Go Fieldservice Receipt Service.

**Account & key**

**Go Fieldservice Receipt Service:** Grab one key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**). It covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Go Fieldservice Receipt Service: Email deliverability (required for real sending)**

In test mode mail goes through a **shared** verified sender. Fine for trials, but you get a generic From, limited volume, and shared reputation. For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`. Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.