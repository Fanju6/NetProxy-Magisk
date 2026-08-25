package subscription

import (
	"bufio"
	"encoding/json/jsontext"
	"errors"
	"os"
	"strings"
)

// LoadHistory 读取并校验 Catalog 分组的 JSONL 更新历史。
func LoadHistory(path string) ([]jsontext.Value, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []jsontext.Value{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	entries := make([]jsontext.Value, 0, 20)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !jsontext.Value([]byte(line)).IsValid() {
			return nil, errors.New("订阅历史包含无效 JSON")
		}
		entries = append(entries, jsontext.Value(append([]byte(nil), []byte(line)...)))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
