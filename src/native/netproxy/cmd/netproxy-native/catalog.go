package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

func runCatalog(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 Catalog 操作")
	}
	action := args[0]
	flags := newFlagSet("catalog " + action)
	input := flags.String("input", "", "输入路径或内容")
	value := flags.String("value", "", "元数据字段值")
	root := flags.String("root", "", "Catalog 根目录")
	moduleConfig := flags.String("module-config", "", "module.conf 路径")
	groupDir := flags.String("group-dir", "", "Catalog 分组目录")
	active := flags.String("active", "", "活动分组 ID")
	progressDir := flags.String("progress-dir", "", "订阅更新进度目录")
	groupType := flags.String("type", "all", "分组类型筛选")
	kind := flags.String("kind", "subscription", "分组 ID 类型")
	groupID := flags.String("group", "", "指定分组 ID")
	name := flags.String("name", "", "分组显示名称")
	tag := flags.String("tag", "", "节点标签")
	allowInsecure := flags.Bool("allow-insecure", false, "跳过节点 TLS 证书校验")
	subscriptionURL := flags.String("url", "", "订阅地址")
	userAgent := flags.String("user-agent", "", "订阅 User-Agent")
	hwid := flags.String("hwid", "", "订阅 HWID")
	headersFile := flags.String("headers-file", "", "自定义请求头 JSON 文件")
	autoUpdate := flags.Bool("auto-update", true, "是否自动更新")
	updateInterval := flags.Int64("update-interval", 86400, "更新间隔秒数")
	intervalSource := flags.String("interval-source", "default", "更新间隔来源")
	updateViaProxy := flags.String("update-via-proxy", "auto", "订阅更新代理模式")
	include := flags.String("include", "", "节点包含表达式")
	exclude := flags.String("exclude", "", "节点排除表达式")
	timeout := flags.Int64("timeout", 60, "订阅请求超时秒数")
	providersOutput := flags.String("providers-output", "", "运行时 Provider 配置输出")
	outboundsOutput := flags.String("outbounds-output", "", "运行时出站配置输出")
	stateOutput := flags.String("state-output", "", "运行时状态输出")
	selector := flags.String("selector", "urltest", "选择模式")
	selected := flags.String("selected", "", "手动节点引用")
	allowEmpty := flags.Bool("allow-empty", false, "允许空 Catalog")
	now := flags.Int64("now", time.Now().Unix(), "当前 Unix 时间")
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if action == "duration" {
		seconds, err := subscription.DurationToSeconds(*value)
		if err != nil {
			return err
		}
		if *format == "raw" {
			fmt.Println(seconds)
			return nil
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.duration", Message: "更新周期已解析", Data: map[string]int64{"seconds": seconds}})
		return nil
	}
	if action == "time" {
		if *format == "raw" {
			fmt.Println(subscription.FormatEpochUTC(*now))
			return nil
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.time", Message: "时间已格式化", Data: map[string]string{"value": subscription.FormatEpochUTC(*now)}})
		return nil
	}
	if action == "schedule-next" {
		interval, err := subscription.DurationToSeconds(*value)
		if err != nil {
			return err
		}
		metadata := subscription.Metadata{UpdateInterval: interval}
		subscription.ScheduleAt(&metadata, time.Unix(*now, 0))
		if *format == "tsv" {
			fmt.Printf("interval\t%d\nnext_update_epoch\t%d\nnext_update_at\t%s\n", metadata.UpdateInterval, metadata.NextUpdateEpoch, metadata.NextUpdateAt)
			return nil
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.schedule_next", Message: "下一次更新时间已计算", Data: map[string]any{"interval": metadata.UpdateInterval, "next_update_epoch": metadata.NextUpdateEpoch, "next_update_at": metadata.NextUpdateAt}})
		return nil
	}
	if action == "new-id" {
		if *root == "" {
			return errors.New("Catalog new-id 需要 --root")
		}
		id, err := catalog.NewGroupID(*root, *kind, *input)
		if err != nil {
			return err
		}
		if *format == "raw" {
			fmt.Println(id)
			return nil
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.new_id", Message: "Catalog 分组 ID 已生成", Data: map[string]string{"group_id": id}})
		return nil
	}
	if action == "resolve" {
		if *root == "" || *groupID == "" {
			return errors.New("Catalog resolve 需要 --root 和 --group")
		}
		resolved, err := catalog.ResolveGroup(*root, *groupID)
		if err != nil {
			return err
		}
		if *format == "raw" {
			fmt.Println(resolved)
			return nil
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.resolve", Message: "Catalog 分组已解析", Data: map[string]string{"group_id": resolved}})
		return nil
	}
	if action == "group-has-nodes" || action == "group-first-tag" || action == "group-contains-tag" || action == "first-nonempty" || action == "group-type" || action == "node-get" || action == "node-export" || action == "group-private" || action == "group-delete" || action == "history" {
		if action == "first-nonempty" {
			if *root == "" {
				return errors.New("Catalog first-nonempty 需要 --root")
			}
			group, err := catalog.FirstNonEmptyGroup(ctx, *root, *groupID)
			if err != nil {
				return err
			}
			if *format == "raw" {
				fmt.Println(group)
				return nil
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.first_nonempty", Message: "首个非空分组", Data: map[string]string{"group_id": group}})
			return nil
		}
		if *root == "" || *groupID == "" {
			return fmt.Errorf("Catalog %s 需要 --root 和 --group", action)
		}
		switch action {
		case "group-has-nodes":
			hasNodes, err := catalog.GroupHasNodes(ctx, *root, *groupID)
			if err != nil {
				return err
			}
			if *format == "raw" {
				fmt.Println(hasNodes)
				return nil
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.group_has_nodes", Message: "Catalog 分组节点状态", Data: map[string]bool{"has_nodes": hasNodes}})
		case "group-type":
			groupType, err := catalog.GroupType(*root, *groupID)
			if err != nil {
				return err
			}
			if *format == "raw" {
				fmt.Println(groupType)
				return nil
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.group_type", Message: "Catalog 分组类型", Data: map[string]string{"type": groupType}})
		case "group-first-tag":
			firstTag, err := catalog.GroupFirstTag(ctx, *root, *groupID)
			if err != nil {
				return err
			}
			if *format == "raw" {
				fmt.Println(firstTag)
				return nil
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.group_first_tag", Message: "Catalog 分组首个节点", Data: map[string]string{"tag": firstTag}})
		case "group-contains-tag":
			if *tag == "" {
				return errors.New("Catalog group-contains-tag 需要 --tag")
			}
			contains, err := catalog.GroupContainsTag(ctx, *root, *groupID, *tag)
			if err != nil {
				return err
			}
			if *format == "raw" {
				fmt.Println(contains)
				return nil
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.group_contains_tag", Message: "Catalog 分组节点标签状态", Data: map[string]bool{"contains": contains}})
		case "node-get":
			if *tag == "" {
				return errors.New("Catalog node-get 需要 --tag")
			}
			document, err := catalog.GroupNode(ctx, *root, *groupID, *tag)
			if err != nil {
				return err
			}
			content, err := provider.Marshal(ctx, document)
			if err != nil {
				return err
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.loaded", Message: "节点配置已读取", Data: json.RawMessage(content)})
		case "node-export":
			if *tag == "" {
				return errors.New("Catalog node-export 需要 --tag")
			}
			exported, err := catalog.ExportGroupNode(ctx, *root, *groupID, *tag)
			if err != nil {
				return err
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.exported", Message: "节点分享链接已生成", Data: exported})
		case "group-private":
			metadata, err := catalog.PrivateMetadata(*root, *groupID)
			if err != nil {
				return err
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.group_private", Message: "Catalog 分组私有信息", Data: metadata})
		case "group-delete":
			if err := catalog.DeleteGroup(*root, *groupID); err != nil {
				return err
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.group_deleted", Message: "Catalog 分组已删除", Data: map[string]string{"group_id": *groupID}})
		case "history":
			history, err := subscription.LoadHistory(filepath.Join(*root, *groupID, "history.jsonl"))
			if err != nil {
				return err
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.history", Message: "Catalog 分组更新历史", Data: history})
		}
		return nil
	}
	if action == "group-init" || action == "group-ensure" {
		if *root == "" || *groupID == "" {
			return fmt.Errorf("Catalog %s 需要 --root 和 --group", action)
		}
		groupType := *groupType
		if groupType == "all" {
			if action == "group-ensure" {
				groupType = "local"
			} else {
				return errors.New("Catalog group-init 需要 --type")
			}
		}
		headers, err := readHeadersFile(*headersFile)
		if err != nil {
			return err
		}
		options := catalog.GroupOptions{
			Root: *root, GroupID: *groupID, Name: *name, Type: groupType,
			URL: *subscriptionURL, UserAgent: *userAgent, HWID: *hwid,
			CustomHeaders: headers, AutoUpdate: *autoUpdate, UpdateInterval: *updateInterval,
			IntervalSource: *intervalSource, UpdateViaProxy: *updateViaProxy,
			Include: *include, Exclude: *exclude, AllowInsecure: *allowInsecure,
			Timeout: *timeout, Now: time.Unix(*now, 0),
		}
		if action == "group-init" {
			err = catalog.InitializeGroup(ctx, options)
		} else {
			err = catalog.EnsureGroup(ctx, options)
		}
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog." + action, Message: "Catalog 分组已准备", Data: map[string]string{"group_id": *groupID}})
		return nil
	}
	if action == "group-name" {
		if *root == "" || *groupID == "" || strings.TrimSpace(*name) == "" {
			return errors.New("Catalog group-name 需要 --root、--group 和 --name")
		}
		if err := catalog.SetGroupName(ctx, *root, *groupID, *name, time.Unix(*now, 0)); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.group_name", Message: "Catalog 分组名称已更新", Data: map[string]string{"group_id": *groupID, "name": strings.TrimSpace(*name)}})
		return nil
	}
	if *root == "" && *groupDir == "" {
		return errors.New("Catalog 操作需要 --root")
	}

	switch action {
	case "group-import":
		if *root == "" || *groupID == "" || *input == "" {
			return errors.New("Catalog group-import 需要 --root、--group 和 --input")
		}
		mutation, err := catalog.ImportGroup(ctx, catalog.ImportOptions{
			Root: *root, GroupID: *groupID, Name: *name, Input: *input,
			AllowInsecure: *allowInsecure, Now: time.Unix(*now, 0),
		})
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.group_imported", Message: "Catalog 分组已导入", Data: mutation})
		return nil
	case "node-append", "node-remove", "node-edit":
		if *groupDir == "" {
			return fmt.Errorf("Catalog %s 需要 --group-dir", action)
		}
		mutationType := *groupType
		if mutationType == "all" {
			mutationType = "local"
		}
		options := catalog.MutationOptions{
			GroupDir: *groupDir, GroupID: *groupID, Name: *name, Type: mutationType,
			Input: *input, Tag: *tag, AllowInsecure: *allowInsecure, Now: time.Unix(*now, 0),
		}
		var mutation catalog.MutationResult
		var err error
		switch action {
		case "node-append":
			mutation, err = catalog.AppendNode(ctx, options)
		case "node-remove":
			mutation, err = catalog.RemoveNode(ctx, options)
		case "node-edit":
			mutation, err = catalog.EditNode(ctx, options)
		}
		if err != nil {
			return err
		}
		if *format == "kv" {
			fmt.Printf("group_id=%s\nnode_count=%d\nrevision=%d\nstructure_changed=%t\n", mutation.GroupID, mutation.NodeCount, mutation.Revision, mutation.StructureChanged)
			return nil
		}
		code := "catalog.node_updated"
		message := "Catalog 节点已更新"
		if action == "node-append" {
			code = "catalog.node_appended"
			message = "Catalog 节点已加入"
		} else if action == "node-remove" {
			code = "catalog.node_removed"
			message = "Catalog 节点已移除"
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: code, Message: message, Data: mutation})
		return nil
	case "groups", "snapshot", "group", "show":
		if (action == "group" || action == "show") && *groupID == "" {
			return fmt.Errorf("Catalog %s 需要 --group", action)
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
			Type: *groupType, WithNodes: action == "snapshot" || action == "show", GroupID: *groupID,
		})
		if err != nil {
			return err
		}
		if action == "group" || action == "show" {
			if len(groups) == 0 {
				return fmt.Errorf("Catalog 分组不存在: %s", *groupID)
			}
			data := any(groups[0])
			if action == "group" {
				data = groups[0].Group
			}
			if *format == "tsv" {
				group := groups[0].Group
				fmt.Printf("id\t%s\nname\t%s\nruntime_tag\t%s\ntype\t%s\nnode_count\t%d\nrevision\t%d\nactive\t%t\n", group.ID, group.Name, group.RuntimeTag, group.Type, group.NodeCount, group.Revision, group.Active)
				return nil
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog." + action, Message: "Catalog 分组快照", Data: data})
		} else if action == "groups" {
			summaries := make([]catalog.GroupSummary, 0, len(groups))
			for _, group := range groups {
				summaries = append(summaries, group.Group)
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.groups", Message: "Catalog 分组快照", Data: summaries})
		} else {
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.snapshot", Message: "Catalog 节点快照", Data: groups})
		}
		return nil
	case "runtime":
		data, err := catalog.BuildRuntime(ctx, catalog.RuntimeOptions{
			Root: *root, ModuleConfig: *moduleConfig, ProvidersOutput: *providersOutput, OutboundsOutput: *outboundsOutput, StateOutput: *stateOutput,
			ActiveGroup: *active, SelectorMode: *selector, SelectedNodeRef: *selected,
			AllowEmpty: *allowEmpty,
		})
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.runtime", Message: "Catalog 运行时配置已生成", Data: data})
		return nil
	case "schedule":
		data, err := catalog.Schedule(*root, *now)
		if err != nil {
			return err
		}
		if *format == "tsv" {
			fmt.Printf("nearest\t%d\n", data.Nearest)
			for _, group := range data.Due {
				fmt.Printf("due\t%s\n", group)
			}
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("Catalog schedule 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.schedule", Message: "订阅调度快照", Data: data})
		return nil
	case "tag":
		if *groupID == "" {
			return errors.New("Catalog tag 需要 --group")
		}
		tag, err := catalog.RuntimeTag(*root, *groupID)
		if err != nil {
			return err
		}
		if *format == "raw" {
			fmt.Println(tag)
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("Catalog tag 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.tag", Message: "Catalog 运行时标签", Data: map[string]string{"tag": tag}})
		return nil
	case "ids":
		ids, err := catalog.GroupIDs(*root, *groupType)
		if err != nil {
			return err
		}
		if *format == "raw" {
			for _, id := range ids {
				fmt.Println(id)
			}
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("Catalog ids 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "catalog.ids", Message: "Catalog 分组 ID", Data: ids})
		return nil
	default:
		return fmt.Errorf("未知 Catalog 操作 %q", action)
	}
}
