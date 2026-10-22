package models

// Subscription stores a recurring membership or service subscription.
type Subscription struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Name         string    `json:"name" gorm:"type:varchar(100);not null"`
	Plan         string    `json:"plan" gorm:"type:varchar(100)"`
	Price        float64   `json:"price" gorm:"type:decimal(12,2);not null"`
	Currency     string    `json:"currency" gorm:"type:varchar(8);not null"`
	BillingCycle int       `json:"billing_cycle" gorm:"type:int;not null"`
	ExpiresAt    LocalTime `json:"expires_at" gorm:"type:timestamp;index"`
	AutoRenewal  bool      `json:"auto_renewal" gorm:"default:false"`
	Visible      bool      `json:"visible" gorm:"index"`
	ShowPrice    bool      `json:"show_price"`
	URL          string    `json:"url" gorm:"type:varchar(500)"`
	Remark       string    `json:"remark" gorm:"type:varchar(500)"`
	Weight       int       `json:"weight" gorm:"type:int;index"`
	CreatedAt    LocalTime `json:"created_at"`
	UpdatedAt    LocalTime `json:"updated_at"`
}
