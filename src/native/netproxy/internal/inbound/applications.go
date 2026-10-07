package inbound

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

type PackageRef struct {
	UserID  uint32
	Package string
}

type PackageUIDResolver func([]PackageRef) (PackageUIDResolution, error)

func ParsePackageRef(value string) (PackageRef, error) {
	user, name, found := strings.Cut(value, ":")
	if !found || user == "" || !validPackageName(name) {
		return PackageRef{}, validationError("inbound.package_invalid", "app", "应用必须使用 <用户ID>:<包名> 格式")
	}
	for _, char := range user {
		if char < '0' || char > '9' {
			return PackageRef{}, validationError("inbound.android_user_invalid", "app", "应用用户 ID 必须是非负十进制整数")
		}
	}
	parsed, err := strconv.ParseUint(user, 10, 32)
	if err != nil {
		return PackageRef{}, validationError("inbound.android_user_invalid", "app", "应用用户 ID 必须是 0 到 4294967295 的整数")
	}
	return PackageRef{UserID: uint32(parsed), Package: name}, nil
}

func (r PackageRef) String() string {
	return strconv.FormatUint(uint64(r.UserID), 10) + ":" + r.Package
}

func validPackageName(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if !(char == '.' || char == '_' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}

func packageRefs(values []string) ([]PackageRef, error) {
	refs := make([]PackageRef, 0, len(values))
	seen := make(map[PackageRef]bool)
	for _, value := range values {
		ref, err := ParsePackageRef(value)
		if err != nil {
			return nil, err
		}
		if !seen[ref] {
			seen[ref] = true
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

func uniqueUint32(values []uint32) []uint32 {
	values = slices.Clone(values)
	slices.Sort(values)
	return slices.Compact(values)
}

// PackageUIDResolution 描述包名解析结果以及当前设备上不存在的配置项。
type PackageUIDResolution struct {
	UIDs    []uint32
	Missing []PackageRef
}

// ResolvePackageUIDs 使用 Android package service 将用户包名解析为精确 UID。
func ResolvePackageUIDs(ctx context.Context, refs []PackageRef) (PackageUIDResolution, error) {
	if err := ctx.Err(); err != nil {
		return PackageUIDResolution{}, err
	}
	return resolvePackageUIDs(refs, func(user uint32) (map[string]uint32, error) { return listPackageUIDs(ctx, user) })
}

func resolvePackageUIDs(refs []PackageRef, list func(uint32) (map[string]uint32, error)) (PackageUIDResolution, error) {
	if len(refs) == 0 {
		return PackageUIDResolution{UIDs: []uint32{}}, nil
	}
	packagesByUser := make(map[uint32]map[string]uint32)
	for _, ref := range refs {
		if _, ok := packagesByUser[ref.UserID]; ok {
			continue
		}
		packages, err := list(ref.UserID)
		if err != nil {
			return PackageUIDResolution{}, err
		}
		packagesByUser[ref.UserID] = packages
	}
	result := make([]uint32, 0, len(refs))
	missing := make([]PackageRef, 0)
	for _, ref := range refs {
		uid, ok := packagesByUser[ref.UserID][ref.Package]
		if !ok {
			missing = append(missing, ref)
			continue
		}
		result = append(result, uid)
	}
	return PackageUIDResolution{UIDs: uniqueUint32(result), Missing: missing}, nil
}

func listPackageUIDs(ctx context.Context, userID uint32) (map[string]uint32, error) {
	command := exec.CommandContext(ctx, "cmd", "package", "list", "packages", "--user", strconv.FormatUint(uint64(userID), 10), "-U")
	var stderr strings.Builder
	command.Stderr = &stderr
	output, err := command.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return parsePackageUIDCommandResult(userID, string(output), stderr.String(), err)
}

func parsePackageUIDCommandResult(userID uint32, output, stderr string, commandErr error) (map[string]uint32, error) {
	if commandErr != nil {
		message := fmt.Sprintf("读取 Android 用户 %d 的应用 UID 失败: %v", userID, commandErr)
		if detail := strings.TrimSpace(stderr); detail != "" {
			message += ": " + detail
		}
		return nil, validationError("inbound.package_list_failed", "app", message)
	}
	if detail := strings.TrimSpace(stderr); detail != "" {
		return nil, validationError("inbound.package_list_failed", "app", fmt.Sprintf("读取 Android 用户 %d 的应用 UID 失败: %s", userID, detail))
	}
	return parsePackageUIDs(output)
}

func parsePackageUIDs(output string) (map[string]uint32, error) {
	packages := make(map[string]uint32)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "package:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[1], "uid:") {
			return nil, validationError("inbound.package_list_invalid", "app", "Android package service 返回了无法识别的应用 UID 数据")
		}
		packageName := strings.TrimPrefix(fields[0], "package:")
		uidText := strings.TrimPrefix(fields[1], "uid:")
		uid, parseErr := strconv.ParseUint(uidText, 10, 32)
		if parseErr != nil || !validPackageName(packageName) {
			return nil, validationError("inbound.package_list_invalid", "app", "Android package service 返回了无效的应用 UID")
		}
		if previous, exists := packages[packageName]; exists && previous != uint32(uid) {
			return nil, validationError("inbound.package_list_invalid", "app", "Android package service 返回了冲突的应用 UID")
		}
		packages[packageName] = uint32(uid)
	}
	if err := scanner.Err(); err != nil {
		return nil, validationError("inbound.package_list_failed", "app", fmt.Sprintf("解析 Android package service 应用 UID 失败: %v", err))
	}
	return packages, nil
}
