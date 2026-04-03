package models

type ServiceAlias struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Canonical string `gorm:"index;not null;check:canonical <> ''" json:"canonical"`
	Alias     string `gorm:"uniqueIndex;not null;check:alias <> ''" json:"alias"`
}
