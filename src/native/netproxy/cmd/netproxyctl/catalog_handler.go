package main

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

func runCatalog(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 Catalog 操作")
	}
	action := args[0]
	flags := newFlagSet("catalog " + action)
	moduleDir := flags.String("module-dir", defaultModuleDir(), "模块根目录")
	root := flags.String("root", "", "Catalog 根目录")
	moduleConfig := flags.String("module-config", "", "module.conf 路径")
	active := flags.String("active", "", "活动分组 ID")
	progressDir := flags.String("progress-dir", "", "订阅更新进度目录")
	groupType := flags.String("type", "all", "分组类型筛选")
	groupID := flags.String("group", "", "指定分组 ID")
	tag := flags.String("tag", "", "节点标签")
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	layout := paths.New(*moduleDir)
	if strings.TrimSpace(*root) == "" {
		*root = layout.Catalog()
	}
	if strings.TrimSpace(*moduleConfig) == "" {
		candidate := layout.ModuleConfig()
		if _, err := os.Stat(candidate); err == nil {
			*moduleConfig = candidate
		}
	}
	if strings.TrimSpace(*progressDir) == "" {
		*progressDir = defaultProgressDir()
	}

	switch action {
	case "node-get":
		if *groupID == "" || *tag == "" {
			return errors.New("Catalog node-get 需要 --group 和 --tag")
		}
		document, err := catalog.GroupNode(ctx, *root, *groupID, *tag)
		if err != nil {
			return err
		}
		content, err := provider.Marshal(ctx, document)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.loaded", Message: "节点配置已读取", Data: jsontext.Value(content)})
		return nil
	case "node-export":
		if *groupID == "" || *tag == "" {
			return errors.New("Catalog node-export 需要 --group 和 --tag")
		}
		exported, err := catalog.ExportGroupNode(ctx, *root, *groupID, *tag)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.exported", Message: "节点分享链接已生成", Data: exported})
		return nil
	case "groups", "show":
		if action == "show" && *groupID == "" {
			return errors.New("Catalog show 需要 --group")
		}
		activeGroup := *active
		if activeGroup == "" && *moduleConfig != "" {
			module, err := moduleconfig.LoadModule(*moduleConfig)
			if err != nil {
				return err
			}
			activeGroup = module.ActiveGroupID
		}
		groups, err := catalog.Scan(ctx, catalog.ScanOptions{
			Root: *root, ActiveGroup: activeGroup, ProgressDir: *progressDir,
			Type: *groupType, WithNodes: action == "show", GroupID: *groupID,
		})
		if err != nil {
			return err
		}
		if action == "show" {
			if len(groups) == 0 {
				return fmt.Errorf("Catalog 分组不存在: %s", *groupID)
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.show", Message: "Catalog 分组快照", Data: groups[0]})
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("Catalog groups 不支持输出格式 %q", *format)
		}
		summaries := make([]catalog.GroupSummary, 0, len(groups))
		for _, group := range groups {
			summaries = append(summaries, group.Group)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.groups", Message: "Catalog 分组快照", Data: summaries})
		return nil
	default:
		return fmt.Errorf("未知 Catalog 操作 %q", action)
	}
}
