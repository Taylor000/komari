package subscriptions

import (
	"testing"
	"time"

	"github.com/komari-monitor/komari/database/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func subscriptionAt(expires time.Time, cycle int, auto bool) models.Subscription {
	return models.Subscription{
		Name: "test", Currency: "¥", BillingCycle: cycle,
		ExpiresAt: models.FromTime(expires), AutoRenewal: auto,
	}
}

func TestNextExpiry(t *testing.T) {
	loc := models.GetAppLocation()
	tests := []struct {
		name    string
		expires time.Time
		now     time.Time
		cycle   int
		auto    bool
		want    time.Time
	}{
		{
			name:    "monthly cycle catches up from saved date",
			expires: time.Date(2026, 7, 7, 0, 0, 0, 0, loc),
			now:     time.Date(2026, 9, 8, 10, 0, 0, 0, loc), cycle: 30, auto: true,
			want: time.Date(2026, 10, 7, 0, 0, 0, 0, loc),
		},
		{
			name:    "monthly cycle clamps month end",
			expires: time.Date(2026, 1, 31, 0, 0, 0, 0, loc),
			now:     time.Date(2026, 2, 1, 0, 0, 0, 0, loc), cycle: 30, auto: true,
			want: time.Date(2026, 2, 28, 0, 0, 0, 0, loc),
		},
		{
			name:    "custom day cycle",
			expires: time.Date(2026, 9, 1, 0, 0, 0, 0, loc),
			now:     time.Date(2026, 9, 8, 0, 0, 0, 0, loc), cycle: 10, auto: true,
			want: time.Date(2026, 9, 11, 0, 0, 0, 0, loc),
		},
		{
			name:    "manual renewal is unchanged",
			expires: time.Date(2026, 9, 1, 0, 0, 0, 0, loc),
			now:     time.Date(2026, 9, 8, 0, 0, 0, 0, loc), cycle: 30, auto: false,
			want: time.Date(2026, 9, 1, 0, 0, 0, 0, loc),
		},
		{
			name:    "one-time subscription is unchanged",
			expires: time.Date(2026, 9, 1, 0, 0, 0, 0, loc),
			now:     time.Date(2026, 9, 8, 0, 0, 0, 0, loc), cycle: -1, auto: true,
			want: time.Date(2026, 9, 1, 0, 0, 0, 0, loc),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextExpiry(subscriptionAt(tt.expires, tt.cycle, tt.auto), tt.now)
			require.True(t, got.Equal(tt.want), "got %s, want %s", got, tt.want)
		})
	}
}

func TestRenewDueIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Subscription{}))

	loc := models.GetAppLocation()
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, loc)
	item := subscriptionAt(time.Date(2026, 9, 7, 0, 0, 0, 0, loc), 30, true)
	require.NoError(t, db.Create(&item).Error)
	require.NoError(t, renewDue(db, now))
	require.NoError(t, renewDue(db, now))

	var got models.Subscription
	require.NoError(t, db.First(&got, item.ID).Error)
	require.True(t, got.ExpiresAt.ToTime().Equal(time.Date(2026, 10, 7, 0, 0, 0, 0, loc)))
}
