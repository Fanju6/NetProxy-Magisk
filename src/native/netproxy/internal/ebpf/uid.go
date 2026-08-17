package ebpf

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// PackageUIDResolution 描述包名解析结果以及当前设备上不存在的配置项。
type PackageUIDResolution struct {
	UIDs    []uint32
	Missing []PackageRef
}

// ResolvePackageUIDs 使用 Android package service 将用户包名解析为精确 UID。
func ResolvePackageUIDs(refs []PackageRef) (PackageUIDResolution, error) {
	return resolvePackageUIDs(refs, listPackageUIDs)
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

func listPackageUIDs(userID uint32) (map[string]uint32, error) {
	command := exec.Command("cmd", "package", "list", "packages", "--user", strconv.FormatUint(uint64(userID), 10), "-U")
	var stderr strings.Builder
	command.Stderr = &stderr
	output, err := command.Output()
	return parsePackageUIDCommandResult(userID, string(output), stderr.String(), err)
}

func parsePackageUIDCommandResult(userID uint32, output, stderr string, commandErr error) (map[string]uint32, error) {
	if commandErr != nil {
		message := fmt.Sprintf("读取 Android 用户 %d 的应用 UID 失败: %v", userID, commandErr)
		if detail := strings.TrimSpace(stderr); detail != "" {
			message += ": " + detail
		}
		return nil, validationError("ebpf.package_list_failed", "应用包名", message)
	}
	if detail := strings.TrimSpace(stderr); detail != "" {
		return nil, validationError("ebpf.package_list_failed", "应用包名", fmt.Sprintf("读取 Android 用户 %d 的应用 UID 失败: %s", userID, detail))
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
			return nil, validationError("ebpf.package_list_invalid", "应用包名", "Android package service 返回了无法识别的应用 UID 数据")
		}
		packageName := strings.TrimPrefix(fields[0], "package:")
		uidText := strings.TrimPrefix(fields[1], "uid:")
		uid, parseErr := strconv.ParseUint(uidText, 10, 32)
		if parseErr != nil || packageName == "" {
			return nil, validationError("ebpf.package_list_invalid", "应用包名", "Android package service 返回了无效的应用 UID")
		}
		packages[packageName] = uint32(uid)
	}
	if err := scanner.Err(); err != nil {
		return nil, validationError("ebpf.package_list_failed", "应用包名", fmt.Sprintf("解析 Android package service 应用 UID 失败: %v", err))
	}
	return packages, nil
}
