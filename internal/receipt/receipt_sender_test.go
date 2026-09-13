package receipt

import (
	"context"
	"errors"
	"testing"
)

type recordingSender struct {
	calls  int
	result SendResult
}

func (s *recordingSender) Send(_ context.Context, _ WorkOrder) (SendResult, error) {
	s.calls++
	return s.result, nil
}

func TestServiceDispatchDecision(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		wantCalls int
		wantErr   error
	}{
		{name: "completed sends receipt", status: "completed", wantCalls: 1},
		{name: "dispatched waits for completion", status: "dispatched", wantErr: ErrNotCompleted},
		{name: "scheduled waits for completion", status: "scheduled", wantErr: ErrNotCompleted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &recordingSender{result: SendResult{MessageID: "msg_test_42"}}
			service := NewService(sender)
			result, err := service.SendReceipt(context.Background(), WorkOrder{
				WorkOrderID: "WO-1042", CustomerEmail: "customer@example.com",
				DispatchStatus: tt.status, AmountCents: 18900, Currency: "usd",
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if sender.calls != tt.wantCalls {
				t.Fatalf("sender calls = %d, want %d", sender.calls, tt.wantCalls)
			}
			if tt.wantCalls == 1 && result.MessageID != "msg_test_42" {
				t.Fatalf("message_id = %q", result.MessageID)
			}
		})
	}
}
