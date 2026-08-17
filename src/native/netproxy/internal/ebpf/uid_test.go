package ebpf

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParsePackageUIDsAcceptsPackageRowsAndIgnoresOtherOutput(t *testing.T) {
	got, err := parsePackageUIDs(strings.Join([]string{
		"Packages:",
		"package:com.example.first uid:10123",
		"package:com.example.second uid:10124 versionCode:2",
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]uint32{
		"com.example.first":  10123,
		"com.example.second": 10124,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed package UIDs = %#v, want %#v", got, want)
	}
}

func TestParsePackageUIDsRejectsMalformedPackageRows(t *testing.T) {
	for _, output := range []string{
		"package:com.example.app",
		"package:com.example.app uid:not-a-number",
		"package: uid:10123",
		"package:com.example.app uid=10123",
	} {
		_, err := parsePackageUIDs(output)
		var validation *ValidationError
		if !errors.As(err, &validation) || len(validation.Diagnostics) != 1 || validation.Diagnostics[0].Code != "ebpf.package_list_invalid" {
			t.Fatalf("malformed package output %q returned unexpected error: %v", output, err)
		}
	}
}

func TestResolvePackageUIDsPreservesMissingOrderAndCachesUsers(t *testing.T) {
	refs := []PackageRef{
		{UserID: 10, Package: "com.example.missing-ten"},
		{UserID: 0, Package: "com.example.installed-zero"},
		{UserID: 10, Package: "com.example.installed-ten"},
		{UserID: 0, Package: "com.example.missing-zero"},
	}
	lists := map[uint32]map[string]uint32{
		0:  {"com.example.installed-zero": 10123},
		10: {"com.example.installed-ten": 20123},
	}
	calls := make(map[uint32]int)
	resolution, err := resolvePackageUIDs(refs, func(userID uint32) (map[string]uint32, error) {
		calls[userID]++
		return lists[userID], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resolution.UIDs, []uint32{10123, 20123}) {
		t.Fatalf("resolved UID order = %#v", resolution.UIDs)
	}
	wantMissing := []PackageRef{refs[0], refs[3]}
	if !reflect.DeepEqual(resolution.Missing, wantMissing) {
		t.Fatalf("missing package order = %#v, want %#v", resolution.Missing, wantMissing)
	}
	if !reflect.DeepEqual(calls, map[uint32]int{0: 1, 10: 1}) {
		t.Fatalf("package list calls = %#v", calls)
	}
}

func TestResolvePackageUIDsPropagatesPackageListFailure(t *testing.T) {
	expected := errors.New("package service unavailable")
	_, err := resolvePackageUIDs([]PackageRef{{UserID: 0, Package: "com.example.app"}}, func(uint32) (map[string]uint32, error) {
		return nil, expected
	})
	if !errors.Is(err, expected) {
		t.Fatalf("package list error was not propagated: %v", err)
	}
}

func TestParsePackageUIDCommandResultTreatsStderrAsFailure(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		stderr      string
		commandErr  error
		wantCode    string
		wantMessage string
	}{
		{
			name:        "successful command with error stderr",
			stderr:      "Error: unknown user",
			wantCode:    "ebpf.package_list_failed",
			wantMessage: "Error: unknown user",
		},
		{
			name:        "non zero command",
			stderr:      "permission denied",
			commandErr:  errors.New("exit status 1"),
			wantCode:    "ebpf.package_list_failed",
			wantMessage: "exit status 1: permission denied",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parsePackageUIDCommandResult(10, test.output, test.stderr, test.commandErr)
			var validation *ValidationError
			if !errors.As(err, &validation) || len(validation.Diagnostics) != 1 {
				t.Fatalf("unexpected package list error: %v", err)
			}
			diagnostic := validation.Diagnostics[0]
			if diagnostic.Code != test.wantCode || !strings.Contains(diagnostic.Message, test.wantMessage) {
				t.Fatalf("unexpected package list diagnostic: %+v", diagnostic)
			}
		})
	}

	packages, err := parsePackageUIDCommandResult(10, "package:com.example.app uid:10123\n", "", nil)
	if err != nil || !reflect.DeepEqual(packages, map[string]uint32{"com.example.app": 10123}) {
		t.Fatalf("valid package list result failed: packages=%#v err=%v", packages, err)
	}
}
