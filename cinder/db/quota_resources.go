package cinder

import "strings"

// quotaGlobalResources are the resource keys Cinder registers without a
// volume type: VolumeTypeQuotaEngine's global quotas plus GroupQuotaEngine's
// groups, which share the same tables.
var quotaGlobalResources = map[string]bool{
	"volumes":              true,
	"per_volume_gigabytes": true,
	"snapshots":            true,
	"gigabytes":            true,
	"backups":              true,
	"backup_gigabytes":     true,
	"groups":               true,
}

// quotaVolumeTypeParts are the only resources Cinder tracks per volume type.
var quotaVolumeTypeParts = []string{"volumes", "gigabytes", "snapshots"}

// ParseQuotaResource splits a Cinder quota resource key into its base resource
// and volume type name.
//
// Cinder builds per-volume-type keys as "<part>_<volume type name>", where part
// is volumes, gigabytes or snapshots. A global key such as "backup_gigabytes"
// returns an empty volumeType. Volume type names may contain underscores, so
// "gigabytes_fast_ssd" returns ("gigabytes", "fast_ssd").
//
// The key holds the volume type's name, not its ID. Cinder rewrites keys when a
// type is renamed, but keeps them after a type is deleted, so a parsed name
// may not match any current volume type.
//
// ok is false for keys Cinder does not register. base is then the whole key.
func ParseQuotaResource(key string) (base, volumeType string, ok bool) {
	if quotaGlobalResources[key] {
		return key, "", true
	}
	for _, part := range quotaVolumeTypeParts {
		if name, found := strings.CutPrefix(key, part+"_"); found && name != "" {
			return part, name, true
		}
	}
	return key, "", false
}
