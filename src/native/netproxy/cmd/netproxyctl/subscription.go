package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

func (c *cli) subscription(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 sub 操作")
	}
	action := args[0]
	flags := newFlagSet("sub")
	name := flags.String("name", "", "订阅名称")
	urlValue := flags.String("url", "", "订阅 URL")
	userAgent := flags.String("user-agent", "", "订阅 User-Agent")
	hwid := flags.String("hwid", "", "订阅 HWID")
	headersFile := flags.String("headers-file", "", "请求头 JSON 文件")
	interval := flags.String("interval", "24h", "更新周期")
	autoUpdate := flags.Bool("auto-update", true, "自动更新")
	viaProxy := flags.String("via-proxy", "auto", "更新代理模式")
	frontProxy := flags.String("front-proxy", "", "前置代理节点引用")
	landingProxy := flags.String("landing-proxy", "", "落地代理节点引用")
	include := flags.String("include", "", "节点包含表达式")
	exclude := flags.String("exclude", "", "节点排除表达式")
	allowInsecure := flags.Bool("allow-insecure", false, "跳过 TLS 校验")
	timeout := flags.Int64("download-timeout", 60, "下载超时秒数")
	private := flags.Bool("private", false, "返回订阅私有设置")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	options := c.options
	positionals := flags.Args()
	if action == "list" {
		groups, err := catalog.Scan(ctx, catalog.ScanOptions{Root: options.CatalogRoot, Type: "subscription", ActiveGroup: readActiveGroup(options), ProgressDir: options.ProgressDir, WithNodes: false})
		if err != nil {
			return err
		}
		data := make([]catalog.GroupSummary, 0, len(groups))
		for _, group := range groups {
			data = append(data, group.Group)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.list", Message: "订阅列表", Data: data})
		return nil
	}
	if action == "add" {
		if len(positionals) > 0 && *urlValue == "" {
			*urlValue = positionals[len(positionals)-1]
			if len(positionals) > 1 && *name == "" {
				*name = positionals[0]
			}
		}
		if *urlValue == "" {
			return errors.New("sub add 需要 URL")
		}
		seconds, err := catalog.DurationToSeconds(*interval)
		if err != nil {
			return err
		}
		headers := map[string]string{}
		if *headersFile != "" {
			content, readErr := os.ReadFile(*headersFile)
			if readErr != nil {
				return readErr
			}
			if err := json.Unmarshal(content, &headers); err != nil {
				return err
			}
		}
		updated, err := moduleapp.AddSubscription(ctx, moduleapp.SubscriptionOptions{Options: options, Name: *name, URL: *urlValue, UserAgent: *userAgent, HWID: *hwid, Headers: headers, AutoUpdate: *autoUpdate, UpdateInterval: seconds, IntervalSource: "user", UpdateViaProxy: *viaProxy, FrontProxy: *frontProxy, LandingProxy: *landingProxy, Include: *include, Exclude: *exclude, AllowInsecure: *allowInsecure, Timeout: *timeout})
		if err != nil {
			return moduleSubscriptionError(err)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.added", Message: "订阅已添加", Data: updated})
		return nil
	}
	if action == "update" {
		if len(positionals) == 0 {
			return errors.New("sub update 需要订阅")
		}
		updated, err := moduleapp.UpdateSubscription(ctx, options, positionals[0])
		if err != nil {
			return moduleSubscriptionError(err)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.updated", Message: "订阅更新完成", Data: updated})
		return nil
	}
	if action == "edit" {
		if len(positionals) == 0 {
			return errors.New("sub edit 需要订阅")
		}
		var headers *map[string]string
		if *headersFile != "" {
			content, readErr := os.ReadFile(*headersFile)
			if readErr != nil {
				return readErr
			}
			value := map[string]string{}
			if err := json.Unmarshal(content, &value); err != nil {
				return err
			}
			headers = &value
		}
		var intervalSeconds *int64
		if flagWasSet(flags, "interval") {
			value, err := catalog.DurationToSeconds(*interval)
			if err != nil {
				return err
			}
			intervalSeconds = &value
		}
		edit := subscription.EditOptions{Now: time.Now(), CustomHeaders: headers}
		if flagWasSet(flags, "name") {
			edit.Name = name
		}
		if flagWasSet(flags, "url") {
			edit.URL = urlValue
		}
		if flagWasSet(flags, "user-agent") {
			edit.UserAgent = userAgent
		}
		if flagWasSet(flags, "hwid") {
			edit.HWID = hwid
		}
		if flagWasSet(flags, "auto-update") {
			edit.AutoUpdate = autoUpdate
		}
		if intervalSeconds != nil {
			edit.UpdateInterval = intervalSeconds
		}
		if flagWasSet(flags, "via-proxy") {
			edit.UpdateViaProxy = viaProxy
		}
		if flagWasSet(flags, "front-proxy") {
			edit.FrontProxy = frontProxy
		}
		if flagWasSet(flags, "landing-proxy") {
			edit.LandingProxy = landingProxy
		}
		if flagWasSet(flags, "include") {
			edit.Include = include
		}
		if flagWasSet(flags, "exclude") {
			edit.Exclude = exclude
		}
		if flagWasSet(flags, "allow-insecure") {
			edit.AllowInsecure = allowInsecure
		}
		if flagWasSet(flags, "download-timeout") {
			edit.Timeout = timeout
		}
		edited, err := moduleapp.EditSubscription(ctx, options, positionals[0], edit)
		if err != nil {
			return moduleSubscriptionError(err)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.edited", Message: "订阅设置已更新", Data: edited})
		return nil
	}
	if action == "update-all" {
		summary, err := moduleapp.UpdateAllSubscriptions(ctx, options)
		if err != nil {
			return moduleSubscriptionError(err)
		}
		if len(summary.Failed) > 0 {
			return fmt.Errorf("部分订阅更新失败")
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.updated_all", Message: "全部订阅更新完成", Data: summary})
		return nil
	}
	if action == "activate" {
		if len(positionals) == 0 {
			return errors.New("sub activate 需要订阅")
		}
		data, err := moduleapp.SelectNode(ctx, options, "auto", positionals[0])
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.activated", Message: "活动订阅已切换", Data: data})
		return nil
	}
	if action == "remove" {
		if len(positionals) == 0 {
			return errors.New("sub remove 需要订阅")
		}
		replacement := ""
		if len(positionals) > 1 {
			replacement = positionals[1]
		}
		if err := moduleapp.RemoveSubscription(ctx, options, positionals[0], replacement); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.removed", Message: "订阅已删除", Data: map[string]string{"id": positionals[0]}})
		return nil
	}
	if action == "cancel" {
		if len(positionals) == 0 {
			return errors.New("sub cancel 需要订阅")
		}
		id, err := catalog.ResolveGroup(ctx, options.CatalogRoot, positionals[0])
		if err != nil {
			return err
		}
		if _, err := subscription.RequestCancel(options.CatalogRoot, id, options.ProgressDir); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.cancelled", Message: "已请求取消订阅更新", Data: map[string]string{"id": id}})
		return nil
	}
	if action == "show" || action == "history" {
		if len(positionals) == 0 {
			return errors.New("sub 操作需要订阅")
		}
		id, err := catalog.ResolveGroup(ctx, options.CatalogRoot, positionals[0])
		if err != nil {
			return err
		}
		if action == "history" {
			data, err := subscription.LoadHistory(filepath.Join(options.CatalogRoot, id, "history.jsonl"))
			if err != nil {
				return err
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.history", Message: "订阅更新历史", Data: data})
			return nil
		}
		if *private || (len(positionals) > 1 && positionals[1] == "--private") {
			data, err := catalog.PrivateMetadata(ctx, options.CatalogRoot, id)
			if err != nil {
				return err
			}
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.show", Message: "订阅详情", Data: data})
			return nil
		}
		groups, err := catalog.Scan(ctx, catalog.ScanOptions{Root: options.CatalogRoot, GroupID: id, ProgressDir: options.ProgressDir, WithNodes: true})
		if err != nil || len(groups) == 0 {
			if err != nil {
				return err
			}
			return errors.New("订阅不存在")
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "subscription.show", Message: "订阅详情", Data: groups[0]})
		return nil
	}
	return fmt.Errorf("未知 sub 操作 %q", action)
}

func flagWasSet(flags *flag.FlagSet, name string) bool {
	found := false
	flags.Visit(func(value *flag.Flag) {
		if value.Name == name {
			found = true
		}
	})
	return found
}
