package pgoworkload

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

const moduleRootEnvironment = "NETPROXY_PGO_MODULE_DIR"

func BenchmarkAndroidPGO(b *testing.B) {
	moduleRoot := os.Getenv(moduleRootEnvironment)
	if moduleRoot == "" {
		b.Skipf("仅用于 Android PGO 采样；请设置 %s", moduleRootEnvironment)
	}
	layout := paths.New(moduleRoot)
	if _, err := os.Stat(layout.ModuleConfig()); err != nil {
		b.Fatalf("模块配置不可用: %v", err)
	}
	options := service.Options{
		CatalogRoot:    layout.Catalog(),
		ModuleConfig:   layout.ModuleConfig(),
		StateFile:      layout.ServiceState(),
		ProgressDir:    layout.ProgressDir(),
		WorkerPIDFile:  layout.WorkerPID(),
		SingBoxPath:    layout.SingBox(),
		ServiceAddress: "127.0.0.1:9090",
		ServiceSecret:  "singbox",
		RequestTimeout: 2 * time.Second,
	}

	b.Run("ServiceStatus", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if _, err := service.ReadStatus(context.Background(), options); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("NodeSnapshot", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if _, err := service.ReadSnapshot(context.Background(), options, ""); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("RuntimeBuild", func(b *testing.B) {
		outputDir := b.TempDir()
		runtimeOptions := catalog.RuntimeOptions{
			Root:            layout.Catalog(),
			ModuleConfig:    layout.ModuleConfig(),
			ProvidersOutput: filepath.Join(outputDir, "providers.json"),
			OutboundsOutput: filepath.Join(outputDir, "outbounds.json"),
		}
		b.ReportAllocs()
		for range b.N {
			if _, err := catalog.BuildRuntime(context.Background(), runtimeOptions); err != nil {
				b.Fatal(err)
			}
		}
	})
}
