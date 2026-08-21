package notification

import (
	"context"
	"errors"
	"testing"
)

func TestConfiguredPushProviderDefaultsToDeterministic(t *testing.T) {
	provider, err := NewConfiguredPushProvider("mock", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	status, err := provider.Send(context.Background(), PushSubscription{ID: "sub-1"}, []byte(`{"title":"demo"}`))
	if err != nil || status != "delivered" {
		t.Fatalf("unexpected mock delivery: %s %v", status, err)
	}
}

func TestConfiguredPushProviderRequiresVAPIDKeys(t *testing.T) {
	_, err := NewConfiguredPushProvider("webpush", "mailto:demo@example.com", "", "private")
	if !errors.Is(err, ErrInvalidPushProvider) {
		t.Fatalf("expected invalid provider error, got %v", err)
	}
}

func TestPushDeliveryErrorDeactivatesGoneSubscription(t *testing.T) {
	err := &PushDeliveryError{StatusCode: 410, Err: errors.New("gone")}
	if !IsInactiveSubscriptionError(err) || PushFailureCode(err) != "http_410" {
		t.Fatalf("unexpected push error classification: inactive=%t code=%s", IsInactiveSubscriptionError(err), PushFailureCode(err))
	}
}
