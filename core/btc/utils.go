package btc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/coming-chat/wallet-SDK/core/base"
	"github.com/coming-chat/wallet-SDK/core/eth"
	"github.com/coming-chat/wallet-SDK/pkg/httpUtil"
)

// 根据地址查余额
func QueryBalance(address, chainnet string) (string, error) {
	host, err := scanHostOf(chainnet)
	if err != nil {
		return "0", err
	}
	url := host + "/address/" + address

	response, err := httpUtil.Request(http.MethodGet, url, nil, nil)
	if err != nil {
		return "0", base.MapAnyToBasicError(err)
	}

	return parseBalanceResponse(response)
}

// 根据公钥查余额
func QueryBalancePubkey(pubkey, chainnet string) (string, error) {
	pubkey = strings.TrimPrefix(pubkey, "0x")
	host, err := scanHostOf(chainnet)
	if err != nil {
		return "0", err
	}
	url := host + "/pubkey/" + pubkey

	response, err := httpUtil.Request(http.MethodGet, url, nil, nil)
	if err != nil {
		return "0", base.MapAnyToBasicError(err)
	}

	return parseBalanceResponse(response)
}

func parseBalanceResponse(response *httpUtil.Res) (string, error) {
	if response.Code != http.StatusOK {
		return "0", fmt.Errorf("code: %d, body: %s", response.Code, string(response.Body))
	}
	respDict := make(map[string]interface{})
	err := json.Unmarshal(response.Body, &respDict)
	if err != nil {
		return "0", err
	}

	chainStats, ok := respDict["chain_stats"].(map[string]interface{})
	if !ok {
		return "0", ErrHttpResponseParse
	}
	funded, ok1 := chainStats["funded_txo_sum"].(float64)
	spend, ok2 := chainStats["spent_txo_sum"].(float64)
	if !ok1 || !ok2 {
		return "0", ErrHttpResponseParse
	}

	balance := int64(math.Max(0, funded-spend))
	return strconv.FormatInt(balance, 10), nil
}

// 对交易进行广播
// @param txHex 签名的tx
// @return 交易 hash
func SendRawTransaction(txHex string, chainnet string) (string, error) {
	client, err := rpcClientOf(chainnet)
	if err != nil {
		return "", err
	}

	tx, err := decodeTx(txHex)
	if err != nil {
		return "", err
	}

	hash, err := client.SendRawTransaction(tx, false)
	if err != nil {
		return "", base.MapAnyToBasicError(err)
	}

	return hash.String(), nil
}

func decodeTx(txHex string) (*wire.MsgTx, error) {
	tx := wire.NewMsgTx(wire.TxVersion)
	raw, err := hex.DecodeString(txHex)
	if err != nil {
		return nil, err
	}
	if err = tx.Deserialize(bytes.NewReader(raw)); err != nil {
		return nil, err
	}
	return tx, nil
}

// 通过交易 hash，获取 btc 交易详情
// 注意：btc 的输入解析很复杂且网络代价比较大，因此只能查询到状态和时间
func FetchTransactionDetail(hashString, chainnet string) (*eth.TransactionDetail, error) {
	client, err := rpcClientOf(chainnet)
	if err != nil {
		return nil, err
	}

	hash, err := chainhash.NewHashFromStr(hashString)
	if err != nil {
		return nil, err
	}

	rawResult, err := client.GetRawTransactionVerbose(hash)
	if err != nil {
		return nil, base.MapAnyToBasicError(err)
	}

	status := eth.TransactionStatusPending
	if rawResult.Confirmations > 0 {
		status = eth.TransactionStatusSuccess
	}
	return &eth.TransactionDetail{
		HashString:      hashString,
		Status:          status,
		FinishTimestamp: rawResult.Time,
	}, nil
}

// 获取交易的状态
// @param hashString 交易的 hash
func FetchTransactionStatus(hashString string, chainnet string) eth.TransactionStatus {
	detail, err := FetchTransactionDetail(hashString, chainnet)
	if err != nil {
		return eth.TransactionStatusNone
	}
	return detail.Status
}

// SDK 批量获取交易的转账状态，hash 列表和返回值，都只能用字符串，逗号隔开传递
// @param hashListString 要批量查询的交易的 hash，用逗号拼接的字符串："hash1,hash2,hash3"
// @return 批量的交易状态，它的顺序和 hashListString 是保持一致的: "status1,status2,status3"
func SdkBatchTransactionStatus(hashListString string, chainnet string) string {
	hashList := strings.Split(hashListString, ",")
	statuses, _ := eth.MapListConcurrentStringToString(hashList, func(s string) (string, error) {
		return strconv.Itoa(FetchTransactionStatus(s, chainnet)), nil
	})
	return strings.Join(statuses, ",")
}
