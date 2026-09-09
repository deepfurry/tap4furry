package auth

import "testing"

func TestStaticRoleCapabilities(t *testing.T) {
	for _, test := range []struct {
		roles []Role
		want  [4]bool
	}{
		{nil, [4]bool{}}, {[]Role{"user", "future"}, [4]bool{}},
		{[]Role{Moderator}, [4]bool{true, true, false, false}},
		{[]Role{Editor}, [4]bool{true, false, true, false}},
		{[]Role{Administrator}, [4]bool{true, true, true, true}},
		{[]Role{Moderator, Editor}, [4]bool{true, true, true, false}},
	} {
		for i, capability := range []Capability{AdminAccess, Moderation, Editorial, Administration} {
			if HasCapability(test.roles, capability) != test.want[i] {
				t.Fatal("static capability matrix failed")
			}
		}
		if HasCapability(test.roles, "future") {
			t.Fatal("unknown capability allowed")
		}
	}
}
