package update

import "testing"

func TestUpdateNeverDowngrades(t *testing.T) {
	for _, v := range []struct {
		current, next string
		want          bool
	}{{"v1.2.3", "v1.2.4", true}, {"v1.2.3", "v1.2.2", false}, {"v1.2.3", "v1.2.3", false}, {"0.1.0-dev", "v0.1.0", true}, {"v1.2.3", "v1.3.0", true}, {"v1.2.3", "invalid", false}} {
		if got := IsNewer(v.current, v.next); got != v.want {
			t.Fatalf("%+v: %v", v, got)
		}
	}
}
