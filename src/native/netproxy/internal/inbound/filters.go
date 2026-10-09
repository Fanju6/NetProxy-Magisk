package inbound

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/sagernet/sing/common/json/badoption"
)

func validateAppFilters(users []int, includePackage, excludePackage []string) error {
	if len(users) != 0 || len(includePackage) != 0 || len(excludePackage) != 0 {
		return validationError("inbound.app_filter_conflict", "app", "共用应用策略与原生 package/user 筛选冲突，请关闭应用策略或清理原生筛选")
	}
	return nil
}

type uidRange struct{ start, end uint32 }

// 合并区间而不展开 UID，避免大范围名单占用无界内存；uint64 防止最大 UID 加一溢出。
func normalizeUIDFilters(uids []uint32, ranges []string) ([]uint32, []string, error) {
	intervals := make([]uidRange, 0, len(uids)+len(ranges))
	for _, uid := range uids {
		intervals = append(intervals, uidRange{uid, uid})
	}
	for _, value := range ranges {
		start, end, found := strings.Cut(value, ":")
		first, firstErr := strconv.ParseUint(start, 0, 32)
		last, lastErr := strconv.ParseUint(end, 0, 32)
		if !found || firstErr != nil || lastErr != nil || first > last {
			return nil, nil, validationError("inbound.uid_range_invalid", "include_uid_range/exclude_uid_range", "UID 范围必须为有效的 start:end: "+value)
		}
		intervals = append(intervals, uidRange{uint32(first), uint32(last)})
	}
	slices.SortFunc(intervals, func(a, b uidRange) int {
		if result := cmp.Compare(a.start, b.start); result != 0 {
			return result
		}
		return cmp.Compare(a.end, b.end)
	})
	merged := make([]uidRange, 0, len(intervals))
	for _, current := range intervals {
		if len(merged) == 0 || uint64(current.start) > uint64(merged[len(merged)-1].end)+1 {
			merged = append(merged, current)
		} else if current.end > merged[len(merged)-1].end {
			merged[len(merged)-1].end = current.end
		}
	}
	resultUIDs := make([]uint32, 0)
	resultRanges := make([]string, 0)
	for _, interval := range merged {
		if interval.start == interval.end {
			resultUIDs = append(resultUIDs, interval.start)
		} else {
			resultRanges = append(resultRanges, strconv.FormatUint(uint64(interval.start), 10)+":"+strconv.FormatUint(uint64(interval.end), 10))
		}
	}
	return resultUIDs, resultRanges, nil
}

func validateUIDFilters(include []uint32, includeRanges []string, exclude []uint32, excludeRanges []string) error {
	if _, _, err := normalizeUIDFilters(include, includeRanges); err != nil {
		return err
	}
	_, _, err := normalizeUIDFilters(exclude, excludeRanges)
	return err
}

// applyRootPolicy 只覆盖 UID 0；空 include 表示不限 UID，不能为接管 Root 将其变成仅包含 0。
func (c Config) applyRootPolicy(include *badoption.Listable[uint32], includeRanges []string, exclude *badoption.Listable[uint32], excludeRanges *badoption.Listable[string], otherIncludes bool) error {
	switch c.RootPolicy {
	case "exclude":
		*exclude = append(*exclude, 0)
	case "include":
		if len(*include)+len(includeRanges) > 0 || otherIncludes || (c.App.Enabled && c.App.Mode == "whitelist") {
			*include = append(*include, 0)
		}
		uids, intervals, err := normalizeUIDFilters(*exclude, *excludeRanges)
		if err != nil {
			return err
		}
		*exclude = slices.DeleteFunc(uids, func(uid uint32) bool { return uid == 0 })
		*excludeRanges = nil
		for _, interval := range intervals {
			start, end, _ := strings.Cut(interval, ":")
			if start == "0" {
				start = "1"
			}
			*excludeRanges = append(*excludeRanges, start+":"+end)
		}
	}
	return nil
}
