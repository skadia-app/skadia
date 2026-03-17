package routing

import (
	"testing"
)

func TestGlobalNodeID(t *testing.T) {
	gid := MakeGlobalNodeID(49508, 12345)
	tid, lid := SplitGlobalNodeID(gid)
	if tid != 49508 || lid != 12345 {
		t.Errorf("split(%d) = %d, %d; want 49508, 12345", gid, tid, lid)
	}
}

func TestGlobalNodeID_Zero(t *testing.T) {
	gid := MakeGlobalNodeID(0, 0)
	tid, lid := SplitGlobalNodeID(gid)
	if tid != 0 || lid != 0 {
		t.Errorf("split(0) = %d, %d; want 0, 0", tid, lid)
	}
}

func TestGlobalNodeID_MaxLocal(t *testing.T) {
	gid := MakeGlobalNodeID(49508, 0xFFFFFFFF)
	tid, lid := SplitGlobalNodeID(gid)
	if tid != 49508 || lid != 0xFFFFFFFF {
		t.Errorf("max local: %d, %d", tid, lid)
	}
}
