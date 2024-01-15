package main

import (
	"log"
	"testing"

	"task/cmd/app/eth"
	"task/cmd/app/model"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func initTestData(db *gorm.DB) *model.Withdrawal {
	withdrawal := model.Withdrawal{
		Amount: decimal.NewFromFloat(1),
		TxHash: "",
		Status: 0,
	}
	err := db.Create(&withdrawal).Error
	if err != nil {
		panic(err)
	}

	err = db.Create(&model.WithdrawalConfirmation{
		WithdrawalID: uint64(withdrawal.ID),
		ManagerID:    1,
	}).Error
	if err != nil {
		panic(err)
	}

	err = db.Create(&model.WithdrawalConfirmation{
		WithdrawalID: uint64(withdrawal.ID),
		ManagerID:    2,
	}).Error
	if err != nil {
		panic(err)
	}

	return &withdrawal
}

func TestStateMachine(t *testing.T) {
	db := model.Init()
	client := eth.Init()
	testData := initTestData(db)

	s := NewStateMachine(testData, client, db)
	log.Printf("----------before execute")
	client.PrintBalance()
	log.Printf("----------before execute\n\n\n")
	s.Execute()
	log.Printf("----------after execute")
	client.PrintBalance()
	log.Printf("----------after execute\n\n\n")
	// cleanup(db, testData)
}

func cleanup(db *gorm.DB, testData *model.Withdrawal) {
	err := db.Delete(testData).Error
	if err != nil {
		panic(err)
	}

	err = db.Delete(&model.WithdrawalConfirmation{}, "withdrawal_id = ?", testData.ID).Error
	if err != nil {
		panic(err)
	}
}
