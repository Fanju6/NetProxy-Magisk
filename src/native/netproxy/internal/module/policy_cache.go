package module

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"time"

	"github.com/sagernet/bbolt"
)

// reF1nd restores manual selections before consulting default. Only while
// stopped, remove a conflicting entry for this Selector, not the cache itself.
func clearConflictingPolicyCache(root map[string]jsontext.Value, dir, group, member string) error {
	var experimental struct {
		Cache struct {
			Enabled bool   `json:"enabled"`
			Path    string `json:"path"`
			ID      string `json:"cache_id"`
		} `json:"cache_file"`
	}
	if raw := root["experimental"]; len(raw) != 0 {
		if err := json.Unmarshal(raw, &experimental); err != nil {
			return err
		}
	}
	cache := experimental.Cache
	if !cache.Enabled {
		return nil
	}
	if cache.Path == "" {
		cache.Path = "cache.db"
	}
	if !filepath.IsAbs(cache.Path) {
		cache.Path = filepath.Join(dir, cache.Path)
	}
	info, err := os.Stat(cache.Path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return policySelectionError("cache_failed", "无法读取 sing-box 选择缓存，默认节点未修改")
	}
	db, err := bbolt.Open(cache.Path, 0o600, &bbolt.Options{
		Timeout: time.Second,
		OpenFile: func(name string, flag int, mode os.FileMode) (*os.File, error) {
			return os.OpenFile(name, flag&^os.O_CREATE, mode)
		},
	})
	if err != nil {
		return policySelectionError("cache_failed", "无法打开 sing-box 选择缓存，默认节点未修改")
	}
	defer db.Close()
	err = db.Update(func(tx *bbolt.Tx) error {
		const selection = "selected"
		bucket := tx.Bucket([]byte(selection))
		if cache.ID != "" {
			namespace := tx.Bucket(append([]byte{0}, cache.ID...))
			if namespace == nil {
				return nil
			}
			bucket = namespace.Bucket([]byte(selection))
		}
		if bucket != nil {
			key := []byte(group)
			if previous := bucket.Get(key); len(previous) > 0 && string(previous) != member {
				return bucket.Delete(key)
			}
		}
		return nil
	})
	if err != nil {
		return policySelectionError("cache_failed", "无法清除该策略组的旧选择，默认节点未修改")
	}
	return nil
}
