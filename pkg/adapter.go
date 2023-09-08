package be

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	pkgadapter "knative.dev/eventing/pkg/adapter/v2"
	"knative.dev/pkg/logging"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"

	uuid "github.com/google/uuid"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/go-redis/redis/v9"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// EnvAccessorCtor for configuration parameters
func EnvAccessorCtor() pkgadapter.EnvConfigAccessor {
	return &envAccessor{}
}

var _ pkgadapter.Adapter = (*ExchangeServer)(nil)

// Upgrader configures the upgrade request response for the WebSocket connection
var Upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// NewAdapter adapter implementation
func NewAdapter(ctx context.Context, envAcc pkgadapter.EnvConfigAccessor, ceClient cloudevents.Client) pkgadapter.Adapter {
	env := envAcc.(*envAccessor)
	e := &ExchangeServer{}
	e.wsClients = make(map[string]*WebSocketClient)
	e.logger = logging.FromContext(ctx)

	// Initialize the Party Chain nodes.
	partyclient, err := ethclient.Dial(env.PartyChainRPC1)
	if err != nil {
		e.logger.Errorw("Error connecting to PartyChain RPC 1", "rpc", env.PartyChainRPC1)
		if !env.Development {
			panic(err)
		}
	}

	partyclientTwo, err := ethclient.Dial(env.PartyChainRPC2)
	if err != nil {
		e.logger.Errorw("Error connecting to PartyChain RPC 2", "rpc", env.PartyChainRPC2)
		if !env.Development {
			panic(err)
		}
	}

	octClient, err := ethclient.Dial(env.OCTARPC1)
	if err != nil {
		e.logger.Errorw("Error connecting to OctaSpace RPC 1", "rpc", env.OCTARPC1)
		if !env.Development {
			panic(err)
		}
	}

	octClient2, err := ethclient.Dial(env.OCTARPC2)
	if err != nil {
		e.logger.Errorw("Error connecting to OctaSpace RPC 2", "rpc", env.OCTARPC2)
		if !env.Development {
			panic(err)
		}
	}

	e.partyChain.rpcClient = partyclient
	e.partyChain.rpcClientTwo = partyclientTwo
	e.octNode.rpcClient = octClient
	e.octNode.rpcClientTwo = octClient2
	e.shimCertLocation = env.ShimCertLocation
	e.watch = env.Watch
	e.dev = env.Development
	e.ceClient = ceClient
	e.gramsShimServerAddress = env.WGramsShimServerAddress
	e.octaShimServerAddress = env.WOctaShimServerAddress
	e.bscUSDTOnOctaSpaceShimServerAddress = env.WBSCUSDTOnOctaSpaceShimServerAddress
	e.bscUSDTOnPartyChainShimServerAddress = env.WBSCUSDTOnPartyChainShimServerAddress
	e.wGRAMSOnOCTAContractAddress = env.WGRAMSOnOCTAContractAddress
	e.wOCTAOnPartyChainContractAddress = env.WOCTAOnPartyChainContractAddress
	e.wBSCUSDTOnPartyChainContractAddress = env.WBSCUSDTOnPartyChainContractAddress
	e.wBSCUSDTOnOctaSpaceContractAddress = env.WBSCUSDTOnOCTAContractAddress
	e.minimumAmount = env.MinimumAmount
	e.fee = env.Fee
	e.SSLCRTLocation = env.ServerSSLCRTFilePath
	e.ServerSSLKeyFilePath = env.ServerSSLKeyFilePath

	if env.PodName == "" {
		e.podName = uuid.New().String()
	} else {
		e.podName = env.PodName
	}

	// Initialize the Redis client.
	e.redisClient = redis.NewClient(&redis.Options{
		Addr:     env.RedisAddress,
		Password: env.RedisPassword,
		DB:       env.RedisDB,
	})

	// Test the Redis connection.
	_, err = e.redisClient.Ping(ctx).Result()
	if err != nil {
		e.logger.Errorw("connecting to Redis")
		if !env.Development {
			panic(err)
		}
	}

	return e
}

// handleRoot handles the root endpoint. it only acks as an acknowledgement that the server is running. and for the ACME challenge.
func (e *ExchangeServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("we poppin pimpin"))
}

func (e *ExchangeServer) Start(ctx context.Context) error {
	e.logger.Info("starting warren...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalCh
		cancel()
	}()

	go e.StartWarren(ctx)
	e.logger.Info("started warren")
	e.logger.Info("starting http server...")
	cert, err := tls.LoadX509KeyPair(e.SSLCRTLocation, e.ServerSSLKeyFilePath)
	if err != nil {
		e.logger.Errorw("loading SSL cert", "error", err)
		return err
	}

	router := mux.NewRouter()
	router.HandleFunc("/", e.handleRoot)
	router.HandleFunc("/wss", e.handleWebSocketConnection)
	router.Handle("/metrics", promhttp.Handler())

	// start a http server without TLS on 8081
	go func() {
		if err := http.ListenAndServe(":8081", router); err != nil && err != http.ErrServerClosed {
			e.logger.Infof("started on 8081")
			e.logger.Fatalf("listen: %s\n", err)
		}
	}()

	server := &http.Server{
		Addr:    ":8080",
		Handler: router,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
	}

	go func() {
		if err := server.ListenAndServeTLS(e.SSLCRTLocation, e.ServerSSLKeyFilePath); err != nil && err != http.ErrServerClosed {
			e.logger.Infof("started on 8080")
			e.logger.Fatalf("listen: %s\n", err)
		}
	}()
	<-ctx.Done()

	e.warrenWG.Wait()

	e.logger.Info("stopping partybridge")
	e.shutdown(ctx, server)
	return nil
}

func (e *ExchangeServer) shutdown(ctx context.Context, server *http.Server) {
	if err := e.updateAccountWatchRequestsOnCrash(); err != nil {
		e.logger.Errorw("error updating account watch requests on crash", "error", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		panic(err)
	} else {
		e.logger.Info("application shutdown")
	}
}

func (e *ExchangeServer) handleWebSocketConnection(w http.ResponseWriter, r *http.Request) {
	e.logger.Debug("handling websocket connection")

	// add the CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	conn, err := Upgrader.Upgrade(w, r, nil)
	if err != nil {
		e.logger.Errorw("failed to upgrade connection to WebSocket", "error", err)
		return
	}

	// Create a channel for sending messages to the client
	messageChan := make(chan []byte)

	sid := uuid.New().String()

	client := &WebSocketClient{conn: conn, sid: sid, send: messageChan}
	e.wsClientsMutex.Lock()
	e.wsClients[sid] = client
	e.wsClientsMutex.Unlock()

	e.logger.Infow("new client connected", "sid", sid)

	helloMsg := HelloMsg{
		Type:          "hello",
		SID:           sid,
		MinimumAmount: e.minimumAmount,
		Fee:           e.fee,
	}

	data, err := json.Marshal(helloMsg)

	if err != nil {
		e.logger.Errorw("failed encode HelloMsg", "sid", sid, "error", err)
		return
	}

	e.logger.Infow("helloMsg", "sid", sid, "data", helloMsg)
	client.conn.WriteMessage(websocket.TextMessage, []byte(data))

	// Start the Goroutine to handle the client's connection
	go e.handleClientRequests(*client)
}

func (e *ExchangeServer) handleClientRequests(client WebSocketClient) {
	defer func() {
		e.wsClientsMutex.Lock()
		delete(e.wsClients, client.sid)
		e.wsClientsMutex.Unlock()
		client.conn.Close()
	}()

	for {
		_, message, err := client.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				e.logger.Errorw("websocket connection closed", "sid", client.sid, "reason", err)
			}
			break
		}
		var req RequestBridgeMsg
		json.Unmarshal([]byte(message), &req)

		e.logger.Infow("handle request", "sid", client.sid, "req", req)

		if req.Type == "requestBridge" {
			if req.Data.Amount.Cmp(ToWei(e.minimumAmount, 18)) == -1 && !e.dev {
				e.sendStatusMsg(client.sid, "error", "amount value is less than minimum")
				return
			}

			if client.acc == nil {
				acc := e.generateEVMAccount(req.Data.Currency)
				client.acc = acc
			}
			client.request = req.Data

			resp := RequestBridgeResponseMsg{
				Type:    "requestBridgeResponse",
				Amount:  req.Data.Amount.Add(req.Data.Amount, ToWei(e.fee, 18)),
				Address: crypto.PubkeyToAddress(client.acc.PublicKey).String(),
			}
			data, err := json.Marshal(resp)

			if err != nil {
				e.logger.Errorw("failed encode RequestBridgeResponseMsg", "sid", client.sid, "error", err)
				return
			}

			e.logger.Infow("requestBridgeResponse", "sid", client.sid, "data", resp)
			client.conn.WriteMessage(websocket.TextMessage, []byte(data))
		}

		if req.Type == "confirmBridge" {
			depositAccountWatchRequest := AccountWatchRequest{
				TransactionID: uuid.New().String(),
				AWRID:         uuid.New().String(),
				Account:       crypto.PubkeyToAddress(client.acc.PublicKey).String(),
				Chain:         client.request.FromChain,
				Amount:        client.request.Amount,
				TimeOut:       time.Now().Add(time.Minute * 30).Unix(),
				LockedBy:      e.podName,
				WSClientID:    client.sid,
				CreatedTime:   time.Now(),
				AssistedSellOrderInformation: AssistedTradeOrderInformation{
					BridgeTo:              client.request.BridgeTo,
					Currency:              client.request.Currency,
					SellerShippingAddress: client.request.ShippingAddress,
					Amount:                client.request.Amount,
					SellersEscrowWallet: EscrowWallet{
						PublicAddress: crypto.PubkeyToAddress(client.acc.PublicKey).String(),
						PrivateKey:    hex.EncodeToString(client.acc.D.Bytes()),
						Chain:         client.request.FromChain,
					},
					TradeAsset: client.request.BridgeTo,
					BridgeFrom: client.request.FromChain,
				},
			}
			if err := e.updateAccountWatchRequestInDB(depositAccountWatchRequest); err != nil {
				e.logger.Errorw("error updating account watch request in db", "sid", client.sid, "error", err.Error(), "data", depositAccountWatchRequest)
				return
			}
		}
	}
}

func (e *ExchangeServer) sendStatusMsg(SID string, msgType string, message string) {
	client, ok := e.wsClients[SID]
	if !ok {
		e.logger.Errorw("client session not found", "sid", SID)
		return
	}

	statusMsg := StatusMsg{
		Type:    msgType,
		Message: message,
	}

	data, err := json.Marshal(statusMsg)

	if err != nil {
		e.logger.Errorw("failed encode statusMsg", "sid", SID, "error", err)
		return
	}

	e.logger.Infow("sendStatusMsg", "sid", SID, "data", statusMsg)

	client.conn.WriteMessage(websocket.TextMessage, []byte(data))
}

// StartWarren starts the warren account watching service
func (e *ExchangeServer) StartWarren(ctx context.Context) error {
	// log the pod name
	e.logger.Infof("starting warren on pod %s", os.Getenv("POD_NAME"))
	// start the account watch service.
	e.ctx = ctx
	e.warrenWG = &sync.WaitGroup{}
	numWorkers := runtime.NumCPU()
	e.warrenWG.Add(numWorkers)
	e.warrenChan = make(chan AccountWatchRequest, numWorkers)

	for i := 0; i < numWorkers; i++ {
		go e.warrenWorker()
	}

	// create a timer that ticks every 30 seconds.
	// create a ticker that ticks every 30 seconds
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()
	if e.dev {
		ticker = time.NewTicker(time.Second * 10)
	}

	canIlive := true
	for canIlive {
		select {
		case <-ticker.C:
			e.logger.Debug("clearing the cawr and updating it with the awr from the database")
			// clear the cawr
			cawr := make([]AccountWatchRequest, 0)
			// update the account watch requests
			// get the account watch requests from the database.
			awr, err := e.retrieveAccountWatchRequestsFromDB()
			if err != nil {
				e.logger.Errorw("error retrieving account watch requests from database", "error", err)
				// do not return the error, continue running the loop
				continue
			}
			if awr != nil {
				if len(awr) > 0 {
					cawr = append(cawr, awr...)
				}
			}
			// send the account watch requests to the warren service.
			e.Warren(cawr)
		case <-ctx.Done():
			// context is canceled, stop the loop
			e.logger.Info("context is canceled, stopping the warren loop")
			canIlive = false
		}
	}
	return nil
}

func (e *ExchangeServer) Warren(awr []AccountWatchRequest) {
	if len(awr) == 0 || awr == nil {
		//e.logger.Debug("no account watch requests to watch")
		return
	}

	// illerate over the awr's, verify that the watch is not locked, and start the watch.
	// if the watch is locked, verify that the watch is not older then 2.2 hours old.
	// if it is older then 2.2 hours old, then restart the watch.
	for _, request := range awr {
		if request.Locked {
			// check if the watch is older then 2.2 hours old.
			// if it is, then restart the watch.
			// if time.Since(request.LockedTime) > time.Hour*2 {
			// 	e.logger.Infof("watch for account %s is older then 2.2 hours, restarting the watch", request.Account)
			// 	// restart the watch.
			// 	request.Locked = true
			// 	request.LockedTime = time.Now()
			// 	request.LockedBy = os.Getenv("POD_NAME")
			// 	e.warrenChan <- request
			// }
			continue
		}
		e.warrenChan <- request
	}
}

func (e *ExchangeServer) warrenWorker() {
	defer e.warrenWG.Done()

	for {
		select {
		case request := <-e.warrenChan:
			if !request.Locked {
				e.logger.Infow("starting watch for account", "sid", request.WSClientID, "account", request.Account, "chain", request.Chain)
				// start the watch.
				go e.watchAccount(&request)
			}
		case <-e.ctx.Done():
			// Exit the loop when the context is canceled
			return
		}
	}
}
