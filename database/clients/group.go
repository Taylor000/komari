package clients

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func normalizeServerGroupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("group name cannot be empty")
	}
	if utf8.RuneCountInString(name) > 100 {
		return "", fmt.Errorf("group name cannot exceed 100 characters")
	}
	return name, nil
}

// SyncLegacyServerGroups imports distinct names already stored on clients.
// This keeps upgrades backward compatible without requiring a one-off manual
// migration before the new group management page can be used.
func SyncLegacyServerGroups() error {
	db := dbcore.GetDBInstance()
	var names []string
	if err := db.Model(&models.Client{}).
		Distinct("`group`").
		Where("`group` <> ''").
		Pluck("`group`", &names).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var maxWeight int
		if err := tx.Model(&models.ServerGroup{}).
			Select("COALESCE(MAX(weight), -1)").
			Scan(&maxWeight).Error; err != nil {
			return err
		}

		for _, rawName := range names {
			name, err := normalizeServerGroupName(rawName)
			if err != nil {
				continue
			}
			if name != rawName {
				if err := tx.Model(&models.Client{}).Where("`group` = ?", rawName).Update("group", name).Error; err != nil {
					return err
				}
			}
			maxWeight++
			group := models.ServerGroup{
				Name:      name,
				Weight:    maxWeight,
				CreatedAt: models.FromTime(time.Now()),
				UpdatedAt: models.FromTime(time.Now()),
			}
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&group)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				maxWeight--
			}
		}
		return nil
	})
}

func ListServerGroups() ([]models.ServerGroup, error) {
	db := dbcore.GetDBInstance()
	var groups []models.ServerGroup
	if err := db.Order("weight ASC, id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}

	type groupCount struct {
		Name  string `gorm:"column:name"`
		Count int64  `gorm:"column:count"`
	}
	var counts []groupCount
	if err := db.Model(&models.Client{}).
		Select("`group` AS name, COUNT(*) AS count").
		Where("`group` <> ''").
		Group("`group`").
		Scan(&counts).Error; err != nil {
		return nil, err
	}
	countByName := make(map[string]int64, len(counts))
	for _, count := range counts {
		countByName[count.Name] = count.Count
	}
	for i := range groups {
		groups[i].ClientCount = countByName[groups[i].Name]
	}
	return groups, nil
}

func CreateServerGroup(rawName string) (models.ServerGroup, error) {
	name, err := normalizeServerGroupName(rawName)
	if err != nil {
		return models.ServerGroup{}, err
	}
	db := dbcore.GetDBInstance()
	var group models.ServerGroup
	err = db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.ServerGroup{}).Where("name = ?", name).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("group name already exists")
		}
		var maxWeight int
		if err := tx.Model(&models.ServerGroup{}).
			Select("COALESCE(MAX(weight), -1)").
			Scan(&maxWeight).Error; err != nil {
			return err
		}
		group = models.ServerGroup{
			Name:      name,
			Weight:    maxWeight + 1,
			CreatedAt: models.FromTime(time.Now()),
			UpdatedAt: models.FromTime(time.Now()),
		}
		return tx.Create(&group).Error
	})
	return group, err
}

func RenameServerGroup(id uint, rawName string) (models.ServerGroup, error) {
	name, err := normalizeServerGroupName(rawName)
	if err != nil {
		return models.ServerGroup{}, err
	}
	db := dbcore.GetDBInstance()
	var group models.ServerGroup
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&group, id).Error; err != nil {
			return err
		}
		if group.Name == name {
			return nil
		}
		var count int64
		if err := tx.Model(&models.ServerGroup{}).
			Where("name = ? AND id <> ?", name, id).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("group name already exists")
		}
		oldName := group.Name
		if err := tx.Model(&models.ServerGroup{}).Where("id = ?", id).Updates(map[string]interface{}{
			"name":       name,
			"updated_at": time.Now(),
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Client{}).Where("`group` = ?", oldName).Updates(map[string]interface{}{
			"group":      name,
			"updated_at": time.Now(),
		}).Error; err != nil {
			return err
		}
		group.Name = name
		return nil
	})
	return group, err
}

func DeleteServerGroup(id uint) error {
	db := dbcore.GetDBInstance()
	return db.Transaction(func(tx *gorm.DB) error {
		var group models.ServerGroup
		if err := tx.First(&group, id).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Client{}).Where("`group` = ?", group.Name).Updates(map[string]interface{}{
			"group":      "",
			"updated_at": time.Now(),
		}).Error; err != nil {
			return err
		}
		return tx.Delete(&group).Error
	})
}

func OrderServerGroups(weights map[uint]int) error {
	db := dbcore.GetDBInstance()
	return db.Transaction(func(tx *gorm.DB) error {
		for id, weight := range weights {
			result := tx.Model(&models.ServerGroup{}).Where("id = ?", id).Update("weight", weight)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return fmt.Errorf("server group %d not found", id)
			}
		}
		return nil
	})
}

func ServerGroupExists(rawName string) (bool, error) {
	name := strings.TrimSpace(rawName)
	if name == "" {
		return true, nil
	}
	var count int64
	err := dbcore.GetDBInstance().Model(&models.ServerGroup{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}
