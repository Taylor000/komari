package subscriptions

import (
	"time"

	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"gorm.io/gorm"
)

// nextExpiry advances from the saved billing date, catching up missed cycles.
// The calendar cycles match the presets shown in the subscription editor.
func nextExpiry(item models.Subscription, now time.Time) time.Time {
	expires := item.ExpiresAt.ToTime()
	if !item.AutoRenewal || item.BillingCycle <= 0 || expires.Year() < 2 || expires.After(now) {
		return expires
	}
	expires = expires.In(models.GetAppLocation())
	months := 0
	switch item.BillingCycle {
	case 30:
		months = 1
	case 90:
		months = 3
	case 180:
		months = 6
	case 365:
		months = 12
	}
	if months == 0 {
		// Estimate the number of elapsed fixed-day cycles, then adjust for
		// daylight-saving transitions through calendar-day arithmetic.
		cycles := int(now.Sub(expires)/(24*time.Hour))/item.BillingCycle + 1
		if cycles < 1 {
			cycles = 1
		}
		next := expires.AddDate(0, 0, item.BillingCycle*cycles)
		for !next.After(now) {
			cycles++
			next = expires.AddDate(0, 0, item.BillingCycle*cycles)
		}
		return next
	}
	for cycles := 1; ; cycles++ {
		// Clamp the day to the target month's end (Jan 31 -> Feb 28),
		// instead of letting AddDate overflow into the following month.
		month := time.Date(expires.Year(), expires.Month()+time.Month(months*cycles), 1,
			expires.Hour(), expires.Minute(), expires.Second(), expires.Nanosecond(), expires.Location())
		day := expires.Day()
		if last := month.AddDate(0, 1, -1).Day(); day > last {
			day = last
		}
		next := month.AddDate(0, 0, day-1)
		if next.After(now) {
			return next
		}
	}
}

// renewSubscription uses a conditional update so a concurrent check or edit
// cannot renew the same saved expiry twice or overwrite newer billing settings.
func renewSubscription(db *gorm.DB, item models.Subscription, now time.Time) error {
	next := nextExpiry(item, now)
	if next.Equal(item.ExpiresAt.ToTime()) {
		return nil
	}
	return db.Model(&models.Subscription{}).
		Where("id = ? AND auto_renewal = ? AND billing_cycle = ? AND expires_at = ?",
			item.ID, true, item.BillingCycle, item.ExpiresAt).
		Updates(map[string]interface{}{
			"expires_at": models.FromTime(next),
			"updated_at": models.FromTime(now),
		}).Error
}

func renewDue(db *gorm.DB, now time.Time) error {
	var items []models.Subscription
	if err := db.Where("auto_renewal = ? AND billing_cycle > 0 AND expires_at <= ?",
		true, models.FromTime(now)).Find(&items).Error; err != nil {
		return err
	}
	for _, item := range items {
		if err := renewSubscription(db, item, now); err != nil {
			return err
		}
	}
	return nil
}

// AutoRenewDue updates tracked dates only; it does not charge any provider.
// Hidden subscriptions also renew, independently of server connectivity.
func AutoRenewDue() error {
	return renewDue(dbcore.GetDBInstance(), time.Now())
}
