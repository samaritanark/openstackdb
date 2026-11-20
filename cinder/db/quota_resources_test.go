package cinder

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseQuotaResource(t *testing.T) {
	for _, tc := range []struct {
		key, base, volumeType string
		ok                    bool
	}{
		{"volumes", "volumes", "", true},
		{"gigabytes", "gigabytes", "", true},
		{"snapshots", "snapshots", "", true},
		{"backups", "backups", "", true},
		{"backup_gigabytes", "backup_gigabytes", "", true},
		{"per_volume_gigabytes", "per_volume_gigabytes", "", true},
		{"groups", "groups", "", true},
		{"volumes_lvm", "volumes", "lvm", true},
		{"gigabytes_fast_ssd", "gigabytes", "fast_ssd", true},
		{"snapshots___DEFAULT__", "snapshots", "__DEFAULT__", true},
		{"volumes_backup_gigabytes", "volumes", "backup_gigabytes", true},
		{"backups_lvm", "backups_lvm", "", false},
		{"volumes_", "volumes_", "", false},
		{"", "", "", false},
	} {
		t.Run(tc.key, func(t *testing.T) {
			base, volumeType, ok := ParseQuotaResource(tc.key)
			require.Equal(t, tc.base, base)
			require.Equal(t, tc.volumeType, volumeType)
			require.Equal(t, tc.ok, ok)
		})
	}
}
