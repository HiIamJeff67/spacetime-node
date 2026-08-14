package notification

import (
	"context"
	"errors"
	"strings"

	v1 "spacetime-node/api/proto/spacetime_node/v1"
)

type API struct {
	v1.UnimplementedNotificationServiceServer
	service *Service
}

func NewAPI(service *Service) *API { return &API{service: service} }

func (a *API) RegisterNotificationSubscription(ctx context.Context, request *v1.RegisterNotificationSubscriptionRequest) (*v1.RegisterNotificationSubscriptionResponse, error) {
	if a == nil || a.service == nil || request == nil {
		return nil, v1.ErrorInvalidRequest("subscription payload is required")
	}
	subscription, err := a.service.Register(ctx, request.GetUserIdHash(), request.GetEndpoint(), request.GetP256Dh(), request.GetAuth(), request.GetUserAgent())
	if err != nil {
		return nil, mapError(err)
	}
	return &v1.RegisterNotificationSubscriptionResponse{SubscriptionId: subscription.ID, Active: subscription.Active, Channel: subscription.Channel}, nil
}

func (a *API) RevokeNotificationSubscription(ctx context.Context, request *v1.RevokeNotificationSubscriptionRequest) (*v1.RevokeNotificationSubscriptionResponse, error) {
	if a == nil || a.service == nil || request == nil || strings.TrimSpace(request.GetSubscriptionId()) == "" {
		return nil, v1.ErrorInvalidRequest("subscription_id is required")
	}
	subscription, err := a.service.Revoke(ctx, request.GetUserIdHash(), request.GetSubscriptionId())
	if err != nil {
		return nil, mapError(err)
	}
	return &v1.RevokeNotificationSubscriptionResponse{SubscriptionId: subscription.ID, Active: subscription.Active}, nil
}

func mapError(err error) error {
	switch {
	case errors.Is(err, ErrInvalidSubscription):
		return v1.ErrorInvalidRequest("invalid notification subscription")
	case errors.Is(err, ErrUserNotFound):
		return v1.ErrorInvalidRequest("user is not registered")
	case errors.Is(err, ErrSubscriptionNotFound):
		return v1.ErrorInvalidRequest("subscription not found")
	default:
		return err
	}
}
