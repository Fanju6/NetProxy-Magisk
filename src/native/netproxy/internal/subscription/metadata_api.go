package subscription

import (
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
)

type Metadata = catalog.Metadata

func NewMetadata(id, name, metadataType, rawURL string, now time.Time) Metadata {
	return catalog.NewMetadata(id, name, metadataType, rawURL, now)
}

func DurationToSeconds(value string) (int64, error) {
	return catalog.DurationToSeconds(value)
}

func FormatEpochUTC(epoch int64) string {
	return catalog.FormatEpochUTC(epoch)
}

func ScheduleAt(metadata *Metadata, now time.Time) {
	catalog.ScheduleAt(metadata, now)
}

func LoadMetadata(path, fallbackID string) (Metadata, error) {
	return catalog.LoadMetadata(path, fallbackID)
}

func SaveMetadataAtomic(path string, metadata Metadata) error {
	return catalog.SaveMetadataAtomic(path, metadata)
}

func normalizeMetadata(metadata Metadata) Metadata {
	return catalog.NormalizeMetadata(metadata)
}
