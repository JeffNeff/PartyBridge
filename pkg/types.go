package be

import (
	"context"
	"math/big"
	"sync"
	"time"

	"crypto/ecdsa"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-redis/redis/v9"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	pkgadapter "knative.dev/eventing/pkg/adapter/v2"
)

const (
	GRAMS   = "grams"
	OCTA    = "octa"
	BSCUSDT = "bscusdt"

	WBSCUSDT = "wbscusdt"
	WGRAMS   = "wgrams"
	WOCTA    = "wocta"
)

type HelloMsg struct {
	Type          string `json:"type"`
	SID           string `json:"sid"`
	Fee           int    `json:"fee"`
	MinimumAmount int    `json:"minimumAmount"`
}

type RequestBridgeMsg struct {
	Type string        `json:"type"`
	Data BridgeRequest `json:"data,omitempty"`
}

type RequestBridgeResponseMsg struct {
	Type    string   `json:"type"`
	Amount  *big.Int `json:"amount"`
	Address string   `json:"address"`
}

type Token struct {
	Total      int    `json:"total"`
	PageNumber int    `json:"pageNumber"`
	PageSize   int    `json:"pageSize"`
	Cursor     any    `json:"cursor"`
	Network    string `json:"network"`
	Owners     []struct {
		TokenAddress      string `json:"tokenAddress"`
		TokenID           string `json:"tokenId"`
		Amount            string `json:"amount"`
		OwnerOf           string `json:"ownerOf"`
		TokenHash         string `json:"tokenHash"`
		BlockNumberMinted string `json:"blockNumberMinted"`
		BlockNumber       string `json:"blockNumber"`
		ContractType      string `json:"contractType"`
		Name              string `json:"name"`
		Symbol            string `json:"symbol"`
		Metadata          any    `json:"metadata"`
		MinterAddress     string `json:"minterAddress"`
	} `json:"owners"`
}

// ErrorEvent represents the expected information in an emitted error event
// "tea.party.error"| ERROREVENT
type ErrorEvent struct {
	Err     string
	Context string
	Data    interface{}
}

type StatusMsg struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// AccountWatchRequest is the information we need to watch a new account
type AccountWatchRequest struct {
	Seller                       bool                          `json:"seller"`
	Account                      string                        `json:"account"`
	Chain                        string                        `json:"chain"`
	Amount                       *big.Int                      `json:"amount"`
	NFTID                        int64                         `json:"nft_id"`
	TransactionID                string                        `json:"transaction_id"`
	TimeOut                      int64                         `json:"timeout"`
	FinalizeOnChain              bool                          `json:"finalizeOnChain"`
	AssistedSellOrderInformation AssistedTradeOrderInformation `json:"assistedSellOrderInformation"`
	Locked                       bool                          `json:"locked"`
	LockedTime                   time.Time                     `json:"lockedTime"`
	LockedBy                     string                        `json:"lockedBy"`
	AWRID                        string                        `json:"awrid"`
	WSClientID                   string                        `json:"wsClientID"`
	CreatedTime                  time.Time                     `json:"createdTime"`
}

// AccountWatchRequestResult is the result of the watch request
type AccountWatchRequestResult struct {
	AccountWatchRequest AccountWatchRequest `json:"account_watch_request"`
	Result              string              `json:"result"`
}

type envAccessor struct {
	pkgadapter.EnvConfig

	Development bool `envconfig:"DEV" default:"false"`
	Watch       bool `envconfig:"WATCH" default:"true"`

	SMARTCONTRACTPRIVATEKEY string `envconfig:"PRIVATE_KEY" required:"true"`

	PartyChainRPC1 string `envconfig:"PARTY_CHAIN_1" required:"true"`
	PartyChainRPC2 string `envconfig:"PARTY_CHAIN_2" required:"true"`

	OCTARPC1 string `envconfig:"OCTA_RPC_1" default:"" required:"true"`
	OCTARPC2 string `envconfig:"OCTA_RPC_2" default:"" required:"true"`

	// redis server
	RedisAddress  string `envconfig:"REDIS_ADDRESS" required:"true"`
	RedisPassword string `envconfig:"REDIS_PASSWORD" default:""`
	RedisDB       int    `envconfig:"REDIS_DB" default:"0"`

	// Shim Servers
	WGramsShimServerAddress               string `envconfig:"WGRAMS_SHIM_SERVER_ADDRESS" required:"true"`
	WOctaShimServerAddress                string `envconfig:"WOCTA_SHIM_SERVER_ADDRESS" required:"true"`
	WBSCUSDTOnOctaSpaceShimServerAddress  string `envconfig:"WBSCUSDT_OCTA_SPACE_SHIM_SERVER_ADDRESS" required:"true"`
	WBSCUSDTOnPartyChainShimServerAddress string `envconfig:"WBSCUSDT_PARTY_CHAIN_SHIM_SERVER_ADDRESS" required:"true"`

	ShimCertLocation string `envconfig:"SHIM_CA_CERT" required:"true"`

	PodName string `envconfig:"HOSTNAME" required:"true"`

	MinimumAmount int `envconfig:"MINIMUM_AMOUNT" default:"10"`
	Fee           int `envconfig:"FEE" default:"5"`

	WGRAMSOnOCTAContractAddress         string `envconfig:"WGRAMS_ON_OCTA_CONTRACT_ADDRESS" required:"true"`
	WOCTAOnPartyChainContractAddress    string `envconfig:"WOCTA_ON_PARTYCHAIN_CONTRACT_ADDRESS" required:"true"`
	WBSCUSDTOnPartyChainContractAddress string `envconfig:"WBSCUSDT_ON_PARTYCHAIN_CONTRACT_ADDRESS" required:"true"`
	WBSCUSDTOnOCTAContractAddress       string `envconfig:"WBSCUSDT_ON_OCTA_CONTRACT_ADDRESS" required:"true"`

	ServerSSLCRTFilePath string `envconfig:"SERVER_SSL_CRT_FILE_PATH" required:"true"`
	ServerSSLKeyFilePath string `envconfig:"SERVER_SSL_KEY_FILE_PATH" required:"true"`
}

type WebSocketClient struct {
	conn    *websocket.Conn
	sid     string
	acc     *ecdsa.PrivateKey
	send    chan []byte
	request BridgeRequest
}

// ExchangeServer holds the state of the exchange server.
type ExchangeServer struct {
	ctx     context.Context
	podName string

	partyChain                           EthereumNode
	octNode                              EthereumNode
	gramsShimServerAddress               string
	octaShimServerAddress                string
	bscUSDTOnPartyChainShimServerAddress string
	bscUSDTOnOctaSpaceShimServerAddress  string
	shimCertLocation                     string

	redisClient *redis.Client

	warrenChan chan AccountWatchRequest
	warrenWG   *sync.WaitGroup

	ceClient cloudevents.Client
	logger   *zap.SugaredLogger
	dev      bool
	watch    bool

	wsClients map[string]*WebSocketClient

	minimumAmount int
	fee           int

	SSLCRTLocation       string
	ServerSSLKeyFilePath string

	wGRAMSOnOCTAContractAddress         string
	wOCTAOnPartyChainContractAddress    string
	wBSCUSDTOnPartyChainContractAddress string
	wBSCUSDTOnOctaSpaceContractAddress  string

	wsClientsMutex sync.Mutex
}

type EthereumNode struct {
	rpcClient    *ethclient.Client
	rpcClientTwo *ethclient.Client
}

type AccountGenResponse struct {
	PrivateKey string `json:"privateKey"`
	PubKey     string `json:"publicKey"`
	Address    string `json:"address"`
}

// SellOrder contains the information expected in a sell order.
type SellOrder struct {
	// Currency reflects the currency that the SELLER wishes to trade. (bitcoin, mineonlium, USDT, etc).
	Currency string `json:"currency"`
	// Amount reflects the ammount of Currency the SELLER wishes to trade.
	Amount *big.Int `json:"amount"`
	// TradeAsset reflects the asset that the SELLER wishes to obtain. (bitcoin, mineonlium, USDT, etc).
	TradeAsset string `json:"tradeAsset"`
	// Price reflects the ammount of TradeAsset the SELLER requires.
	Price *big.Int `json:"price"`
	// TXID reflects the Transaction ID of the SELL order to be created.
	TXID string `json:"txid"`
	// Locked tells us if this transaction is pending/proccessing another payment.
	Locked bool `json:"locked" default:false`
	// SellerShippingAddress reflects the public key of the account the seller wants to receive on
	SellerShippingAddress string `json:"sellerShippingAddress"`
	// SellerNKNAddress reflects the  public NKN address of the seller.
	SellerNKNAddress string `json:"sellerNKNAddress"`
	// PaymentTransactionID reflects the transaction ID of the payment made in MO.
	PaymentTransactionID string `json:"paymentTransactionID"`
	// RefundAddress reflects the address of which the funds will be refunded in case of a failure.
	RefundAddress string `json:"refundAddress"`
	// Private reflects if the trade order is to be private or not. I.E. listed in the public
	// market place or not.
	Private bool `json:"private"`
	// OnChain reflects if the trade order is to be finalized on-chain or not.
	OnChain bool `json:"onChain"`
	// Assisted reflects if the trade order is to be assisted by the exchange or not.
	Assisted bool `json:"assisted"`
	// AssistedTradeOrderInformation reflects the information required to assist the trade order.
	AssistedTradeOrderInformation AssistedTradeOrderInformation `json:"assistedTradeOrderInformation"`
	// NFTID reflects the NFT ID of the NFT that is being traded.
	NFTID int64 `json:"nftID"`
}

type AssistedTradeOrderInformation struct {
	// SellersEscrowWallet represents the wallet that the seller has already funded
	// with the currency they wish to trade.
	SellersEscrowWallet   EscrowWallet `json:"sellersEscrowWallet"`
	SellerRefundAddress   string       `json:"sellerRefundAddress"`
	SellerShippingAddress string       `json:"sellerShippingAddress"`
	// TradeAsset reflects the asset that the SELLER wishes to obtain. (bitcoin, mineonlium, USDT, etc). Or Bridge.
	TradeAsset string `json:"tradeAsset"`
	// Price reflects the ammount of TradeAsset the SELLER requires.
	Price *big.Int `json:"price"`
	// Currency reflects the currency that the SELLER wishes to trade. (bitcoin, mineonlium, USDT, etc).
	Currency string `json:"currency"`
	// Amount reflects the ammount of Currency the SELLER wishes to trade.
	Amount *big.Int `json:"amount"`
	// BridgeTo reflects the blockchain that the seller wishes to bridge to.
	BridgeTo string `json:"bridgeTo"`
	// BridgeFrom reflects the blockchain that the seller wishes to bridge from.
	BridgeFrom string `json:"bridgeFrom"`
}

// CompletedTransactionInformation represents data expected when
// describing a transaction that has been completed on-chain.
type CompletedTransactionInformation struct {
	// the transaction id of the completed transaction
	TXID string `json:"txid"`
	// the amount of the transaction
	Amount *big.Int `json:"amount"`
	// the blockchain the transaction was completed on
	Blockchain string `json:"blockchain"`
}

type EscrowWallet struct {
	PublicAddress string `json:"publicAddress"`
	PrivateKey    string `json:"privateKey"`
	Chain         string `json:"chain"`
}

// CompletedOrder contains all the required elements to complete an order
type CompletedOrder struct {
	// BuyerEscrowWallet the escrow wallet that the buyer will be inserting the
	// TradeAsset into.
	BuyerEscrowWallet EscrowWallet `json:"buyerEscrowWallet"`
	// SellerEscrowWallet the escrow wallet that the seller will be inserting the
	// Currency into.
	SellerEscrowWallet EscrowWallet `json:"sellerEscrowWallet"`
	// SellerPaymentComplete is a boolean that tells us if the seller has completed
	// the payment.
	SellerPaymentComplete bool `json:"sellerPaymentComplete"`
	// BuyerPaymentComplete is a boolean that tells us if the buyer has completed
	// the payment.
	BuyerPaymentComplete bool `json:"buyerPaymentComplete"`
	// Amount the amount of funds that we are sending to the buyer.
	Amount *big.Int `json:"amount"`
	// OrderID the orderID that we are completing.
	OrderID string `json:"orderID"`
	// BuyerShippingAddress the public key of the account the buyer wants to receive on
	BuyerShippingAddress string `json:"buyerShippingAddress"`
	// BuyerRefundAddress
	BuyerRefundAddress string `json:"buyerRefundAddress"`
	// BuyerToFinalizeOnChain is a boolean that tells us if the buyer has elected
	// to finalize the transaction on-chain.
	BuyerToFinalizeOnChain bool `json:"buyerToFinalizeOnChain"`
	// SellerRefundAddress
	SellerRefundAddress string `json:"sellerRefundAddress"`
	// SellerShippingAddress the public key of the account the seller wants to receive on
	SellerShippingAddress string `json:"sellerShippingAddress"`
	// SellerToFinalizeOnChain is a boolean that tells us if the seller has elected
	// to finalize the transaction on-chain.
	SellerToFinalizeOnChain bool `json:"sellerToFinalizeOnChain"`
	// BuyerNKNAddress the public NKN address of the buyer.
	BuyerNKNAddress string `json:"buyerNKNAddress"`
	// SellerNKNAddress the public NKN address of the seller.
	SellerNKNAddress string `json:"sellerNKNAddress"`
	// TradeAsset is the asset that we are sending to the buyer.
	TradeAsset string `json:"tradeAsset"`
	// Currency the currency that we are sending to the seller.
	Currency string `json:"currency"`
	// Price the price of the trade. (how much of the TradeAsset we are asking
	// from the seller for the Currency)
	Price *big.Int `json:"price"`
	// Timeout the amount of time that we are willing to wait for the transaction to be mined.
	Timeout int64 `json:"timeout"`
	// Stage reflects the stage of the order.
	Stage int `json:"stage"`
	//FailureReason reflects the reason for the failure of the order, if any.
	FailureReason string `json:"failureReason"`
	// Assisted reflects if the trade order is to be assisted by the exchange or not.
	Assisted bool `json:"assisted"`
	// AssistedTradeOrderInformation reflects the information required to assist the trade order.
	AssistedTradeOrderInformation *AssistedTradeOrderInformation `json:"assistedTradeOrderInformation"`
	// NFTID reflects the NFT ID of the NFT that is being traded.
	NFTID int64 `json:"nftID"`
}

type BridgeRequest struct {
	// Currency reflects the currency that the SELLER wishes to trade. (bitcoin, mineonlium, USDT, etc).
	Currency string `json:"currency"`
	// Amount reflects the ammount of Currency the SELLER wishes to trade.
	Amount          *big.Int `json:"amount"`
	FromChain       string   `json:"fromChain"`
	BridgeTo        string   `json:"bridgeTo"`
	ShippingAddress string   `json:"shippingAddress"`
	TxId            string   `json:"TxId"`
}
