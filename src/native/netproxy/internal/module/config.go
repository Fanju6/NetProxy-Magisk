package module

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/sagernet/sing-box/option"
)

var (
	configProcessRunning = service.ProcessRunning
	configReload         = reloadAppliedConfig
	configRestoreReload  = reloadConfigSnapshot
	configStop           = StopService
	configStart          = startAppliedConfig
)

// ConfigDocument 是配置工作台可见的文件摘要。
type ConfigDocument struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Category string `json:"category"`
	Editable bool   `json:"editable"`
	Section  string `json:"section,omitempty"`
}

var ErrConfigConflict = errors.New("配置已被修改，请重新加载后再保存")

var configSections = []string{
	"log", "dns", "ntp", "certificate", "certificate_providers",
	"http_clients", "network_namespaces", "endpoints", "inbounds", "outbounds",
	"providers", "route", "services", "experimental",
}

func configSection(target string) string {
	if section, found := strings.CutPrefix(target, "inbound/"); found && (section == "backend" || section == "app" || section == "ebpf" || section == "tun") {
		return section
	}
	section, hasPrefix := strings.CutPrefix(target, "singbox/")
	if hasPrefix && slices.Contains(configSections, section) {
		return section
	}
	return ""
}

// ListConfigs 返回所有可管理的配置文件，不读取运行时 JSON 内容。
func ListConfigs(options Options) ([]ConfigDocument, error) {
	if err := options.validate(); err != nil {
		return nil, err
	}
	result := make([]ConfigDocument, 0)
	result = append(result, ConfigDocument{ID: "inbound", Filename: "inbound.json", Category: "inbound", Editable: true})
	for _, section := range []string{"backend", "app", "ebpf", "tun"} {
		result = append(result, ConfigDocument{ID: "inbound/" + section, Filename: section, Category: "inbound", Editable: true, Section: section})
	}
	if _, err := os.Stat(paths.SingBoxConfig(options.SingBoxDir)); err == nil {
		result = append(result, ConfigDocument{ID: "singbox/config.json", Filename: "config.json", Category: "config", Editable: true})
		for _, section := range configSections {
			result = append(result, ConfigDocument{ID: "singbox/" + section, Filename: section, Category: "config", Editable: true, Section: section})
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	entries, err := os.ReadDir(paths.SingBoxLocalRulesDir(options.SingBoxDir))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		result = append(result, ConfigDocument{
			ID:       "singbox/rules/local/" + entry.Name(),
			Filename: entry.Name(),
			Category: "rules",
			Editable: true,
		})
	}
	for _, name := range []string{"inbound.json", "outbounds.json", "providers.json"} {
		path := filepath.Join(options.RuntimeDir, name)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			continue
		}
		result = append(result, ConfigDocument{
			ID: "runtime/" + name, Filename: name, Category: "runtime", Editable: false,
		})
	}
	return result, nil
}

// ReadConfig 读取一个配置文件并保留原始文本。
func ReadConfig(options Options, target string) (map[string]string, error) {
	path, err := ResolveConfig(options, target)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if section := configSection(target); section != "" {
		object, err := configObject(content)
		if err != nil {
			return nil, err
		}
		content, err = sectionContent(object, section)
		if err != nil {
			return nil, err
		}
	}
	return map[string]string{"target": target, "content": string(content), "revision": configRevision(content)}, nil
}

func configRevision(content []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(content))
}

func configObject(content []byte) (map[string]jsontext.Value, error) {
	var object map[string]jsontext.Value
	if err := json.Unmarshal(content, &object); err != nil {
		return nil, fmt.Errorf("配置必须是有效 JSON 对象: %w", err)
	}
	if object == nil {
		return nil, errors.New("配置必须是 JSON 对象，不能为 null")
	}
	return object, nil
}

func sectionContent(object map[string]jsontext.Value, section string) ([]byte, error) {
	fragment := make(map[string]jsontext.Value)
	if value, exists := object[section]; exists {
		fragment[section] = value
	}
	return json.Marshal(fragment, json.Deterministic(true), jsontext.WithIndent("  "))
}

func prepareConfigEdit(current, replacement []byte, section, expectedRevision string) ([]byte, string, error) {
	var object map[string]jsontext.Value
	var err error
	if section != "" {
		object, err = configObject(current)
		if err != nil {
			return nil, "", err
		}
		if expectedRevision != "" {
			current, err = sectionContent(object, section)
			if err != nil {
				return nil, "", err
			}
		}
	}
	if expectedRevision != "" && configRevision(current) != expectedRevision {
		return nil, "", ErrConfigConflict
	}
	if section == "" {
		return replacement, configRevision(replacement), nil
	}
	fragment, err := configObject(replacement)
	if err != nil {
		return nil, "", err
	}
	for key := range fragment {
		if key != section {
			return nil, "", fmt.Errorf("当前编辑器只允许修改 %s，不能包含 %s", section, key)
		}
	}
	replacement, err = json.Marshal(fragment, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		return nil, "", err
	}
	if value, exists := fragment[section]; exists {
		object[section] = value
	} else {
		delete(object, section)
	}
	merged, err := json.Marshal(object, json.Deterministic(true), jsontext.WithIndent("  "))
	return merged, configRevision(replacement), err
}

// ApplyConfig 通过候选文件、校验和原子替换应用配置。
func ApplyConfig(ctx context.Context, options Options, target, source string, validateOnly bool, expectedRevision string) (revision string, err error) {
	event := "config.apply"
	message := "配置保存"
	if validateOnly {
		event = "config.validate"
		message = "配置校验"
	}
	defer func() { logOperation(options, "config", event, message, false, err) }()
	destination, err := ResolveConfig(options, target)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(target, "runtime/") {
		return "", errors.New("运行时配置只读")
	}
	replacement, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("配置内容文件不存在: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return "", err
	}
	if err := options.validate(); err != nil {
		return "", err
	}
	options, release, err := lockConfigApply(ctx, options, destination, validateOnly)
	if err != nil {
		return "", err
	}
	defer release()
	return applyConfigLocked(ctx, options, target, destination, replacement, validateOnly, expectedRevision)
}

func lockConfigApply(ctx context.Context, options Options, destination string, validateOnly bool) (Options, func(), error) {
	var lifecycle *lifecycleLock
	var err error
	if !validateOnly {
		lifecycle, err = waitLifecycleLock(ctx, options.StateFile)
		if err != nil {
			return options, nil, err
		}
		if err := recoverConfigApply(ctx, options); err != nil {
			lifecycle.release()
			return options, nil, err
		}
	}
	options, release, err := lockConfigFiles(ctx, options, destination, options.ModuleConfig, options.InboundConfig, paths.SingBoxConfig(options.SingBoxDir))
	if err != nil {
		if lifecycle != nil {
			lifecycle.release()
		}
		return options, nil, err
	}
	return options, func() {
		release()
		if lifecycle != nil {
			lifecycle.release()
		}
	}, nil
}

func applyConfigLocked(ctx context.Context, options Options, target, destination string, replacement []byte, validateOnly bool, expectedRevision string) (string, error) {
	// 锁内读取最新主配置后只替换目标分区，不能把客户端的整份旧快照写回。
	section := configSection(target)
	var current []byte
	inboundTarget := target == "inbound" || strings.HasPrefix(target, "inbound/")
	if section != "" || expectedRevision != "" || inboundTarget {
		var err error
		current, err = os.ReadFile(destination)
		if err != nil {
			return "", err
		}
	}
	if inboundTarget && section != "" {
		fragment, err := configObject(replacement)
		if err != nil {
			return "", err
		}
		value, found := fragment[section]
		if !found || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return "", errors.New("入站分区必需，不能删除或设置为 null")
		}
	}
	content, revision, err := prepareConfigEdit(current, replacement, section, expectedRevision)
	if err != nil {
		return "", err
	}
	candidate, err := os.CreateTemp(filepath.Dir(destination), ".config-candidate-")
	if err != nil {
		return "", err
	}
	candidatePath := candidate.Name()
	defer os.Remove(candidatePath)
	if err := candidate.Close(); err != nil {
		return "", err
	}
	if err := os.WriteFile(candidatePath, content, 0o600); err != nil {
		return "", err
	}
	if inboundTarget {
		err = validateInboundTree(ctx, options, candidatePath, content, section)
	} else if section != "" {
		err = validateSingBoxTree(ctx, options, candidatePath)
	} else {
		err = validateConfig(ctx, options, target, candidatePath, content)
	}
	if err != nil {
		return "", err
	}
	if validateOnly {
		return revision, nil
	}
	applyRuntime := true
	switchBackend := false
	if target == "module" {
		previous, previousErr := moduleconfig.LoadModule(destination)
		next, nextErr := moduleconfig.LoadModule(candidatePath)
		if previousErr == nil && nextErr == nil {
			previous.AutoStart = next.AutoStart
			applyRuntime = previous != next
		}
	}
	if inboundTarget {
		previous, parseErr := inbound.Parse(current)
		if parseErr != nil {
			return "", parseErr
		}
		next, parseErr := inbound.Parse(content)
		if parseErr != nil {
			return "", parseErr
		}
		oldEffective, effectiveErr := previous.EffectiveContent()
		if effectiveErr != nil {
			return "", effectiveErr
		}
		newEffective, effectiveErr := next.EffectiveContent()
		if effectiveErr != nil {
			return "", effectiveErr
		}
		applyRuntime = !bytes.Equal(oldEffective, newEffective)
		switchBackend = previous.Backend != next.Backend
		if configProcessRunning(options.SingBoxPath) {
			state, stateErr := ReadServiceState(options.StateFile)
			if stateErr != nil {
				return "", stateErr
			}
			if state.ActiveBackend != "" {
				switchBackend = state.ActiveBackend != next.Backend
				applyRuntime = applyRuntime || switchBackend
			}
		}
	}
	transaction, err := beginConfigApply(options, destination)
	if err != nil {
		return "", err
	}
	if !applyRuntime {
		transaction.journal.Action = "none"
	}
	if switchBackend && transaction.journal.WasRunning {
		transaction.journal.Action = "switch"
	}
	if err := transaction.writeJournal(); err != nil {
		return "", errors.Join(err, transaction.rollback())
	}
	if transaction.journal.Action == "switch" {
		if err := transaction.setPhase("switch_started"); err != nil {
			return "", errors.Join(err, transaction.rollback())
		}
		if err := configStop(ctx, options); err != nil {
			return "", rollbackConfigApply(options, transaction, err)
		}
	}
	if err := os.Rename(candidatePath, destination); err != nil {
		return "", rollbackConfigApply(options, transaction, err)
	}
	if err := transaction.setPhase("static_replaced"); err != nil {
		return "", rollbackConfigApply(options, transaction, fmt.Errorf("记录配置应用阶段失败: %w", err))
	}
	if !transaction.journal.WasRunning || !applyRuntime {
		if err := transaction.commit(); err != nil {
			return "", errors.Join(fmt.Errorf("提交配置事务失败: %w", err), transaction.rollback())
		}
		return revision, nil
	}
	if err := transaction.setPhase("reload_started"); err != nil {
		return "", rollbackConfigApply(options, transaction, fmt.Errorf("记录配置 reload 阶段失败: %w", err))
	}
	apply := configReload
	if transaction.journal.Action == "switch" {
		apply = configStart
	}
	if err := apply(ctx, options); err != nil {
		return "", rollbackConfigApply(options, transaction, fmt.Errorf("配置应用失败: %w", err))
	}
	if target == "module" {
		// reload 可能校正已失效的节点选择，revision 必须对应锁内最终内容。
		applied, err := os.ReadFile(destination)
		if err != nil {
			return "", rollbackAfterCommitFailure(ctx, options, transaction, err)
		}
		revision = configRevision(applied)
	}
	if err := transaction.commit(); err != nil {
		return "", rollbackAfterCommitFailure(ctx, options, transaction, err)
	}
	return revision, nil
}

func rollbackAfterCommitFailure(ctx context.Context, options Options, transaction *configApplyTransaction, commitErr error) error {
	return rollbackConfigApply(options, transaction, fmt.Errorf("提交配置事务失败: %w", commitErr))
}

func validateConfig(ctx context.Context, options Options, target, candidate string, content []byte) error {
	switch target {
	case "module":
		_, err := moduleconfig.LoadModule(candidate)
		return err
	}
	if !jsontext.Value(content).IsValid() {
		return errors.New("配置不是有效 JSON")
	}
	if target == "singbox/config.json" {
		if _, err := configObject(content); err != nil {
			return err
		}
		return validateSingBoxTree(ctx, options, candidate)
	}
	if strings.HasPrefix(target, "singbox/rules/local/") {
		var rules option.PlainRuleSetCompat
		if err := json.Unmarshal(content, &rules); err != nil {
			return fmt.Errorf("规则集格式无效: %w", err)
		}
		check, err := json.Marshal(map[string]any{
			"log": option.LogOptions{Disabled: true},
			"route": map[string]any{"rule_set": []map[string]string{
				{"type": "local", "tag": "netproxy-check", "format": "source", "path": candidate},
			}},
		}, json.Deterministic(true))
		if err != nil {
			return err
		}
		// 独立 check 只加载候选规则，不启动入站、不加载用户其他配置，也不下载远程规则。
		command := exec.CommandContext(ctx, options.SingBoxPath, "check", "-c", "stdin")
		command.Stdin = bytes.NewReader(check)
		if output, err := command.CombinedOutput(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("规则集检查失败: %w: %s", err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func validateInboundTree(ctx context.Context, options Options, candidate string, content []byte, section string) error {
	if section == "backend" || section == "app" {
		if _, err := inbound.Parse(content); err != nil {
			return err
		}
	} else if err := inbound.Validate(content, section); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp("", "netproxy-inbound-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	checkOptions := options
	checkOptions.RuntimeDir = temporary
	checkOptions.InboundConfig = candidate
	prepared, err := Prepare(ctx, checkOptions, true)
	if err != nil {
		return err
	}
	return checkPreparedConfiguration(ctx, checkOptions, prepared)
}

func validateManagedInbound(options Options, config inbound.Config) error {
	content, err := os.ReadFile(paths.SingBoxConfig(options.SingBoxDir))
	if err != nil {
		return err
	}
	object, err := configObject(content)
	if err != nil {
		return err
	}
	var configured []struct {
		Type string `json:"type"`
		Tag  string `json:"tag"`
	}
	if raw, exists := object["inbounds"]; exists {
		if err := json.Unmarshal(raw, &configured); err != nil {
			return err
		}
	}
	for _, entry := range configured {
		if entry.Type == "ebpf" || entry.Type == "tun" || entry.Tag == inbound.Tag {
			return &inbound.ValidationError{Diagnostics: []inbound.Diagnostic{{Level: "error", Code: "inbound.static_conflict", Field: "inbounds", Message: "主配置不能重复定义受管 eBPF/TUN 入站或占用 netproxy-in 标签"}}}
		}
	}
	if config.Backend != "tun" {
		return nil
	}
	var route struct {
		AutoDetectInterface bool          `json:"auto_detect_interface"`
		DefaultInterface    string        `json:"default_interface"`
		DefaultMark         option.FwMark `json:"default_mark"`
	}
	if raw, exists := object["route"]; exists {
		if err := json.Unmarshal(raw, &route); err != nil {
			return err
		}
	}
	if !route.AutoDetectInterface && route.DefaultInterface == "" {
		return &inbound.ValidationError{Diagnostics: []inbound.Diagnostic{{Level: "error", Code: "tun.interface_required", Field: "route", Message: "TUN 需要 route.auto_detect_interface 或 route.default_interface 防止出口回环"}}}
	}
	if route.DefaultMark != 0 {
		return tunRoutingMarkConflict("route.default_mark")
	}
	for _, key := range []string{"outbounds", "endpoints", "http_clients"} {
		if raw, exists := object[key]; exists {
			var entries []jsontext.Value
			if err := json.Unmarshal(raw, &entries); err != nil {
				return err
			}
			for _, entry := range entries {
				if err := validateTUNDialerMark(entry, key+".routing_mark"); err != nil {
					return err
				}
			}
		}
	}
	if raw, exists := object["dns"]; exists {
		var dns struct {
			Servers []jsontext.Value `json:"servers"`
		}
		if err := json.Unmarshal(raw, &dns); err != nil {
			return err
		}
		for _, entry := range dns.Servers {
			if err := validateTUNDialerMark(entry, "dns.servers.routing_mark"); err != nil {
				return err
			}
		}
	}
	if raw, exists := object["ntp"]; exists {
		if err := validateTUNDialerMark(raw, "ntp.routing_mark"); err != nil {
			return err
		}
	}
	return nil
}

func validateTUNDialerMark(content []byte, field string) error {
	var dialer struct {
		RoutingMark option.FwMark `json:"routing_mark"`
	}
	if err := json.Unmarshal(content, &dialer); err != nil {
		return err
	}
	if dialer.RoutingMark != 0 {
		return tunRoutingMarkConflict(field)
	}
	return nil
}

func tunRoutingMarkConflict(field string) error {
	// 上游在首次拨号才执行冲突检查，check 通过不能证明此组合可运行。
	return &inbound.ValidationError{Diagnostics: []inbound.Diagnostic{{Level: "error", Code: "tun.routing_mark_conflict", Field: field, Message: "TUN auto_redirect 不能与 default_mark 或 routing_mark 同时使用"}}}
}

// validateSingBoxTree 在临时配置树中检查候选静态配置，避免直接覆盖用户正在使用的文件。
func validateSingBoxTree(ctx context.Context, options Options, candidate string) error {
	temporary, err := os.MkdirTemp("", "netproxy-config-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	if err := copyDirectory(paths.SingBoxRulesDir(options.SingBoxDir), paths.SingBoxRulesDir(temporary)); err != nil {
		return err
	}
	candidatePath := paths.SingBoxConfig(temporary)
	if err := copyFile(candidatePath, candidate, 0o600); err != nil {
		return err
	}
	checkOptions := options
	checkOptions.RuntimeDir = filepath.Join(temporary, "runtime")
	checkOptions.SingBoxDir = temporary
	prepared, err := Prepare(ctx, checkOptions, true)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, options.SingBoxPath, "check", "-c", candidatePath,
		"-c", prepared.Providers, "-c", prepared.Outbounds, "-c", prepared.Inbound)
	command.Dir = temporary
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("sing-box 配置检查失败: %w", err)
	}
	return nil
}

func copyDirectory(source, destination string) error {
	info, err := os.Stat(source)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("不是配置目录: %s", source)
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		return copyFile(target, path, 0o600)
	})
}

// ResolveConfig 将客户端配置 ID 安全解析为模块内文件。
func ResolveConfig(options Options, target string) (string, error) {
	switch target {
	case "module":
		return options.ModuleConfig, nil
	case "inbound", "inbound/backend", "inbound/app", "inbound/ebpf", "inbound/tun":
		return options.InboundConfig, nil
	case "singbox/config.json":
		return paths.SingBoxConfig(options.SingBoxDir), nil
	}
	if configSection(target) != "" {
		return paths.SingBoxConfig(options.SingBoxDir), nil
	}
	if !strings.HasPrefix(target, "singbox/") && !strings.HasPrefix(target, "runtime/") {
		return "", errors.New("不支持的配置目标")
	}
	root := options.SingBoxDir
	prefix := "singbox/"
	if strings.HasPrefix(target, "runtime/") {
		root = options.RuntimeDir
		prefix = "runtime/"
	}
	relative := filepath.FromSlash(strings.TrimPrefix(target, prefix))
	parts := strings.Split(filepath.ToSlash(relative), "/")
	validLocalRulePath := len(parts) == 3 && parts[0] == "rules" && parts[1] == "local" && filepath.Ext(parts[2]) == ".json" && parts[2] != "" && parts[2][0] != '.'
	if prefix == "singbox/" && !validLocalRulePath {
		return "", errors.New("配置目标路径无效")
	}
	if prefix == "runtime/" && (len(parts) != 1 || filepath.Ext(parts[0]) != ".json" || parts[0] == "" || parts[0][0] == '.') {
		return "", errors.New("配置目标路径无效")
	}
	name := parts[len(parts)-1]
	if prefix == "runtime/" && !isRuntimeConfigName(name) {
		return "", errors.New("不支持的运行时文件")
	}
	for _, char := range name {
		if !(char == '.' || char == '-' || char == '_' || char >= '0' && char <= '9' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z') {
			return "", errors.New("配置文件名无效")
		}
	}
	return filepath.Join(root, relative), nil
}

func isRuntimeConfigName(name string) bool {
	switch name {
	case "providers.json", "outbounds.json", "inbound.json":
		return true
	default:
		return false
	}
}

func copyFile(destination, source string, mode fs.FileMode) error {
	content, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.WriteFile(destination, content, mode); err != nil {
		return err
	}
	return os.Chmod(destination, mode)
}
