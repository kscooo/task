package model

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// WithdrawalState 交易状态
type WithdrawalState uint8

const (
	StateUnchained WithdrawalState = iota // 未上链
	StatePending                          // 上链中
	StateSuccess                          // 上链成功
	StateFailure                          // 上链失败
	StateException                        // 其他异常情况
)

// Withdrawal 提款申请
type Withdrawal struct {
	ID        uint            `gorm:"primary_key" json:"id,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	Amount    decimal.Decimal `gorm:"not null" json:"amount"`            // 提款金额
	TxHash    string          `gorm:"not null" json:"tx_hash,omitempty"` // 交易哈希
	Status    uint64          `gorm:"not null" json:"status,omitempty"`  // 状态 0: 未上链 1: 上链中 2: 上链成功 3: 上链失败 4: 其他异常情况
}

// WithdrawalConfirmation 提款申请确认
type WithdrawalConfirmation struct {
	ID           uint      `gorm:"primary_key" json:"id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	WithdrawalID uint64    `gorm:"not null;uniqueIndex:idx_withdrawal_manager" json:"withdrawal_id,omitempty"` // 提款申请 ID
	ManagerID    uint64    `gorm:"not null;uniqueIndex:idx_withdrawal_manager" json:"manager_id,omitempty"`    // 经理 ID
}

func Init() *gorm.DB {
	dsn := "host=task-postgres user=gorm password=gorm dbname=gorm port=5432 sslmode=disable TimeZone=Asia/Shanghai"
	// dsn := "host=localhost user=gorm password=gorm dbname=gorm port=5432 sslmode=disable TimeZone=Asia/Shanghai"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	// Migrate the schema
	err = db.AutoMigrate(
		&Withdrawal{},
		&WithdrawalConfirmation{},
	)
	if err != nil {
		panic(err)
	}

	return db
}
