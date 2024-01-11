package main

import (
	"log"
	"net/http"

	"task/cmd/app/eth"
	"task/cmd/app/model"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/umbracle/ethgo"
)

type WithdrawalRequest struct {
	Amount string `json:"amount"`
}

type WithdrawalApproveRequest struct {
	ManagerID uint64 `json:"manager_id"`
}

func main() {
	client := eth.Init()

	// // 随机生成私钥
	// key, err := wallet.GenerateKey()
	// if err != nil {
	// 	log.Fatalf("generate key failed: err=%v", err)
	// }
	//
	// privateKey, err := key.MarshallPrivateKey()
	// if err != nil {
	// 	log.Fatalf("marshall private key failed: err=%v", err)
	// }
	//
	// privateKeyHex := hex.EncodeToString(privateKey)
	// log.Printf("Private Key in Hex: 0x%s", privateKeyHex)

	// // 发起转账、查询交易 hash
	// hash, err := client.SendTransaction(decimal.NewFromInt(1))
	// if err != nil {
	// 	log.Fatalf("send transaction failed: err=%v", err)
	// }
	//
	// receipt, err := client.GetTransactionReceipt(hash)
	// if err != nil {
	// 	log.Fatalf("get transaction receipt failed: err=%v", err)
	// }
	// log.Printf("receipt=%+v", receipt)
	//
	// transaction, err := client.GetTransactionByHash(hash)
	// if err != nil {
	// 	log.Fatalf("get transaction by hash failed: err=%v", err)
	// }
	// log.Printf("transaction=%+v", transaction)

	db := model.Init()

	r := gin.Default()
	// 创建提款申请 (POST /withdrawal/create)
	r.POST("/withdrawal/create", func(c *gin.Context) {
		// 获取参数
		req := &WithdrawalRequest{}
		err := c.BindJSON(req)
		if err != nil {
			log.Printf("invalid request: params: %+v, err: %v", c.Params, err)
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "invalid request",
			})
			return
		}

		amount, err := decimal.NewFromString(req.Amount)
		if err != nil {
			log.Printf("invalid request: params=%+v, err=%v", c.Params, err)
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "invalid request",
			})
			return
		}

		// 校验参数，必须大于 0
		if amount.LessThanOrEqual(decimal.Zero) {
			log.Printf("invalid request: params=%+v, err=%v", c.Params, err)
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "invalid request",
			})
			return
		}

		log.Printf("req=%+v", req)
		// 创建入库
		withdrawal := &model.Withdrawal{
			Amount: amount,
		}

		err = db.Create(withdrawal).Error
		if err != nil {
			log.Printf("create withdrawal failed: err=%v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "create withdrawal failed",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":    "success",
			"request_id": withdrawal.ID,
		})
	})

	// 检索提款申请状态 (GET /withdrawal/status/{request_id})
	r.GET("/withdrawal/status/:request_id", func(c *gin.Context) {
		requestID := c.Param("request_id")
		log.Printf("requestID=%s", requestID)

		// 传 0 则查询所有
		var withdrawals []*model.Withdrawal
		if requestID == "0" {
			err := db.Find(&withdrawals).Error
			if err != nil {
				log.Printf("find withdrawal failed: err=%v", err)
				c.JSON(http.StatusInternalServerError, gin.H{
					"message": "find withdrawal failed",
				})
				return
			}
		} else {
			err := db.Where("id = ?", requestID).Find(&withdrawals).Error
			if err != nil {
				log.Printf("find withdrawal failed: err=%v", err)
				c.JSON(http.StatusInternalServerError, gin.H{
					"message": "find withdrawal failed",
				})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"withdrawals": withdrawals,
		})
	})

	// 经理审批提款申请 (POST /withdrawal/approve/{request_id})
	r.POST("/withdrawal/approve/:request_id", func(c *gin.Context) {
		req := &WithdrawalApproveRequest{}
		err := c.ShouldBindJSON(req)
		if err != nil {
			log.Printf("invalid request: params=%+v, err=%v", c.Params, err)
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "invalid request",
			})
			return
		}
		requestID := c.Param("request_id")
		mangerID := req.ManagerID

		log.Printf("requestID=%s, mangerID=%d", requestID, mangerID)
		if requestID == "" || mangerID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "invalid request",
			})
			return
		}

		// 查询是否存在
		var withdrawal model.Withdrawal
		err = db.Where("id = ?", requestID).First(&withdrawal).Error
		if err != nil {
			log.Printf("find withdrawal failed: err=%v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "find withdrawal failed",
			})
			return
		}

		// 插入审批记录
		withdrawalConfirmation := &model.WithdrawalConfirmation{
			WithdrawalID: uint64(withdrawal.ID),
			ManagerID:    mangerID,
		}

		err = db.Create(withdrawalConfirmation).Error
		if err != nil {
			log.Printf("create withdrawal confirmation failed: err=%v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "create withdrawal confirmation failed",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "success",
		})
	})

	// 执行提款 (POST /withdrawal/execute/{request_id})
	r.POST("/withdrawal/execute/:request_id", func(c *gin.Context) {
		// 每次打印余额
		defer func() {
			client.PrintBalance()
		}()

		// 获取参数
		requestID := c.Param("request_id")
		log.Printf("requestID=%s", requestID)
		if requestID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "invalid request",
			})
			return
		}

		tx := db.Begin()
		// 查询是否存在，且状态不是已上链的
		var withdrawal model.Withdrawal
		err := tx.Where("id = ?", requestID).
			Where("status != ?", 2).
			First(&withdrawal).Error
		if err != nil {
			tx.Rollback()
			log.Printf("find withdrawal failed: err=%v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "find withdrawal failed",
			})
			return
		}

		// 查询是否有俩个以上的审批记录
		var count int64
		err = tx.Model(&model.WithdrawalConfirmation{}).
			Where("withdrawal_id = ?", withdrawal.ID).
			Count(&count).
			Error
		if err != nil {
			tx.Rollback()
			log.Printf("count withdrawal confirmation failed: err=%v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "count withdrawal confirmation failed",
			})
			return
		}

		if count < 2 {
			tx.Rollback()
			log.Printf("invalid request: count=%d", count)
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "invalid request",
			})
			return
		}

		// 0) 查看是否有 tx hash，如果有，查询 receipt 是否成功
		//      a) 如果 receipt 获取不到，说明上链中，不做任何操作，更新提款申请状态为上链中
		//      b) 如果 receipt 状态为成功, 则更新提款申请状态为上链成功
		//      c) 如果 receipt 状态为失败, 则重新发起上链请求
		//      d) 如果 receipt 状态为其他异常情况，打印日志，清除 tx hash, 状态变更为未上链
		if withdrawal.TxHash != "" {
			receipt, err := client.GetTransactionReceipt(ethgo.HexToHash(withdrawal.TxHash))
			if err != nil {
				tx.Rollback()
				log.Printf("get transaction receipt failed: err=%v", err)
				c.JSON(http.StatusInternalServerError, gin.H{
					"message": "get transaction receipt failed",
				})
				return
			}

			// 更新提款申请状态为上链中
			if receipt == nil {
				if withdrawal.Status != 1 {
					withdrawal.Status = 1
					err = tx.Save(&withdrawal).Error
					if err != nil {
						tx.Rollback()
						log.Printf("update withdrawal status failed: err=%v", err)
						c.JSON(http.StatusInternalServerError, gin.H{
							"message": "update withdrawal status failed",
						})
						return
					}
				}

				tx.Commit()
				c.JSON(http.StatusOK, gin.H{
					"message": "success",
				})
				return
			}

			// 上链成功
			if receipt.Status == 1 {
				withdrawal.Status = 2
				err = tx.Save(&withdrawal).Error
				if err != nil {
					tx.Rollback()
					log.Printf("update withdrawal status failed: err=%v", err)
					c.JSON(http.StatusInternalServerError, gin.H{
						"message": "update withdrawal status failed",
					})
					return
				}

				tx.Commit()
				c.JSON(http.StatusOK, gin.H{
					"message": "success",
				})
				return
			}

			// 上链失败，重新发起上链请求
			if receipt.Status == 0 {
				goto sendTransaction
			}

			// 其他异常情况，打印日志，清除 tx hash, 状态变更为未上链
			log.Printf("invalid receipt status: status=%d", receipt.Status)
			withdrawal.TxHash = ""
			withdrawal.Status = 0
			err = tx.Save(&withdrawal).Error
			tx.Commit()

			c.JSON(http.StatusOK, gin.H{
				"message": "success",
			})
			return
		}

		// 1) 如果没有 tx hash/或者是上链失败，则（重新）发起上链请求，并保存 tx hash，根据 tx hash 查询 receipt
		//	  a) 如果获取不到 receipt，说明上链中，不做任何操作，更新提款申请状态为上链中
		//	  b) 如果 receipt 状态为成功, 则更新提款申请状态为上链成功
		//	  c) 如果 receipt 状态为失败, 则更新提款申请状态为上链失败
		//    d) 如果 receipt 状态为其他异常情况，打印日志，清除 tx hash, 状态变更为未上链
	sendTransaction:
		// 发起上链请求
		hash, err := client.SendTransaction(withdrawal.Amount)
		if err != nil {
			tx.Rollback()
			log.Printf("send transaction failed: err=%v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "send transaction failed",
			})
			return
		}

		// 保存 tx hash
		withdrawal.TxHash = hash.String()
		// 更新提款申请状态为上链中
		withdrawal.Status = 1
		err = tx.Save(&withdrawal).Error
		if err != nil {
			tx.Rollback()
			log.Printf("update withdrawal tx hash failed: err=%v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "update withdrawal tx hash failed",
			})
			return
		}

		// 查询 receipt
		receipt, err := client.GetTransactionReceipt(ethgo.HexToHash(withdrawal.TxHash))
		if err != nil {
			tx.Rollback()
			log.Printf("get transaction receipt failed: err=%v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "get transaction receipt failed",
			})
			return
		}

		if receipt == nil {
			log.Printf("receipt is nil, withdrawal=%+v", withdrawal)
			tx.Commit()
			c.JSON(http.StatusOK, gin.H{
				"message": "success",
			})
			return
		}

		// 上链成功
		if receipt.Status == 1 {
			withdrawal.Status = 2
			err = tx.Save(&withdrawal).Error
			if err != nil {
				tx.Rollback()
				log.Printf("update withdrawal status failed: err=%v", err)
				c.JSON(http.StatusInternalServerError, gin.H{
					"message": "update withdrawal status failed",
				})
				return
			}

			tx.Commit()
			c.JSON(http.StatusOK, gin.H{
				"message": "success",
			})
			return
		}

		// 上链失败
		if receipt.Status == 0 {
			withdrawal.Status = 3
			err = tx.Save(&withdrawal).Error
			if err != nil {
				tx.Rollback()
				log.Printf("update withdrawal status failed: err=%v", err)
				c.JSON(http.StatusInternalServerError, gin.H{
					"message": "update withdrawal status failed",
				})
				return
			}
		}

		// 其他异常情况，打印日志，清除 tx hash, 状态变更为未上链
		log.Printf("invalid receipt status: status=%d", receipt.Status)
		withdrawal.TxHash = ""
		withdrawal.Status = 0
		err = tx.Save(&withdrawal).Error
		tx.Commit()

		c.JSON(http.StatusOK, gin.H{
			"message": "success",
		})
		return
	})

	r.Run()
}
