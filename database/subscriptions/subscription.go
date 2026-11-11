package subscriptions

import (
	"errors"
	"time"

	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"gorm.io/gorm"
)

func List(includeHidden bool) ([]models.Subscription, error) {
	var list []models.Subscription
	query := dbcore.GetDBInstance().Order("weight ASC, id ASC")
	if !includeHidden {
		query = query.Where("visible = ?", true)
	}
	err := query.Find(&list).Error
	return list, err
}

func Create(item *models.Subscription) error {
	db := dbcore.GetDBInstance()
	return db.Transaction(func(tx *gorm.DB) error {
		var maxWeight int
		if err := tx.Model(&models.Subscription{}).
			Select("COALESCE(MAX(weight), -1)").Scan(&maxWeight).Error; err != nil {
			return err
		}
		item.Weight = maxWeight + 1
		item.ExpiresAt = models.FromTime(nextExpiry(*item, time.Now()))
		return tx.Create(item).Error
	})
}

func Update(id uint, fields map[string]interface{}) (*models.Subscription, error) {
	db := dbcore.GetDBInstance()
	var item models.Subscription
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&item, id).Error; err != nil {
			return err
		}
		if err := tx.Model(&item).Updates(fields).Error; err != nil {
			return err
		}
		if err := tx.First(&item, id).Error; err != nil {
			return err
		}
		if err := renewSubscription(tx, item, time.Now()); err != nil {
			return err
		}
		return tx.First(&item, id).Error
	})
	return &item, err
}

func Delete(id uint) error {
	db := dbcore.GetDBInstance()
	result := db.Delete(&models.Subscription{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Reorder sets each subscription's weight to its position in ids. ids must
// list every subscription exactly once, so a stale page cannot leave gaps or
// duplicate weights behind.
func Reorder(ids []uint) error {
	db := dbcore.GetDBInstance()
	return db.Transaction(func(tx *gorm.DB) error {
		var total int64
		if err := tx.Model(&models.Subscription{}).Count(&total).Error; err != nil {
			return err
		}
		seen := make(map[uint]struct{}, len(ids))
		for _, id := range ids {
			seen[id] = struct{}{}
		}
		if int64(len(ids)) != total || len(seen) != len(ids) {
			return ErrOrderMismatch
		}
		for weight, id := range ids {
			result := tx.Model(&models.Subscription{}).Where("id = ?", id).Update("weight", weight)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrOrderMismatch
			}
		}
		return nil
	})
}

// ErrOrderMismatch means the submitted order does not match the stored list.
var ErrOrderMismatch = errors.New("subscription order does not match the current list")
