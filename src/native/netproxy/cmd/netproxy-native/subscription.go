package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

func runSubscription(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少订阅操作: update")
	}
	if args[0] == "edit" {
		return runSubscriptionEdit(ctx, args[1:])
	}
	if args[0] != "update" {
		return fmt.Errorf("未知订阅操作 %q", args[0])
	}
	flags := newFlagSet("subscription update")
	root := flags.String("root", "", "Catalog 根目录")
	groupID := flags.String("group", "", "订阅分组 ID")
	progressDir := flags.String("progress-dir", "", "订阅进度目录")
	proxyURL := flags.String("proxy", "", "通过 HTTP 代理下载")
	fallbackDirect := flags.Bool("fallback-direct", false, "代理失败后回退直连")
	now := flags.Int64("now", time.Now().Unix(), "当前 Unix 时间")
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*root) == "" || strings.TrimSpace(*groupID) == "" {
		return errors.New("subscription update 需要 --root 和 --group")
	}
	updated, err := subscription.Update(ctx, subscription.UpdateOptions{
		Root:               *root,
		GroupID:            *groupID,
		ProgressDir:        *progressDir,
		ProxyURL:           *proxyURL,
		UseConfiguredProxy: true,
		FallbackDirect:     *fallbackDirect,
		Now:                time.Unix(*now, 0),
	})
	if err != nil {
		var structured *subscription.Error
		if errors.As(err, &structured) {
			return &resultError{Code: structured.Code, Message: structured.Message, Data: structured.Data}
		}
		return err
	}
	code := "subscription.updated"
	message := "订阅更新完成"
	if updated.NotModified {
		if *format == "kv" {
			fmt.Printf("group_id=%s\nnode_count=%d\nrevision=%d\nnot_modified=true\nstructure_changed=false\n", updated.GroupID, updated.NodeCount, updated.Revision)
			return nil
		}
		code = "subscription.not_modified"
		message = "订阅未发生变化"
	}
	if *format == "kv" {
		fmt.Printf("group_id=%s\nnode_count=%d\nrevision=%d\nnot_modified=%t\nstructure_changed=%t\n", updated.GroupID, updated.NodeCount, updated.Revision, updated.NotModified, updated.StructureChanged)
		return nil
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: code, Message: message, Data: updated})
	return nil
}

func runSubscriptionEdit(ctx context.Context, args []string) error {
	flags := newFlagSet("subscription edit")
	root := flags.String("root", "", "Catalog 根目录")
	groupID := flags.String("group", "", "订阅分组 ID")
	progressDir := flags.String("progress-dir", "", "订阅更新进度目录")
	proxyURL := flags.String("proxy", "", "通过 HTTP 代理下载")
	fallbackDirect := flags.Bool("fallback-direct", false, "代理失败后回退直连")
	name := flags.String("name", "", "订阅名称")
	subscriptionURL := flags.String("url", "", "订阅地址")
	userAgent := flags.String("user-agent", "", "订阅 User-Agent")
	hwid := flags.String("hwid", "", "订阅 HWID")
	headersFile := flags.String("headers-file", "", "自定义请求头 JSON 文件")
	autoUpdate := flags.Bool("auto-update", false, "自动更新开关")
	interval := flags.String("interval", "", "更新周期")
	updateViaProxy := flags.String("via-proxy", "", "订阅更新代理模式")
	include := flags.String("include", "", "节点包含表达式")
	exclude := flags.String("exclude", "", "节点排除表达式")
	allowInsecure := flags.Bool("allow-insecure", false, "跳过 TLS 证书校验")
	timeout := flags.Int64("timeout", 0, "下载超时秒数")
	now := flags.Int64("now", time.Now().Unix(), "当前 Unix 时间")
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*root) == "" || strings.TrimSpace(*groupID) == "" {
		return errors.New("subscription edit 需要 --root 和 --group")
	}
	var headers *map[string]string
	if flagWasSet(flags, "headers-file") {
		value, err := readHeadersFile(*headersFile)
		if err != nil {
			return err
		}
		headers = &value
	}
	var intervalSeconds *int64
	if flagWasSet(flags, "interval") {
		value, err := subscription.DurationToSeconds(*interval)
		if err != nil {
			return err
		}
		intervalSeconds = &value
	}
	options := subscription.EditOptions{
		Root: *root, GroupID: *groupID, ProgressDir: *progressDir, ProxyURL: *proxyURL,
		FallbackDirect: *fallbackDirect, Now: time.Unix(*now, 0), CustomHeaders: headers,
	}
	if flagWasSet(flags, "name") {
		options.Name = name
	}
	if flagWasSet(flags, "url") {
		options.URL = subscriptionURL
	}
	if flagWasSet(flags, "user-agent") {
		options.UserAgent = userAgent
	}
	if flagWasSet(flags, "hwid") {
		options.HWID = hwid
	}
	if flagWasSet(flags, "auto-update") {
		options.AutoUpdate = autoUpdate
	}
	options.UpdateInterval = intervalSeconds
	if flagWasSet(flags, "via-proxy") {
		options.UpdateViaProxy = updateViaProxy
	}
	if flagWasSet(flags, "include") {
		options.Include = include
	}
	if flagWasSet(flags, "exclude") {
		options.Exclude = exclude
	}
	if flagWasSet(flags, "allow-insecure") {
		options.AllowInsecure = allowInsecure
	}
	if flagWasSet(flags, "timeout") {
		options.Timeout = timeout
	}
	edited, err := subscription.Edit(ctx, options)
	if err != nil {
		var structured *subscription.Error
		if errors.As(err, &structured) {
			return &resultError{Code: structured.Code, Message: structured.Message, Data: structured.Data}
		}
		return err
	}
	if flagWasSet(flags, "format") && *format == "kv" {
		fmt.Printf("group_id=%s\nname_changed=%t\nrequires_update=%t\nnode_count=%d\nrevision=%d\nnot_modified=%t\nstructure_changed=%t\n", edited.GroupID, edited.NameChanged, edited.RequiresUpdate, edited.NodeCount, edited.Revision, edited.NotModified, edited.StructureChanged)
		return nil
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.edited", Message: "订阅设置已更新", Data: edited})
	return nil
}

func flagWasSet(flags *flag.FlagSet, name string) bool {
	set := false
	flags.Visit(func(value *flag.Flag) {
		if value.Name == name {
			set = true
		}
	})
	return set
}
