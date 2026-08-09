package journey

import "testing"

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
