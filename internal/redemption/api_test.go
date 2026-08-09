package redemption

import (
	"errors"
	"testing"

	v1 "spacetime-node/api/proto/spacetime_node/v1"
)

func TestToProtoPreservesRedemptionState(t *testing.T) {
	response := toProto(Redemption{
		ID:                       "redemption-1",
		JourneyID:                "journey-1",
		OfferID:                  "offer-1",
		Status:                   "verified",
		PointsCost:               80,
		MerchantVerificationCode: "verify-1",
	})
	if response.GetRedemptionId() != "redemption-1" || response.GetStatus() != v1.RedemptionStatus_REDEMPTION_STATUS_VERIFIED || response.GetPointsCost() != 80 {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestMapErrorUsesStableReason(t *testing.T) {
	if got := mapError(ErrIdempotencyKeyConflict); !v1.IsIdempotencyKeyConflict(got) {
		t.Fatalf("expected idempotency conflict, got %v", got)
	}
	if got := mapError(ErrVerificationFailed); !v1.IsMerchantVerificationFailed(got) {
		t.Fatalf("expected merchant verification failure, got %v", got)
	}
	if got := mapError(errors.New("database unavailable")); got.Error() != "database unavailable" {
		t.Fatalf("unexpected passthrough error: %v", got)
	}
}
