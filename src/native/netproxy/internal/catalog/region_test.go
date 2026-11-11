package catalog

import "testing"

func TestRegionIncludeRegexMatchesRuntimeProviderNames(t *testing.T) {
	pattern := RegionIncludeRegex("示例分组", []string{"🇯🇵日本东京A1", "🇯🇵日本东京A2"})
	if pattern == nil {
		t.Fatal("expected region include regex")
	}
	for _, tag := range []string{"示例分组/🇯🇵日本东京A1", "示例分组/🇯🇵日本东京A2"} {
		if !pattern.MatchString(tag) {
			t.Fatalf("region regex does not match runtime tag %q: %s", tag, pattern)
		}
	}
	for _, tag := range []string{"🇯🇵日本东京A1", "示例分组/🇺🇸美国A1"} {
		if pattern.MatchString(tag) {
			t.Fatalf("region regex matched unrelated tag %q: %s", tag, pattern)
		}
	}
}
