package eth

import (
	"encoding/hex"
	"log"
	"math/big"
	"time"

	"github.com/shopspring/decimal"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/wallet"
)

const (
	From = "0xBF5e18bCdA7e9189B92EF17a5dd7E7e4767dBc36"
	To   = "0x23AC5dEDa8a5C6D9b4721b05E7882bE718E5C07d"

	FromPrivateKeys = "0xe5127005b33e35e669aec6f463adc2c1eac00710bdfc299e0c0f46f786c5197a"
	ToPrivateKeys   = "0x73fbad0ba67e2e45f92956a834f7c5dcb95f3450cc07bd52aa78e50b24954635"
)

type Client struct {
	*jsonrpc.Eth
}

func (c *Client) SendTransaction(amount decimal.Decimal) (ethgo.Hash, error) {
	// 获取 gas
	gasPrice, err := c.GasPrice()
	if err != nil {
		log.Fatalf("get gas price failed: err=%v", err)
	}
	log.Printf("gasPrice=%d", gasPrice)

	fromAddr := convertAddress(From)
	toAddr := convertAddress(To)

	gas, err := c.EstimateGas(&ethgo.CallMsg{
		From:     fromAddr,
		To:       &toAddr,
		Data:     nil,
		GasPrice: gasPrice,
		Gas:      nil,
		Value:    new(big.Int).SetUint64(1),
	})
	if err != nil {
		log.Printf("estimate gas failed: err=%v", err)
		return ethgo.Hash{}, err
	}
	log.Printf("gas=%d", gas)

	// 获取 nonce
	nonce, err := c.GetNonce(fromAddr, ethgo.Latest)
	if err != nil {
		log.Printf("get nonce failed: err=%v", err)
		return ethgo.Hash{}, err
	}
	log.Printf("nonce=%d", nonce)

	// 获取 chainID
	chainID, err := c.ChainID()
	if err != nil {
		log.Printf("get chain id failed: err=%v", err)
		return ethgo.Hash{}, err
	}
	log.Printf("chainID=%d", chainID)

	// 构造交易
	wei := ethToWei(amount)
	txn := &ethgo.Transaction{
		Type:     ethgo.TransactionLegacy,
		From:     fromAddr,
		To:       &toAddr,
		GasPrice: gasPrice,
		Gas:      gas + 10,
		Value:    wei,
		Nonce:    nonce,
		ChainID:  chainID,
	}

	// 构造签名
	key, err := wallet.NewWalletFromPrivKey(convertPrivateKey(FromPrivateKeys))
	if err != nil {
		log.Printf("new wallet from private key failed: err=%v", err)
		return ethgo.Hash{}, err
	}

	// 签名
	signer := wallet.NewEIP155Signer(chainID.Uint64())
	signedTxn, err := signer.SignTx(txn, key)
	if err != nil {
		log.Printf("sign transaction failed: err=%v", err)
		return ethgo.Hash{}, err
	}

	// 编码
	txnRaw, err := signedTxn.MarshalRLPTo(nil)
	if err != nil {
		log.Printf("marshal rlp failed: err=%v", err)
		return ethgo.Hash{}, err
	}

	// 发送交易
	hash, err := c.SendRawTransaction(txnRaw)
	if err != nil {
		log.Printf("send raw transaction failed: err=%v", err)
		return ethgo.Hash{}, err
	}
	log.Printf("hash=%s", hash.String())

	return hash, nil
}

func (c *Client) PrintBalance() {
	// 获取地址
	accounts, err := c.Accounts()
	if err != nil {
		log.Fatalf("get accounts failed: err=%v", err)
	}

	log.Printf("accounts=%+v", accounts)

	for _, account := range accounts {
		// 获取余额
		balance, err := c.GetBalance(account, ethgo.Latest)
		if err != nil {
			return
		}
		log.Println()
		log.Println()
		log.Printf("account=%s ", account.String())
		printfBalance(balance)
		log.Println()
		log.Println()
	}
}

func Init() *Client {
	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	for range ticker.C {
		client, err := jsonrpc.NewClient("http://task-ganache:8545")
		// client, err := jsonrpc.NewClient("http://localhost:8545")
		if err != nil {
			log.Printf("new client err: %v", err)
			continue
		}
		version, err := client.Web3().ClientVersion()
		if err != nil {
			log.Printf("get client version err: %v", err)
			continue
		}
		log.Println(version)
		return &Client{client.Eth()}
	}

	panic("failed to connect to eth")
}

// convertAddress 转换地址
func convertAddress(addressHex string) ethgo.Address {
	// 去除前缀0x
	trimmedAddress := addressHex[2:]

	// 将十六进制字符串转换为字节切片
	addressBytes, err := hex.DecodeString(trimmedAddress)
	if err != nil {
		panic(err)
	}

	// 将字节切片转换为[20]byte
	var address [20]byte
	copy(address[:], addressBytes[:20])

	return address
}

// convertPrivateKey 转换私钥
func convertPrivateKey(privateKeyHex string) []byte {
	// 去除前缀0x
	truncatedPrivateKey := privateKeyHex[2:]

	// 将十六进制字符串转换为字节切片
	privateKeyBytes, err := hex.DecodeString(truncatedPrivateKey)
	if err != nil {
		panic(err)
	}

	return privateKeyBytes
}

// printfBalance 打印余额
func printfBalance(balance *big.Int) {
	// 以太坊中 1 ether = 1e18 wei
	ether := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(1e18))

	log.Printf("Balance in Wei: %s", balance) // 余额以wei表示
	log.Printf("Balance in Ether: %f", ether) // 余额以ether表示
}

// EthToWei 将 Eth 转换为 Wei
func ethToWei(amountInEth decimal.Decimal) *big.Int {
	weiMultiplier := decimal.NewFromInt(1e18) // 1 ether = 1e18 wei
	amountInWei := amountInEth.Mul(weiMultiplier)

	// 将 decimal.Decimal 转换为 *big.Int
	weiBigInt, _ := new(big.Int).SetString(amountInWei.String(), 10)
	return weiBigInt
}
