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

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subworker"
)

func runSubworker(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 Worker 操作: start|stop|restart|wake|status|once|run")
	}
	action := args[0]
	flags := newFlagSet("subworker " + action)
	root := flags.String("root", "", "Catalog 根目录")
	progressDir := flags.String("progress-dir", "/dev/netproxy/subscriptions", "订阅进度目录")
	pidFile := flags.String("pid-file", "/dev/netproxy/subworker.pid", "Worker PID 文件")
	logFile := flags.String("log-file", "", "Worker 日志文件")
	moduleConf := flags.String("module-conf", "", "模块配置文件")
	reloadScript := flags.String("reload-script", "", "服务 reload 适配脚本")
	singBox := flags.String("sing-box", "", "sing-box 二进制路径")
	serviceAddress := flags.String("service-address", "127.0.0.1:9090", "Service API 地址")
	serviceSecret := flags.String("service-secret", "singbox", "Service API 密钥")
	group := flags.String("group", "", "指定单个订阅分组")
	now := flags.Int64("now", time.Now().Unix(), "当前 Unix 时间")
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*root) == "" {
		return errors.New("subworker 需要 --root")
	}
	options := subworker.NewOptions(*root)
	options.ProgressDir = *progressDir
	options.PIDFile = *pidFile
	options.LogFile = *logFile
	options.ModuleConf = *moduleConf
	options.ReloadScript = *reloadScript
	options.SingBoxPath = *singBox
	options.ServiceAddress = *serviceAddress
	options.ServiceSecret = *serviceSecret
	if options.ModuleConf == "" {
		return errors.New("subworker 需要 --module-conf")
	}
	if options.LogFile == "" {
		options.LogFile = filepath.Join(filepath.Dir(*root), "..", "logs", "subscription.log")
	}
	if options.ReloadScript == "" {
		options.ReloadScript = filepath.Join(filepath.Dir(*root), "..", "scripts", "core", "service.sh")
	}
	switch action {
	case "run":
		logger, closer, err := subworker.OpenLogger(options.LogFile)
		if err != nil {
			return err
		}
		defer closer.Close()
		return subworker.RunProcess(ctx, options, logger)
	case "start":
		status, err := subworker.Start(ctx, options, os.Args[0])
		if err != nil {
			return err
		}
		return writeWorkerResult(*format, "subworker.started", "订阅 Worker 已启动", status)
	case "stop":
		if err := subworker.Stop(options); err != nil {
			return err
		}
		return writeWorkerResult(*format, "subworker.stopped", "订阅 Worker 已停止", subworker.Status{State: "stopped"})
	case "restart":
		if err := subworker.Stop(options); err != nil {
			return err
		}
		status, err := subworker.Start(ctx, options, os.Args[0])
		if err != nil {
			return err
		}
		return writeWorkerResult(*format, "subworker.restarted", "订阅 Worker 已重启", status)
	case "wake":
		status, err := subworker.Wake(ctx, options, os.Args[0])
		if err != nil {
			return err
		}
		return writeWorkerResult(*format, "subworker.woken", "订阅 Worker 已唤醒", status)
	case "status":
		status, err := subworker.ReadStatus(options)
		if err != nil {
			return err
		}
		if *format == "tsv" {
			fmt.Printf("state\t%s\npid\t%d\nnearest\t%d\n", status.State, status.PID, status.Nearest)
			return nil
		}
		return writeWorkerResult(*format, "subworker.status", "订阅 Worker 状态", status)
	case "once":
		logger, closer, err := subworker.OpenLogger(options.LogFile)
		if err != nil {
			return err
		}
		defer closer.Close()
		currentTime := time.Unix(*now, 0)
		if *group != "" {
			updated, updateErr := subworker.UpdateGroup(ctx, options, *group, currentTime, logger)
			if updateErr != nil {
				return updateErr
			}
			if *format == "kv" {
				fmt.Printf("group_id=%s\nnode_count=%d\nrevision=%d\nnot_modified=%t\nstructure_changed=%t\n", updated.GroupID, updated.NodeCount, updated.Revision, updated.NotModified, updated.StructureChanged)
				return nil
			}
			return writeWorkerResult(*format, "subworker.once", "订阅更新完成", updated)
		}
		summary, err := subworker.RunDue(ctx, options, currentTime, logger)
		if err != nil {
			return err
		}
		return writeWorkerResult(*format, "subworker.once", "订阅到期任务已处理", summary)
	default:
		return fmt.Errorf("未知 Worker 操作 %q", action)
	}
}

func writeWorkerResult(format, code, message string, data any) error {
	if format == "tsv" {
		encoded, err := json.Marshal(data)
		if err != nil {
			return err
		}
		fmt.Println(string(encoded))
		return nil
	}
	if format != "json" {
		return fmt.Errorf("Worker 不支持输出格式 %q", format)
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: code, Message: message, Data: data})
	return nil
}
