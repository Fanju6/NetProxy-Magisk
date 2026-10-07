package inbound

import (
	"reflect"
	"testing"
)

func TestUIDRangesNormalizeWithoutExpansion(t *testing.T) {
	for _, test := range []struct {
		name       string
		uids       []uint32
		ranges     []string
		wantUIDs   []uint32
		wantRanges []string
	}{
		{"deduplicate", []uint32{3, 1, 1, 3}, nil, []uint32{1, 3}, []string{}},
		{"overlap", []uint32{3, 8, 9}, []string{"1:4", "4:7"}, []uint32{}, []string{"1:9"}},
		{"singleton", nil, []string{"10:10", "0x1:0x3", "3:5"}, []uint32{10}, []string{"1:5"}},
		{"full", []uint32{0, ^uint32(0)}, []string{"0:4294967295"}, []uint32{}, []string{"0:4294967295"}},
		{"overflow-boundary", []uint32{0, ^uint32(0)}, []string{"4294967294:4294967295"}, []uint32{0}, []string{"4294967294:4294967295"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			uids, ranges, err := normalizeUIDFilters(test.uids, test.ranges)
			if err != nil || !reflect.DeepEqual(uids, test.wantUIDs) || !reflect.DeepEqual(ranges, test.wantRanges) {
				t.Fatalf("%v %v %v", uids, ranges, err)
			}
		})
	}
	for _, invalid := range []string{"", "1", ":1", "1:", "2:1", "-1:2", "0:4294967296", "0:1:2", "first:last"} {
		if _, _, err := normalizeUIDFilters(nil, []string{invalid}); err == nil {
			t.Fatalf("接受非法 UID 范围: %q", invalid)
		}
	}
}

func TestPackageRefStrictAndCanonical(t *testing.T) {
	for _, value := range []string{"0:com.example.app", "10:com.example.app", "000:android", "4294967295:com.example.app"} {
		ref, err := ParsePackageRef(value)
		if err != nil {
			t.Fatal(err)
		}
		if again, err := ParsePackageRef(ref.String()); err != nil || again != ref {
			t.Fatal(ref, again, err)
		}
	}
	for _, value := range []string{"", "com.example.app", ":com.example.app", "0:", "-1:com.example.app", "+1:com.example.app", "4294967296:com.example.app", "0:com.example.app:other", "0:com.example.app,10:com.example.app", "0:com.example.app，10:com.example.app", " 0:com.example.app", "0:com.example.app ", "0:com.example/$app"} {
		if _, err := ParsePackageRef(value); err == nil {
			t.Fatalf("接受非法引用: %q", value)
		}
	}
}
