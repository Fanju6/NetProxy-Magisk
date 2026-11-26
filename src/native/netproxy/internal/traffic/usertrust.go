package traffic

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/processlock"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

const (
	userCredentialRoot = "/data/misc/user"
	userCertDir        = "cacerts-added"
	userTrustMarker    = ".netproxy-debug-ca"
	androidSystemUID   = 1000
	userCertSlots      = 64
)

type userCAOwnership struct {
	Name   string `json:"name"`
	Digest string `json:"sha256"`
}

type CAStores struct {
	User   string `json:"user"`
	System string `json:"system"`
}

func observeCAStores(app *SourceApp, cert Certificate) *CAStores {
	if runtime.GOOS != "android" {
		return nil
	}
	stores := &CAStores{User: "unknown", System: "unknown"}
	if app != nil {
		user := filepath.Join(userCredentialRoot, strconv.FormatUint(uint64(app.UserID), 10))
		if root, err := os.OpenRoot(user); err == nil {
			stores.User = userCAStoreState(root, cert)
			_ = root.Close()
		}
	}
	for i := len(systemStores) - 1; i >= 0; i-- {
		store := procRoot(1, systemStores[i].target)
		if _, err := os.Stat(store); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			break
		}
		path := filepath.Join(store, cert.Filename)
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			stores.System = "absent"
		} else if err == nil {
			if content, err := readStockCA(path); err == nil {
				stores.System = "absent"
				if certificateDigest(content) == cert.SHA256 {
					stores.System = "present"
				}
			}
		}
		break
	}
	return stores
}

func userCAStoreState(root *os.Root, cert Certificate) string {
	base := strings.TrimSuffix(cert.Filename, ".0")
	for index := range userCertSlots {
		content, err := readUserCAFile(root, filepath.Join(userCertDir, base+"."+strconv.Itoa(index)))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "unknown"
		}
		if certificateDigest(content) == cert.SHA256 {
			return "present"
		}
	}
	return "absent"
}

func ReconcileUserTrust(ctx context.Context, layout paths.Layout) error {
	return reconcileUserTrust(ctx, layout, false)
}

func RemoveUserTrust(ctx context.Context, layout paths.Layout) error {
	return reconcileUserTrust(ctx, layout, true)
}

func desiredUserCA(layout paths.Layout) (*authority, error) {
	policy, err := loadPolicy(layout)
	if err != nil {
		return nil, err
	}
	_, markerErr := os.Stat(layout.TrafficUserTrust())
	if !policy.Enabled && errors.Is(markerErr, os.ErrNotExist) {
		return nil, nil
	}
	if markerErr != nil && !errors.Is(markerErr, os.ErrNotExist) {
		return nil, errors.New("无法读取个人 CA 安装状态")
	}
	ca, err := loadCA(layout.TrafficCA())
	if err != nil {
		return nil, errors.New("无法读取待安装的公开抓包 CA")
	}
	if errors.Is(markerErr, os.ErrNotExist) {
		if err := provider.WriteAtomic(layout.TrafficUserTrust(), nil, 0o600); err != nil {
			return nil, errors.New("无法保存个人 CA 安装状态")
		}
	}
	return ca, nil
}

func reconcileUserTrust(ctx context.Context, layout paths.Layout, remove bool) error {
	if runtime.GOOS != "android" || !layout.IsLive() {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	lock, err := processlock.Acquire(ctx, layout.TrafficTrust()+".user.lock")
	if err != nil {
		return errors.New("无法锁定个人 CA 存储")
	}
	defer lock.Release()

	var ca *authority
	if remove {
		if err := os.Remove(layout.TrafficUserTrust()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("无法清除个人 CA 安装状态")
		}
	} else {
		ca, err = desiredUserCA(layout)
		if err != nil {
			return err
		}
		if ca == nil {
			return nil
		}
	}

	users, err := os.ReadDir(userCredentialRoot)
	if err != nil {
		return errors.New("无法读取 Android 用户证书区")
	}
	var result error
	for _, user := range users {
		if ctx.Err() != nil {
			return errors.Join(result, ctx.Err())
		}
		id, err := strconv.ParseUint(user.Name(), 10, 16)
		if err != nil || id > (1<<31-1-androidSystemUID)/androidUIDRange || !user.IsDir() || strconv.FormatUint(id, 10) != user.Name() {
			continue
		}
		userPath := filepath.Join(userCredentialRoot, user.Name())
		root, err := os.OpenRoot(userPath)
		if err != nil {
			result = errors.Join(result, fmt.Errorf("用户 %d: 无法打开证书区", id))
			continue
		}
		label := func(name string) error {
			return exec.CommandContext(ctx, "/system/bin/restorecon", filepath.Join(userPath, name)).Run()
		}
		uid := int(id)*androidUIDRange + androidSystemUID
		changed, err := reconcileUserCA(root, ca, uid, uid, label)
		_ = root.Close()
		if err != nil {
			result = errors.Join(result, fmt.Errorf("用户 %d: %w", id, err))
		}
		if changed {
			// 通知已运行的客户端刷新证书缓存。Android 的新进程直接读取用户存储。
			_ = exec.CommandContext(ctx, "/system/bin/am", "broadcast", "--user", user.Name(),
				"-a", "android.security.action.TRUST_STORE_CHANGED").Run()
		}
	}
	return result
}

func reconcileUserCA(root *os.Root, ca *authority, uid, gid int, label func(string) error) (bool, error) {
	info, dirErr := root.Lstat(userCertDir)
	if dirErr == nil && !info.IsDir() || dirErr != nil && !errors.Is(dirErr, os.ErrNotExist) {
		return false, errors.New("个人证书目录无效")
	}
	markerData, err := readUserCAFile(root, userTrustMarker)
	var marker userCAOwnership
	if err == nil {
		if json.Unmarshal(markerData, &marker, json.RejectUnknownMembers(true)) != nil {
			return false, errors.New("个人 CA 所有权记录无效")
		}
		rawDigest, digestErr := hex.DecodeString(marker.Digest)
		if !androidCAFilename.MatchString(marker.Name) || digestErr != nil || len(rawDigest) != sha256.Size {
			return false, errors.New("个人 CA 所有权记录无效")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, errors.New("无法读取个人 CA 所有权记录")
	}

	changed := false
	if marker.Name != "" {
		path := filepath.Join(userCertDir, marker.Name)
		data, readErr := readUserCAFile(root, path)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return false, errors.New("无法读取已有个人 CA")
		}
		if certificateDigest(data) == marker.Digest {
			if ca != nil && ca.public.SHA256 == marker.Digest {
				return false, nil
			}
			if err := removeOwnedUserCA(root, marker.Name); err != nil {
				return false, errors.New("无法清理模块安装的个人 CA")
			}
			changed = true
		}
		if err := root.Remove(userTrustMarker); err != nil {
			return changed, errors.New("无法清理个人 CA 所有权记录")
		}
	}
	if ca == nil {
		return changed, nil
	}

	if errors.Is(dirErr, os.ErrNotExist) {
		if err := root.Mkdir(userCertDir, 0o755); err != nil {
			return changed, errors.New("无法创建个人证书目录")
		}
		dir, err := root.Open(userCertDir)
		if err != nil {
			return changed, errors.New("无法打开个人证书目录")
		}
		err = errors.Join(dir.Chown(uid, gid), dir.Chmod(0o755), dir.Close())
		if err != nil || label(userCertDir) != nil {
			return changed, errors.New("无法设置个人证书目录权限或标签")
		}
	}

	base := strings.TrimSuffix(ca.public.Filename, ".0")
	for index := range userCertSlots {
		name := base + "." + strconv.Itoa(index)
		path := filepath.Join(userCertDir, name)
		data, err := readUserCAFile(root, path)
		if err == nil && certificateDigest(data) == ca.public.SHA256 {
			return changed, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			if err != nil {
				return changed, errors.New("无法安全检查个人证书目录")
			}
			continue
		}
		metadata, err := json.Marshal(userCAOwnership{Name: name, Digest: ca.public.SHA256})
		if err != nil || publishUserCAFile(root, userTrustMarker, metadata, 0o600, -1, -1, nil) != nil {
			return changed, errors.New("无法写入个人 CA 所有权记录")
		}
		if err := publishUserCAFile(root, path, ca.cert.Raw, 0o644, uid, gid, label); err != nil {
			return changed, errors.New("无法写入公开个人 CA 或设置文件标签")
		}
		return true, nil
	}
	return changed, errors.New("个人证书目录中的同哈希证书过多")
}

func readUserCAFile(root *os.Root, path string) ([]byte, error) {
	info, err := root.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 64<<10 {
		return nil, errors.New("证书文件无效")
	}
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, 64<<10))
}

func certificateDigest(data []byte) string {
	if block, _ := pem.Decode(data); block != nil && block.Type == "CERTIFICATE" {
		data = block.Bytes
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func publishUserCAFile(root *os.Root, name string, data []byte, mode os.FileMode, uid, gid int, label func(string) error) error {
	parent := filepath.Dir(name)
	tempName := filepath.Join(parent, ".netproxy-ca-"+rand.Text())
	temporary, err := root.OpenFile(tempName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(tempName)
	_, writeErr := temporary.Write(data)
	var ownerErr error
	if uid >= 0 {
		ownerErr = temporary.Chown(uid, gid)
	}
	if err := errors.Join(writeErr, ownerErr, temporary.Chmod(mode), temporary.Sync(), temporary.Close()); err != nil {
		return err
	}
	if label != nil {
		if err := label(tempName); err != nil {
			return err
		}
	}
	if err := root.Link(tempName, name); err != nil {
		return err
	}
	dir, err := root.Open(parent)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func removeOwnedUserCA(root *os.Root, name string) error {
	prefix, suffix, _ := strings.Cut(name, ".")
	index, err := strconv.Atoi(suffix)
	if err != nil || index < 0 || index >= userCertSlots || strconv.Itoa(index) != suffix {
		return errors.New("个人 CA 文件名无效")
	}
	path := filepath.Join(userCertDir, name)
	next := filepath.Join(userCertDir, prefix+"."+strconv.Itoa(index+1))
	if _, err := root.Lstat(next); errors.Is(err, os.ErrNotExist) {
		return root.Remove(path)
	} else if err != nil {
		return err
	}
	// Conscrypt 在遇到首个不存在的序号时停止扫描；保留更高序号的其他证书。
	f, err := root.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	return f.Close()
}
