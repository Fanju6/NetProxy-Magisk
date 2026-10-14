package apiconfig

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/processlock"
)

const DefaultListen = "127.0.0.1:9091"

type Config struct {
	Listen      string `json:"listen"`
	Secret      string `json:"secret"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"private_key"`
}

func (config Config) Validate() error {
	address, err := netip.ParseAddrPort(config.Listen)
	if err != nil || !address.Addr().Unmap().IsLoopback() || address.Port() == 0 {
		return errors.New("API 监听地址必须为带有效端口的本机 IP 地址")
	}
	secret, err := hex.DecodeString(config.Secret)
	if err != nil || len(secret) != 32 {
		return errors.New("API secret 必须为 32 字节随机值的十六进制编码")
	}
	return config.validateCertificate(address.Addr().Unmap())
}

func Load(path string) (Config, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Config{}, fmt.Errorf("读取 API 配置失败: %w", err)
	}
	if !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return Config{}, errors.New("API 配置必须为仅所有者可读写的普通文件")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("读取 API 配置失败: %w", err)
	}
	var config Config
	if err := json.Unmarshal(content, &config, json.RejectUnknownMembers(true)); err != nil {
		return Config{}, errors.New("API 配置格式无效")
	}
	return config, config.Validate()
}

// Initialize 只创建新凭据；已有配置即使损坏也不能被初始化操作覆盖。
func Initialize(ctx context.Context, path string) (Config, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Config{}, err
	}
	lock, err := processlock.Acquire(ctx, path+".lock")
	if err != nil {
		return Config{}, err
	}
	defer lock.Release()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return Load(path)
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Config{}, err
	}
	config := Config{Listen: DefaultListen, Secret: hex.EncodeToString(random[:])}
	if err := config.generateCertificate(); err != nil {
		return Config{}, err
	}
	content, err := json.Marshal(config, json.Deterministic(true))
	if err != nil {
		return Config{}, err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".api-")
	if err != nil {
		return Config{}, err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(append(content, '\n'))
	if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return Config{}, err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return Config{}, err
	}
	return config, nil
}
