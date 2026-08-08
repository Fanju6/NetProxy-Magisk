package subscription

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"strings"
)

// LoadHistory 读取并校验 Catalog 分组的 JSONL 更新历史。
func LoadHistory(path string) ([]json.RawMessage, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []json.RawMessage{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	entries := make([]json.RawMessage, 0, 20)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !json.Valid([]byte(line)) {
			return nil, errors.New("订阅历史包含无效 JSON")
		}
		entries = append(entries, json.RawMessage(append([]byte(nil), []byte(line)...)))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
