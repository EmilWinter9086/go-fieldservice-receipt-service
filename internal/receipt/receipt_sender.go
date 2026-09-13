package receipt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const emailSendPath = "/v1/email/send"

type Photo struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type Technician struct {
	Name       string `json:"name"`
	FollowUpBy string `json:"follow_up_by"`
}

type WorkOrder struct {
	WorkOrderID    string     `json:"work_order_id"`
	CustomerEmail  string     `json:"customer_email"`
	DispatchStatus string     `json:"dispatch_status"`
	AmountCents    int64      `json:"amount_cents"`
	Currency       string     `json:"currency"`
	Photos         []Photo    `json:"photos"`
	Technician     Technician `json:"technician"`
}

type SendResult struct {
	MessageID string `json:"message_id"`
}

var ErrNotCompleted = errors.New("receipt requires a completed work order")

type Sender interface {
	Send(ctx context.Context, order WorkOrder) (SendResult, error)
}

type Service struct {
	sender Sender
}

func NewService(sender Sender) *Service {
	return &Service{sender: sender}
}

func (s *Service) SendReceipt(ctx context.Context, order WorkOrder) (SendResult, error) {
	if order.DispatchStatus != "completed" {
		return SendResult{}, ErrNotCompleted
	}
	if order.WorkOrderID == "" || order.CustomerEmail == "" || order.Currency == "" || order.AmountCents < 0 {
		return SendResult{}, errors.New("work_order_id, customer_email, currency, and a non-negative amount_cents are required")
	}
	return s.sender.Send(ctx, order)
}

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type InfraiSender struct {
	baseURL string
	apiKey  string
	client  *http.Client
	wait    func(context.Context, time.Duration) error
}

func NewInfraiSender(baseURL, apiKey string, client *http.Client) *InfraiSender {
	return &InfraiSender{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  client,
		wait: func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

type emailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

func (s *InfraiSender) Send(ctx context.Context, order WorkOrder) (SendResult, error) {
	body, err := renderReceipt(order)
	if err != nil {
		return SendResult{}, err
	}
	payload, err := json.Marshal(emailRequest{
		To:      order.CustomerEmail,
		Subject: "Receipt for work order " + order.WorkOrderID,
		HTML:    body,
	})
	if err != nil {
		return SendResult{}, err
	}

	for attempt := 0; attempt < 4; attempt++ {
		result, retryAfter, err := s.sendOnce(ctx, payload, order.WorkOrderID, attempt)
		if retryAfter == 0 {
			return result, err
		}
		if err := s.wait(ctx, retryAfter); err != nil {
			return SendResult{}, err
		}
	}
	return SendResult{}, errors.New("email request retry limit reached")
}

func (s *InfraiSender) sendOnce(ctx context.Context, payload []byte, workOrderID string, attempt int) (SendResult, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+emailSendPath, bytes.NewReader(payload))
	if err != nil {
		return SendResult{}, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "work-order-receipt-"+workOrderID)

	res, err := s.client.Do(req)
	if err != nil {
		return SendResult{}, 0, err
	}
	defer res.Body.Close()

	var env envelope
	decoder := json.NewDecoder(io.LimitReader(res.Body, 1<<20))
	if err := decoder.Decode(&env); err != nil {
		return SendResult{}, 0, fmt.Errorf("decode email response: %w", err)
	}
	if !env.OK {
		apiErr := &APIError{HTTPStatus: res.StatusCode, Message: "email request rejected"}
		if env.Error != nil {
			apiErr.Code = env.Error.Code
			apiErr.Message = env.Error.Message
			if apiErr.Message == "" {
				apiErr.Message = env.Error.Hint
			}
		}
		if res.StatusCode == http.StatusTooManyRequests {
			return SendResult{}, retryDelay(res.Header.Get("Retry-After"), time.Second*time.Duration(1<<attempt)), apiErr
		}
		return SendResult{}, 0, apiErr
	}
	if res.StatusCode >= http.StatusInternalServerError {
		return SendResult{}, 0, fmt.Errorf("email transport status %d", res.StatusCode)
	}

	var result SendResult
	if err := json.Unmarshal(env.Data, &result); err != nil {
		return SendResult{}, 0, fmt.Errorf("decode email data: %w", err)
	}
	if result.MessageID == "" {
		return SendResult{}, 0, errors.New("email response omitted message_id")
	}
	return result, 0, nil
}

func retryDelay(value string, fallback time.Duration) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}

var receiptTemplate = template.Must(template.New("receipt").Funcs(template.FuncMap{
	"money": func(cents int64, currency string) string {
		return fmt.Sprintf("%s %.2f", strings.ToUpper(currency), float64(cents)/100)
	},
}).Parse(`<!doctype html>
<html><body>
<h1>Work order {{.WorkOrderID}} completed</h1>
<p>Amount: {{money .AmountCents .Currency}}</p>
<p>Technician: {{.Technician.Name}}</p>
<p>Follow-up by: {{.Technician.FollowUpBy}}</p>
{{if .Photos}}<h2>Work photos</h2><ul>{{range .Photos}}<li><a href="{{.URL}}">{{.Label}}</a></li>{{end}}</ul>{{end}}
</body></html>`))

func renderReceipt(order WorkOrder) (string, error) {
	var out bytes.Buffer
	if err := receiptTemplate.Execute(&out, order); err != nil {
		return "", err
	}
	return out.String(), nil
}
