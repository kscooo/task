package main

import (
	"log"
	"net/http"
	"time"

	"task/cmd/app/eth"
	"task/cmd/app/model"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/umbracle/ethgo"
	"gorm.io/gorm"
)

type WithdrawalRequest struct {
	Amount string `json:"amount"`
}

type WithdrawalApproveRequest struct {
	ManagerID uint64 `json:"manager_id"`
}

// TransactionState 交易状态
type TransactionState uint8

const (
	StateUnchained TransactionState = iota // 未上链
	StatePending                           // 上链中
	StateSuccess                           // 上链成功
	StateFailure                           // 上链失败
	StateException                         // 其他异常情况
)

// TransactionEvent 触发状态转换的事件
type TransactionEvent uint8

const (
	EventStart             TransactionEvent = iota // 开始处理
	EventCheck                                     // 检查状态
	EventRetry                                     // 重试事件
	EventMaxRetriesReached                         // 达到最大重试次数
	EventSuccess                                   // 上链成功
)

type StateMachine struct {
	state         TransactionState
	retries       int
	maxRetries    int
	retryInterval time.Duration
	withdrawal    *model.Withdrawal
	client        *eth.Client
	tx            *gorm.DB
}

func NewStateMachine(
	withdrawal *model.Withdrawal,
	client *eth.Client,
	tx *gorm.DB,
) *StateMachine {
	return &StateMachine{
		state:         StateUnchained,
		retries:       0,
		maxRetries:    3,
		retryInterval: time.Second * 1,
		withdrawal:    withdrawal,
		client:        client,
		tx:            tx,
	}
}

func (sm *StateMachine) Execute() {
	sm.next(EventStart)
}

// 如果没有 tx hash，则是未上链，则发起上链请求，并保存 tx hash
// 根据 tx hash 查询 receipt，根据 receipt 状态更新提款申请状态
// 总重试的次数为 3 次，每次间隔 1s
//	  a) 如果获取不到 receipt，说明上链中，更新提款申请状态为上链中，间隔 1s 重新查询 receipt。
//       重试，receipt 仍然获取不到，则打印日志，状态为上链中。
//	  b) 如果 receipt 状态为成功, 则更新提款申请状态为上链成功。
//	  c) 如果 receipt 状态为失败, 打印日志，间隔 1s 重新发起上链请求。
//	     重试，receipt 仍然失败，则打印日志，状态为上链失败。
//    d) 如果 receipt 状态为其他异常情况，打印日志，间隔 1s 重新发起上链请求。
//	     重试，receipt 仍然失败，则打印日志，状态为其他异常情况。
// 流程图：img.png
func (sm *StateMachine) next(event TransactionEvent) {
	switch event {
	case EventStart:
		// 发起上链请求，失败则重试
		hash, err := sm.client.SendTransaction(sm.withdrawal.Amount)
		if err != nil {
			log.Printf("send transaction failed: err=%v", err)
			sm.state = StateUnchained
			sm.next(EventRetry)
			return
		}

		sm.state = StatePending
		sm.saveHashAndStatus(hash.String(), model.StatePending)
		sm.next(EventCheck)
	case EventCheck:
		// 查询 receipt
		receipt, err := sm.client.GetTransactionReceipt(ethgo.HexToHash(sm.withdrawal.TxHash))
		if err != nil {
			log.Printf("get transaction receipt failed: err=%v", err)
			sm.state = StatePending
			sm.next(EventRetry)
			return
		}
		// mock pending
		// receipt = nil
		if receipt == nil {
			sm.state = StatePending
			sm.next(EventRetry)
			return
		}

		// mock failure
		// receipt.Status = 0
		// mock exception
		// receipt.Status = 2

		if receipt.Status == 1 {
			sm.state = StateSuccess
			sm.updateWithdrawalStatus(model.StateSuccess)
			sm.next(EventSuccess)
		} else if receipt.Status == 0 {
			sm.state = StateFailure
			sm.updateWithdrawalStatus(model.StateFailure)
			sm.next(EventRetry)
		} else {
			sm.state = StateException
			sm.updateWithdrawalStatus(model.StateException)
			sm.next(EventRetry)
		}
	case EventRetry:
		if sm.retries >= sm.maxRetries {
			sm.next(EventMaxRetriesReached)
			return
		}
		sm.retries++
		time.Sleep(sm.retryInterval)
		// 重试，根据状态跳转
		if sm.state == StatePending {
			sm.next(EventCheck)
		} else {
			sm.next(EventStart)
		}
	case EventMaxRetriesReached:
		log.Printf("max retries reached: retries=%d, state=%d", sm.retries, sm.state)
		if sm.state == StateUnchained {
			sm.updateWithdrawalStatus(model.StateException)
		}
	case EventSuccess:
		log.Printf("success: retries=%d, state=%d", sm.retries, sm.state)
	default:
		log.Printf("invalid event: event=%d", event)
	}
}

func (sm *StateMachine) updateWithdrawalStatus(status model.WithdrawalState) {
	sm.withdrawal.Status = uint64(status)
	err := sm.tx.Save(sm.withdrawal).Error
	if err != nil {
		log.Printf("update withdrawal status failed: err=%v", err)
		return
	}
}

func (sm *StateMachine) saveHashAndStatus(hash string, status model.WithdrawalState) {
	sm.withdrawal.TxHash = hash
	sm.withdrawal.Status = uint64(status)
	err := sm.tx.Save(sm.withdrawal).Error
	if err != nil {
		log.Printf("update withdrawal tx hash and status failed: err=%v", err)
		return
	}
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

		// 达到两个时自动执行提款
		// 开启事务，事务隔离级别为最高档 serializable
		tx := db.Begin()

		// 插入审批记录
		withdrawalConfirmation := &model.WithdrawalConfirmation{
			WithdrawalID: uint64(withdrawal.ID),
			ManagerID:    mangerID,
		}

		err = tx.Create(withdrawalConfirmation).Error
		if err != nil {
			tx.Rollback()
			log.Printf("create withdrawal confirmation failed: err=%v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "create withdrawal confirmation failed",
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

		// 少于两个审批记录，直接返回
		if count < 2 {
			tx.Commit()
			c.JSON(http.StatusOK, gin.H{
				"message": "success",
			})
			return
		}

		// 两个以上的审批记录，自动执行提款
		// 封装状态机，自动执行提款流程
		// 如果有 tx hash，则不做任何操作，直接返回。
		err = tx.Where("id = ?", requestID).First(&withdrawal).Error
		if err != nil {
			tx.Rollback()
			log.Printf("find withdrawal failed: err=%v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "find withdrawal failed",
			})
			return
		}

		// 检查状态是否符合预期
		if withdrawal.TxHash != "" {
			if withdrawal.Status != 0 {
				log.Printf("invalid status: status=%d", withdrawal.Status)
			}
			tx.Commit()
			c.JSON(http.StatusOK, gin.H{
				"message": "success",
			})
			return
		}

		if !(withdrawal.TxHash == "" && withdrawal.Status == 0) {
			log.Printf("invalid status: tx_hash=%s, status=%d", withdrawal.TxHash, withdrawal.Status)
			tx.Commit()
			c.JSON(http.StatusOK, gin.H{
				"message": "success",
			})
			return
		}

		// 执行提款
		sm := NewStateMachine(&withdrawal, client, tx)
		sm.Execute()

		tx.Commit()
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

		// 少于两个审批记录，不允许提款
		if count < 2 {
			tx.Rollback()
			log.Printf("invalid request: count=%d", count)
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "manager count less than 2",
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

		// 1) 如果没有 tx hash/上链失败，则（重新）发起上链请求，并保存 tx hash，根据 tx hash 查询 receipt
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
