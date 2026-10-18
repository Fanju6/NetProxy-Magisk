package provider

import "reflect"

// Member snapshots cannot acknowledge a same-tag protocol or credential change.
// Added/removed members can use native file watching; replaced members need reload.
func ReplacesMembers(before, after Document) bool {
	old := make(map[string]any, len(before.Outbounds)+len(before.Endpoints))
	for _, node := range before.Outbounds {
		old[node.Tag] = node
	}
	for _, node := range before.Endpoints {
		old[node.Tag] = node
	}
	for _, node := range after.Outbounds {
		if previous, exists := old[node.Tag]; exists && !reflect.DeepEqual(previous, node) {
			return true
		}
	}
	for _, node := range after.Endpoints {
		if previous, exists := old[node.Tag]; exists && !reflect.DeepEqual(previous, node) {
			return true
		}
	}
	return false
}
