package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

func transferETH(client *ethclient.Client, fromPrivKey *ecdsa.PrivateKey, to common.Address, amount *big.Int) error {
	ctx := context.Background()

	// fromPrivKey 是付款方私钥。这里先从私钥推导出公钥，再推导出付款方地址。
	// 这笔交易最终会消耗付款方余额：转账金额 amount + gas 费用。
	publicKey := fromPrivKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("error casting public key to ECDSA")
	}

	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
	// 查询发送账户当前的待处理交易 nonce，防止重放攻击
	nonce, err := client.PendingNonceAt(ctx, fromAddress)
	if err != nil {
		return err
	}

	// ETH 转账的标准 gas 上限为 21000
	gasLimit := uint64(21000)
	// 从节点获取当前市场建议的 gas 价格
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return err
	}

	// 创建交易对象：nonce, to, value, gasLimit, gasPrice, data(nil)
	// 这是普通 ETH 转账交易，data 为 nil，不调用智能合约。
	tx := types.NewTransaction(nonce, to, amount, gasLimit, gasPrice, nil)
	// 链 ID：1337 是本地开发链（Ganache）的默认 ID
	chainID := big.NewInt(1337)
	// 使用 EIP155 签名规范对交易进行签名（包含 chainID 防止跨链重放攻击）
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), fromPrivKey)
	if err != nil {
		return err
	}

	// SendTransaction 会把签名后的交易提交到本地 Ethereum 节点。
	// 交易被打包后，付款方余额变化会持久化在本地链状态里。
	return client.SendTransaction(ctx, signedTx)

}
