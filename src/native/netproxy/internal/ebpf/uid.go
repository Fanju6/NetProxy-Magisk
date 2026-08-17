package ebpf

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ResolvePackageUIDs 使用 Android package service 将用户包名解析为精确 UID。
func ResolvePackageUIDs(refs []PackageRef) ([]uint32, error) {
	if len(refs) == 0 {
		return []uint32{}, nil
	}
	byUser := make(map[uint32][]PackageRef)
	for _, ref := range refs {
		byUser[ref.UserID] = append(byUser[ref.UserID], ref)
	}
	result := make([]uint32, 0, len(refs))
	for userID, userRefs := range byUser {
		packages, err := listPackageUIDs(userID)
		if err != nil {
			return nil, err
		}
		for _, ref := range userRefs {
			uid, ok := packages[ref.Package]
			if !ok {
				return nil, validationError("ebpf.package_uid_not_found", "应用包名", fmt.Sprintf("用户 %d 未找到应用 %s，请刷新应用列表后重试", ref.UserID, ref.Package))
			}
			result = append(result, uid)
		}
	}
	return uniqueUint32(result), nil
}

func listPackageUIDs(userID uint32) (map[string]uint32, error) {
	command := exec.Command("cmd", "package", "list", "packages", "--user", strconv.FormatUint(uint64(userID), 10), "-U")
	output, err := command.Output()
	if err != nil {
		return nil, validationError("ebpf.package_list_failed", "应用包名", fmt.Sprintf("读取 Android 用户 %d 的应用 UID 失败: %v", userID, err))
	}
	packages := make(map[string]uint32)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || !strings.HasPrefix(fields[0], "package:") {
			continue
		}
		packageName := strings.TrimPrefix(fields[0], "package:")
		uidText := strings.TrimPrefix(fields[1], "uid:")
		uid, parseErr := strconv.ParseUint(uidText, 10, 32)
		if parseErr == nil && packageName != "" {
			packages[packageName] = uint32(uid)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, validationError("ebpf.package_list_failed", "应用包名", fmt.Sprintf("解析 Android 用户 %d 的应用 UID 失败: %v", userID, err))
	}
	return packages, nil
}
