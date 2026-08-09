package redemption

import (
	"context"
	"errors"
	"strings"

	"github.com/go-kratos/kratos/v3/transport"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
	"github.com/google/uuid"

	v1 "spacetime-node/api/proto/spacetime_node/v1"
)

type API struct {
	v1.UnimplementedRedemptionServiceServer
	service *Service
}

func NewAPI(service *Service) *API {
	return &API{service: service}
}

func (a *API) CreateRedemption(ctx context.Context, request *v1.CreateRedemptionRequest) (*v1.CreateRedemptionResponse, error) {
	if a == nil || a.service == nil || request == nil || request.GetRequestContext() == nil {
		return nil, v1.ErrorInvalidRequest("request_context with journey_id is required")
	}
	journeyID := request.GetRequestContext().GetJourneyId()
	traceID := request.GetRequestContext().GetTraceId()
	if journeyID == "" {
		return nil, v1.ErrorInvalidRequest("request_context.journey_id is required")
	}
	if traceID == "" {
		traceID = uuid.NewString()
	}
	idempotencyKey := strings.TrimSpace(request.GetIdempotencyKey())
	if idempotencyKey == "" {
		idempotencyKey = requestHeader(ctx, "Idempotency-Key")
	}
	if idempotencyKey == "" {
		return nil, v1.ErrorInvalidRequest("Idempotency-Key is required")
	}
	redemption, err := a.service.Create(ctx, CreateRequest{
		UserIDHash:     request.GetUserIdHash(),
		JourneyID:      journeyID,
		OfferID:        request.GetOfferId(),
		IdempotencyKey: idempotencyKey,
		TraceID:        traceID,
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &v1.CreateRedemptionResponse{Redemption: toProto(redemption)}, nil
}

func (a *API) GetRedemption(ctx context.Context, request *v1.GetRedemptionRequest) (*v1.GetRedemptionResponse, error) {
	if a == nil || a.service == nil || request == nil || request.GetRedemptionId() == "" {
		return nil, v1.ErrorInvalidRequest("redemption_id is required")
	}
	redemption, err := a.service.Get(ctx, request.GetRedemptionId())
	if err != nil {
		return nil, mapError(err)
	}
	return &v1.GetRedemptionResponse{Redemption: toProto(redemption)}, nil
}

func (a *API) VerifyRedemption(ctx context.Context, request *v1.VerifyRedemptionRequest) (*v1.VerifyRedemptionResponse, error) {
	if a == nil || a.service == nil || request == nil || request.GetRedemptionId() == "" || request.GetMerchantId() == "" || request.GetVerificationCode() == "" {
		return nil, v1.ErrorInvalidRequest("redemption_id, merchant_id, and verification_code are required")
	}
	traceID := ""
	if requestContext := request.GetRequestContext(); requestContext != nil {
		traceID = requestContext.GetTraceId()
	}
	if traceID == "" {
		traceID = uuid.NewString()
	}
	redemption, err := a.service.Verify(ctx, VerifyRequest{
		RedemptionID:     request.GetRedemptionId(),
		MerchantID:       request.GetMerchantId(),
		VerificationCode: request.GetVerificationCode(),
		TraceID:          traceID,
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &v1.VerifyRedemptionResponse{Redemption: toProto(redemption)}, nil
}

func requestHeader(ctx context.Context, key string) string {
	if httpContext, ok := ctx.(khttp.Context); ok {
		return strings.TrimSpace(httpContext.Header().Get(key))
	}
	if serverTransport, ok := transport.FromServerContext(ctx); ok {
		return strings.TrimSpace(serverTransport.RequestHeader().Get(key))
	}
	return ""
}

func toProto(redemption Redemption) *v1.Redemption {
	status := v1.RedemptionStatus_REDEMPTION_STATUS_UNSPECIFIED
	switch redemption.Status {
	case "pending":
		status = v1.RedemptionStatus_REDEMPTION_STATUS_PENDING
	case "succeeded":
		status = v1.RedemptionStatus_REDEMPTION_STATUS_SUCCEEDED
	case "rejected":
		status = v1.RedemptionStatus_REDEMPTION_STATUS_REJECTED
	case "verified":
		status = v1.RedemptionStatus_REDEMPTION_STATUS_VERIFIED
	}
	return &v1.Redemption{
		RedemptionId:             redemption.ID,
		JourneyId:                redemption.JourneyID,
		OfferId:                  redemption.OfferID,
		Status:                   status,
		PointsCost:               redemption.PointsCost,
		MerchantVerificationCode: redemption.MerchantVerificationCode,
	}
}

func mapError(err error) error {
	switch {
	case errors.Is(err, ErrInvalidRequest):
		return v1.ErrorInvalidRequest("invalid redemption request")
	case errors.Is(err, ErrJourneyNotFound):
		return v1.ErrorJourneyNotFound("journey not found")
	case errors.Is(err, ErrOfferUnavailable):
		return v1.ErrorOfferUnavailable("offer unavailable")
	case errors.Is(err, ErrInsufficientPoints):
		return v1.ErrorInsufficientPoints("insufficient points")
	case errors.Is(err, ErrIdempotencyKeyConflict):
		return v1.ErrorIdempotencyKeyConflict("idempotency key is already used for another redemption")
	case errors.Is(err, ErrRedemptionNotFound):
		return v1.ErrorRedemptionNotFound("redemption not found")
	case errors.Is(err, ErrVerificationFailed), errors.Is(err, ErrRedemptionNotVerifiable):
		return v1.ErrorMerchantVerificationFailed("redemption cannot be verified")
	case errors.Is(err, ErrUserNotFound):
		return v1.ErrorInvalidRequest("user is not registered")
	default:
		return err
	}
}
