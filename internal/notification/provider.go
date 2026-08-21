package notification

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

var ErrInvalidPushProvider = errors.New("invalid push provider configuration")

type PushDeliveryError struct {
	StatusCode int
	Err        error
}

func (e *PushDeliveryError) Error() string {
	if e == nil || e.Err == nil {
		return "push delivery failed"
	}
	return e.Err.Error()
}

func (e *PushDeliveryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func IsInactiveSubscriptionError(err error) bool {
	var deliveryErr *PushDeliveryError
	return errors.As(err, &deliveryErr) && (deliveryErr.StatusCode == http.StatusNotFound || deliveryErr.StatusCode == http.StatusGone)
}

func PushFailureCode(err error) string {
	var deliveryErr *PushDeliveryError
	if errors.As(err, &deliveryErr) && deliveryErr.StatusCode > 0 {
		return fmt.Sprintf("http_%d", deliveryErr.StatusCode)
	}
	return "provider_error"
}

type PushSubscription struct {
	ID       string
	Endpoint string
	P256DH   string
	Auth     string
}

type PushProvider interface {
	Send(context.Context, PushSubscription, []byte) (string, error)
}

type deterministicProvider struct{}

func (deterministicProvider) Send(context.Context, PushSubscription, []byte) (string, error) {
	return "delivered", nil
}

type webPushProvider struct {
	subscriber string
	publicKey  string
	privateKey string
	httpClient webpush.HTTPClient
}

func NewConfiguredPushProvider(mode, subscriber, publicKey, privateKey string) (PushProvider, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "mock":
		return deterministicProvider{}, nil
	case "webpush":
		if strings.TrimSpace(subscriber) == "" || strings.TrimSpace(publicKey) == "" || strings.TrimSpace(privateKey) == "" {
			return nil, ErrInvalidPushProvider
		}
		return &webPushProvider{
			subscriber: subscriber,
			publicKey:  publicKey,
			privateKey: privateKey,
			httpClient: &http.Client{Timeout: 10 * time.Second},
		}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported provider %q", ErrInvalidPushProvider, mode)
	}
}

func (p *webPushProvider) Send(ctx context.Context, subscription PushSubscription, payload []byte) (string, error) {
	if p == nil || subscription.Endpoint == "" || subscription.P256DH == "" || subscription.Auth == "" {
		return "", ErrInvalidPushProvider
	}
	response, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: subscription.Endpoint,
		Keys:     webpush.Keys{P256dh: subscription.P256DH, Auth: subscription.Auth},
	}, &webpush.Options{
		HTTPClient:      p.httpClient,
		Subscriber:      p.subscriber,
		VAPIDPublicKey:  p.publicKey,
		VAPIDPrivateKey: p.privateKey,
		TTL:             300,
		Urgency:         webpush.UrgencyNormal,
	})
	if err != nil {
		return "", fmt.Errorf("web push send: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", &PushDeliveryError{
			StatusCode: response.StatusCode,
			Err:        fmt.Errorf("web push service returned %s", response.Status),
		}
	}
	return "sent", nil
}
