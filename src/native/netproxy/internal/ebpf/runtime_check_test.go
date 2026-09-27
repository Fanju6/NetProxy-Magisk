package ebpf

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"encoding/json/jsontext"
	json "encoding/json/v2"
)

func TestRuntimeWithTargetSingBoxCheck(t *testing.T) {
	binary := os.Getenv("NETPROXY_TEST_SINGBOX")
	if binary == "" {
		t.Skip("需要目标 Linux/Android sing-box 二进制")
	}
	for _, mode := range []string{"local", "shared", "both"} {
		t.Run(mode, func(t *testing.T) {
			content := "EBPF_LOCAL_BYPASS_RULE_SET=\"\"\nEBPF_SHARED_BYPASS_RULE_SET=\"\"\nEBPF_NETWORK=\"tcp,udp\"\n"
			switch mode {
			case "local":
				content += "EBPF_LOCAL_ENABLED=1\nEBPF_SHARED_ENABLED=0\n"
			case "shared":
				content += "EBPF_LOCAL_ENABLED=0\nEBPF_SHARED_ENABLED=1\nEBPF_SHARED_INTERFACES=\"eth0\"\n"
			case "both":
				content += "EBPF_LOCAL_ENABLED=1\nEBPF_SHARED_ENABLED=1\nEBPF_SHARED_INTERFACES=\"eth0\"\n"
			}
			config := loadFixture(t, content)
			value, err := config.Build(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(value.Runtime, json.Deterministic(true))
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]jsontext.Value
			if err := json.Unmarshal(encoded, &document); err != nil {
				t.Fatal(err)
			}
			document["outbounds"] = jsontext.Value(`[{"type":"direct","tag":"direct"}]`)
			encoded, err = json.Marshal(document, json.Deterministic(true))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.CommandContext(t.Context(), binary, "check", "-c", path).CombinedOutput(); err != nil {
				t.Fatalf("目标 sing-box check 失败: %v\n%s\n%s", err, output, encoded)
			}
		})
	}
}
