package user

import (
	"testing"

	v1 "spacetime-node/api/proto/spacetime_node/v1"
)

func TestValidUserIDHash(t *testing.T) {
	valid := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if !validUserIDHash(valid) {
		t.Fatal("expected valid user hash")
	}
	for _, invalid := range []string{"", "sha256:short", "sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"} {
		if validUserIDHash(invalid) {
			t.Fatalf("expected invalid user hash: %q", invalid)
		}
	}
}

func TestValidatePreferences(t *testing.T) {
	valid := &v1.UpdateUserPreferencesRequest{
		UserIdHash:             "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		FavoriteStationIds:     []string{"R04"},
		PreferredCategories:    []string{"coffee"},
		BudgetMinPoints:        80,
		BudgetMaxPoints:        300,
		Timezone:               "Asia/Taipei",
		NotificationsEnabled:   true,
		NotificationStartLocal: "07:00",
		NotificationEndLocal:   "10:00",
	}
	if err := validatePreferences(valid); err != nil {
		t.Fatal(err)
	}
	invalidBudget := &v1.UpdateUserPreferencesRequest{
		BudgetMinPoints: 80,
		BudgetMaxPoints: 79,
		Timezone:        "Asia/Taipei",
	}
	if err := validatePreferences(invalidBudget); err == nil {
		t.Fatal("expected invalid budget range")
	}
	invalidWindow := &v1.UpdateUserPreferencesRequest{
		Timezone:               "Asia/Taipei",
		NotificationsEnabled:   true,
		NotificationStartLocal: "07:00",
	}
	if err := validatePreferences(invalidWindow); err == nil {
		t.Fatal("expected incomplete notification window")
	}
}
