package bot

import "testing"

// The IDs configured in k8s/secrets.yaml.
var configuredUsers = []int64{888397843, 685751256}

func TestAllowListAdmitsExactlyTheConfiguredUsers(t *testing.T) {
	b := &Bot{allowed: allowList(configuredUsers)}

	for _, id := range configuredUsers {
		if !b.allows(id) {
			t.Errorf("allows(%d) = false, want true for a configured user", id)
		}
	}

	// A neighbouring ID must not slip through, which is what a mis-split
	// ALLOWED_USERS value would look like.
	for _, id := range []int64{0, 8883978, 685751257, 88839784} {
		if b.allows(id) {
			t.Errorf("allows(%d) = true, want false for an unlisted user", id)
		}
	}
}

func TestEmptyAllowListMakesTheBotPublic(t *testing.T) {
	b := &Bot{allowed: allowList(nil)}

	if !b.allows(1) {
		t.Error("allows(1) = false, want true when no allow list is configured")
	}
}
