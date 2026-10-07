package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
)

func TestPublicCommandsKeepSingleJSONContract(t *testing.T) {
	root := t.TempDir()
	options := moduleapp.NewOptions(root)
	options.StateFile = filepath.Join(root, "state", "service.json")
	options.ProgressDir = filepath.Join(root, "state", "subscriptions")
	options.WorkerPIDFile = filepath.Join(root, "state", "worker.pid")
	options.WiFiStateFile = filepath.Join(root, "state", "wifi_state")
	for path, content := range map[string]string{
		options.ModuleConfig:                             "ACTIVE_GROUP_ID=default\nSELECTOR_MODE=urltest\nOUTBOUND_MODE=rule\n",
		options.InboundConfig:                            `{"backend":"ebpf","app":{"enabled":false,"mode":"blacklist","proxy_apps":[],"bypass_apps":[]},"ebpf":{"type":"ebpf","tag":"netproxy-in","local":{"enabled":true},"shared":{"enabled":false}},"tun":{"type":"tun","tag":"netproxy-in","interface_name":"netproxy","address":["172.19.0.1/30"],"auto_route":true,"auto_redirect":true}}`,
		filepath.Join(options.SingBoxDir, "config.json"): "{}\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{Root: options.CatalogRoot, GroupID: "default", Name: "本地配置", Type: "local"}); err != nil {
		t.Fatal(err)
	}
	command := &cli{options: options}
	for _, test := range []struct {
		args   string
		code   string
		status int
	}{
		{"service status", "service.status", 0},
		{"catalog list", "catalog.groups", 0},
		{"catalog show default", "catalog.show", 0},
		{"node list", "node.list", 0},
		{"node current", "node.current", 0},
		{"sub list", "subscription.list", 0},
		{"mode", "mode.current", 0},
		{"network evaluate --type not_wifi", "network.evaluated", 0},
		{"app list", "app.list", 0},
		{"config list", "config.list", 0},
		{"config read module", "config.read", 0},
		{"logs show service", "logs.show", 0},
		{"node get invalid", "node.ref_invalid", 2},
		{"catalog show", "usage.invalid", 2},
		{"node import missing.yaml custom", "usage.invalid", 2},
	} {
		t.Run(test.args, func(t *testing.T) {
			capture, err := os.CreateTemp(t.TempDir(), "stdout-")
			if err != nil {
				t.Fatal(err)
			}
			defer capture.Close()
			previous := os.Stdout
			var status int
			func() {
				os.Stdout = capture
				defer func() { os.Stdout = previous }()
				status = command.run(t.Context(), append([]string{"--json"}, strings.Fields(test.args)...))
			}()
			payload, err := os.ReadFile(capture.Name())
			if err != nil {
				t.Fatal(err)
			}
			var response struct {
				Schema int            `json:"schema"`
				OK     bool           `json:"ok"`
				Code   string         `json:"code"`
				Data   jsontext.Value `json:"data"`
			}
			if err := json.Unmarshal(payload, &response); err != nil {
				t.Fatalf("stdout 不是单一 JSON: %s: %v", payload, err)
			}
			if status != test.status || response.Schema != 1 || response.OK != (status == 0) || response.Code != test.code {
				t.Fatalf("契约不匹配: exit=%d, %s", status, payload)
			}
			if test.args == "service status" {
				var data map[string]jsontext.Value
				if err := json.Unmarshal(response.Data, &data); err != nil || string(data["state"]) != `"stopped"` {
					t.Fatalf("服务状态必须直接位于 data: %s: %v", response.Data, err)
				}
			}
		})
	}
}
