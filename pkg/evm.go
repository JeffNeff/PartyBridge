package be

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"time"

	bridge "github.com/TeaPartyCrypto/partybridge/pkg/contract/bridge"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

func (e *ExchangeServer) generateEVMAccount(chain string) *ecdsa.PrivateKey {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		log.Fatal(err)
	}

	privateKeyBytes := crypto.FromECDSA(privateKey)
	pk := hexutil.Encode(privateKeyBytes)[2:]
	e.logger.Debug("Generated " + chain + " Private Key: " + pk)
	return privateKey
}

func (a *ExchangeServer) waitAndVerifyEVMChain(ctx context.Context, client, client2 *ethclient.Client, request AccountWatchRequest) {
	// request.Account = "0x5D22D5c8675d3e3a6a1f296d740d6381CbD18769"
	if !a.watch {
		a.logger.Info("dev mode is on, not watching for payment. Returning success")
		awrr := &AccountWatchRequestResult{
			AccountWatchRequest: request,
			Result:              "success",
		}

		if err := a.Dispatch(awrr); err != nil {
			a.logger.Error("error dispatching account watch request result: " + err.Error())
			data := "The bridge server has encountered an error. Please contact support with the following ID: " + request.TransactionID
			a.sendStatusMsg(awrr.AccountWatchRequest.WSClientID, "error", data)
			// we need to store the error in redis so that we can manually resolve the issue later.
			a.storeFailedAccountWatchRequest(request)
			// remove the account watch request from the db
			if err := a.removeAccountWatchRequestFromDB(awrr.AccountWatchRequest.TransactionID); err != nil {
				a.logger.Errorw("failed to remove account watch request from db", err)
			}
			// TODO:: we need to send an update to the Developer Portal. So that we can address this issue
		}
		return
	}
	a.logger.Info("Watching for " + request.Account + " to have a payment of " + request.Amount.String() + " on chain " + request.Chain)

	// create a ticker that ticks every 60 seconds
	ticker := time.NewTicker(time.Second * 60)
	defer ticker.Stop()
	if a.dev {
		ticker = time.NewTicker(time.Second * 10)
	}

	// create a timer that times out after the specified timeout
	timer := time.NewTimer(time.Second * time.Duration(request.TimeOut))
	defer timer.Stop()

	account := common.HexToAddress(request.Account)

	// start a for loop that checks the balance of the address
	canILive := true
	for canILive {
		select {
		case <-ticker.C:
			balance, err := client.BalanceAt(context.Background(), account, nil)
			if err != nil {
				a.logger.Error("occured getting balance of " + request.Account + ": " + err.Error())
				return
			}
			a.logger.Infof("balance of %v is %v on chain %v", account, balance, request.Chain)
			// if the balance is equal to the amount, verify with the
			// second RPC server.
			if balance.Cmp(request.Amount) == 0 || balance.Cmp(request.Amount) == 1 {
				verifiedBalance, err := client2.BalanceAt(context.Background(), account, nil)
				if err != nil {
					a.logger.Error("occured getting balance of " + request.Account + ": " + err.Error() + " from the secondary ETH RPC server")
					return
				}

				if verifiedBalance.Cmp(request.Amount) == 0 || verifiedBalance.Cmp(request.Amount) == 1 {
					a.logger.Info("attempting to complete order " + request.TransactionID)
					// send a complete order event
					awrr := &AccountWatchRequestResult{
						AccountWatchRequest: request,
						Result:              "success",
					}

					if err := a.Dispatch(awrr); err != nil {
						a.logger.Error("error dispatching account watch request result: " + err.Error())
						data := "The bridge server has encountered an error. Please contact support with the following ID: " + request.TransactionID
						a.sendStatusMsg(awrr.AccountWatchRequest.WSClientID, "error", data)
						// we need to store the error in redis so that we can manually resolve the issue later.
						a.storeFailedAccountWatchRequest(request)
						// remove the account watch request from the db
						if err := a.removeAccountWatchRequestFromDB(awrr.AccountWatchRequest.TransactionID); err != nil {
							a.logger.Errorw("failed to remove account watch request from db", err)
						}
						// TODO:: we need to send an update to the Developer Portal. So that we can address this issue
					}
					canILive = false
					return
				} else {
					a.logger.Error("balance of " + request.Account + " is not equal to " + request.Amount.String())
					return
				}
			}
		case <-timer.C:
			// if the timer times out, return an error
			e := fmt.Sprintf("timeout occured waiting for " + request.Account + " to have a payment of " + request.Amount.String())
			a.logger.Info(e)
			awrr := &AccountWatchRequestResult{
				AccountWatchRequest: request,
				Result:              "error",
			}

			if err := a.Dispatch(awrr); err != nil {
				a.logger.Error("error dispatching account watch request result: " + err.Error())
			}

			canILive = false

			return
		}
	}

}

func (a *ExchangeServer) waitAndVerifyWGRAMSBridgeTokenOnOctaSpace(ctx context.Context, client, client2 *ethclient.Client, request AccountWatchRequest) {
	if !a.watch {
		a.logger.Info("dev mode is on, not watching for payment. Returning success")
		awrr := &AccountWatchRequestResult{
			AccountWatchRequest: request,
			Result:              "success",
		}

		if err := a.Dispatch(awrr); err != nil {
			a.logger.Error("error dispatching account watch request result: " + err.Error())
		}
		return
	}
	a.logger.Info("Watching for " + request.Account + " to have a payment of " + request.Amount.String() + " tokens on chain " + request.Chain)

	// create a ticker that ticks every 60 seconds
	ticker := time.NewTicker(time.Second * 60)
	defer ticker.Stop()
	if a.dev {
		ticker = time.NewTicker(time.Second * 10)
	}

	// create a timer that times out after the specified timeout
	timer := time.NewTimer(time.Second * time.Duration(request.TimeOut))
	defer timer.Stop()

	account := common.HexToAddress(request.Account)

	// start a for loop that checks the balance of the address
	canILive := true
	for canILive {
		select {
		case <-ticker.C:
			balance, err := a.queryWGRAMSBridgeContractOnOctaSpaceUserAccountBalance(request.Account, client)
			if err != nil {
				a.logger.Error("occured getting balance of " + request.Account + ": " + err.Error())
				return
			}
			a.logger.Infof("balance of %v is %v on chain %v", account, balance, request.Chain)
			// if the balance is equal to the amount, verify with the
			// second RPC server.
			if balance.Cmp(request.Amount) == 0 || balance.Cmp(request.Amount) == 1 {
				verifiedBalance, err := a.queryWGRAMSBridgeContractOnOctaSpaceUserAccountBalance(request.Account, client)
				if err != nil {
					a.logger.Error("occured getting balance of " + request.Account + ": " + err.Error() + " from the secondary ETH RPC server")
					return
				}

				if verifiedBalance.Cmp(request.Amount) == 0 || verifiedBalance.Cmp(request.Amount) == 1 {
					a.logger.Info("attempting to complete order " + request.TransactionID)
					// send a complete order event
					awrr := &AccountWatchRequestResult{
						AccountWatchRequest: request,
						Result:              "success",
					}

					if err := a.Dispatch(awrr); err != nil {
						a.logger.Error("error dispatching account watch request result: " + err.Error())
					}
					canILive = false
					return
				} else {
					a.logger.Error("balance of " + request.Account + " is not equal to " + request.Amount.String())
					return
				}
			}
		case <-timer.C:
			// if the timer times out, return an error
			e := fmt.Sprintf("timeout occured waiting for " + request.Account + " to have a payment of " + request.Amount.String())
			a.logger.Info(e)
			awrr := &AccountWatchRequestResult{
				AccountWatchRequest: request,
				Result:              "error",
			}

			if err := a.Dispatch(awrr); err != nil {
				a.logger.Error("error dispatching account watch request result: " + err.Error())
			}

			canILive = false

			return
		}
	}

}

func (a *ExchangeServer) waitAndVerifyWOCTABridgeTokenOnPartychain(ctx context.Context, client, client2 *ethclient.Client, request AccountWatchRequest) {
	if !a.watch {
		a.logger.Info("dev mode is on, not watching for payment. Returning success")
		awrr := &AccountWatchRequestResult{
			AccountWatchRequest: request,
			Result:              "success",
		}

		if err := a.Dispatch(awrr); err != nil {
			a.logger.Error("error dispatching account watch request result: " + err.Error())
		}
		return
	}
	a.logger.Info("Watching for " + request.Account + " to have a payment of " + request.Amount.String() + " tokens on chain " + request.Chain)

	// create a ticker that ticks every 60 seconds
	ticker := time.NewTicker(time.Second * 60)
	defer ticker.Stop()
	if a.dev {
		ticker = time.NewTicker(time.Second * 10)
	}

	// create a timer that times out after the specified timeout
	timer := time.NewTimer(time.Second * time.Duration(request.TimeOut))
	defer timer.Stop()

	account := common.HexToAddress(request.Account)

	// start a for loop that checks the balance of the address
	canILive := true
	for canILive {
		select {
		case <-ticker.C:
			balance, err := a.queryWOCTABridgeContractOnPartyChainUserAccountBalance(request.Account, client)
			if err != nil {
				a.logger.Error("occured getting balance of " + request.Account + ": " + err.Error())
				return
			}
			a.logger.Infow("check balance", "sid", request.WSClientID, "account", account, "balance", balance, "chain", request.Chain)
			// if the balance is equal to the amount, verify with the
			// second RPC server.
			if balance.Cmp(request.Amount) == 0 || balance.Cmp(request.Amount) == 1 {
				verifiedBalance, err := a.queryWOCTABridgeContractOnPartyChainUserAccountBalance(request.Account, client)
				if err != nil {
					a.logger.Error("occured getting balance of " + request.Account + ": " + err.Error() + " from the secondary ETH RPC server")
					return
				}

				if verifiedBalance.Cmp(request.Amount) == 0 || verifiedBalance.Cmp(request.Amount) == 1 {
					a.logger.Info("attempting to complete order " + request.TransactionID)
					// send a complete order event
					awrr := &AccountWatchRequestResult{
						AccountWatchRequest: request,
						Result:              "success",
					}

					if err := a.Dispatch(awrr); err != nil {
						a.logger.Error("error dispatching account watch request result: " + err.Error())
					}
					canILive = false
					return
				} else {
					a.logger.Error("balance of " + request.Account + " is not equal to " + request.Amount.String())
					return
				}
			}
		case <-timer.C:
			// if the timer times out, return an error
			e := fmt.Sprintf("timeout occured waiting for " + request.Account + " to have a payment of " + request.Amount.String())
			a.logger.Info(e)
			awrr := &AccountWatchRequestResult{
				AccountWatchRequest: request,
				Result:              "error",
			}

			if err := a.Dispatch(awrr); err != nil {
				a.logger.Error("error dispatching account watch request result: " + err.Error())
			}

			canILive = false

			return
		}
	}

}

// waitAndVerifyWBSCUSDTBridgeTokenOnOctaSpace waits for a payment of WBSCUSDT tokens on the Octa.Space chain
// TODO: waitAndVerifyWBSCUSDTBridgeTokenOnOctaSpace & waitAndVerifyWBSCUSDTBridgeTokenOnPartychain logic should be merged into a single function
func (a *ExchangeServer) waitAndVerifyWBSCUSDTBridgeTokenOnOctaSpace(ctx context.Context, client, client2 *ethclient.Client, request AccountWatchRequest) {
	if !a.watch {
		a.logger.Info("dev mode is on, not watching for payment. Returning success")
		awrr := &AccountWatchRequestResult{
			AccountWatchRequest: request,
			Result:              "success",
		}

		if err := a.Dispatch(awrr); err != nil {
			a.logger.Error("error dispatching account watch request result: " + err.Error())
		}
		return
	}
	a.logger.Info("Watching for " + request.Account + " to have a payment of " + request.Amount.String() + " tokens on chain " + request.Chain)

	// create a ticker that ticks every 60 seconds
	ticker := time.NewTicker(time.Second * 60)
	defer ticker.Stop()
	if a.dev {
		ticker = time.NewTicker(time.Second * 10)
	}

	// create a timer that times out after the specified timeout
	timer := time.NewTimer(time.Second * time.Duration(request.TimeOut))
	defer timer.Stop()

	account := common.HexToAddress(request.Account)

	// start a for loop that checks the balance of the address
	canILive := true
	for canILive {
		select {
		case <-ticker.C:
			balance, err := a.queryWBSCUSDTBridgeContractOnOctaSpaceUserAccountBalance(request.Account, client)
			if err != nil {
				a.logger.Error("occured getting balance of " + request.Account + ": " + err.Error())
				return
			}
			a.logger.Infow("check balance", "sid", request.WSClientID, "account", account, "balance", balance, "chain", request.Chain)
			// if the balance is equal to the amount, verify with the
			// second RPC server.
			if balance.Cmp(request.Amount) == 0 || balance.Cmp(request.Amount) == 1 {
				verifiedBalance, err := a.queryWBSCUSDTBridgeContractOnOctaSpaceUserAccountBalance(request.Account, client)
				if err != nil {
					a.logger.Error("occured getting balance of " + request.Account + ": " + err.Error() + " from the secondary ETH RPC server")
					return
				}

				if verifiedBalance.Cmp(request.Amount) == 0 || verifiedBalance.Cmp(request.Amount) == 1 {
					a.logger.Info("attempting to complete order " + request.TransactionID)
					// send a complete order event
					awrr := &AccountWatchRequestResult{
						AccountWatchRequest: request,
						Result:              "success",
					}

					if err := a.Dispatch(awrr); err != nil {
						a.logger.Error("error dispatching account watch request result: " + err.Error())
					}
					canILive = false
					return
				} else {
					a.logger.Error("balance of " + request.Account + " is not equal to " + request.Amount.String())
					return
				}
			}
		case <-timer.C:
			// if the timer times out, return an error
			e := fmt.Sprintf("timeout occured waiting for " + request.Account + " to have a payment of " + request.Amount.String())
			a.logger.Info(e)
			awrr := &AccountWatchRequestResult{
				AccountWatchRequest: request,
				Result:              "error",
			}

			if err := a.Dispatch(awrr); err != nil {
				a.logger.Error("error dispatching account watch request result: " + err.Error())
			}

			canILive = false

			return
		}
	}

}

func (e *ExchangeServer) queryWBSCUSDTBridgeContractOnOctaSpaceUserAccountBalance(account string, rpc *ethclient.Client) (*big.Int, error) {
	e.logger.Info("querying contract " + e.wBSCUSDTOnOctaSpaceContractAddress + " for balance of " + account)

	contract, err := bridge.NewPartyBridge(common.HexToAddress(e.wBSCUSDTOnOctaSpaceContractAddress), rpc)
	if err != nil {
		fmt.Println("error encounterd creating contract instance: " + err.Error())
		return nil, err
	}

	balance, err := contract.BalanceOf(nil, common.HexToAddress(account))
	if err != nil {
		fmt.Println("error encounterd creating contract instance: " + err.Error())
		return nil, err
	}

	fmt.Println("balance of " + account + " is " + balance.String())

	return balance, nil
}

// TODO: waitAndVerifyWBSCUSDTBridgeTokenOnOctaSpace & waitAndVerifyWBSCUSDTBridgeTokenOnPartychain logic should be merged into a single function
func (a *ExchangeServer) waitAndVerifyWBSCUSDTBridgeTokenOnPartychain(ctx context.Context, client, client2 *ethclient.Client, request AccountWatchRequest) {
	if !a.watch {
		a.logger.Info("dev mode is on, not watching for payment. Returning success")
		awrr := &AccountWatchRequestResult{
			AccountWatchRequest: request,
			Result:              "success",
		}

		if err := a.Dispatch(awrr); err != nil {
			a.logger.Error("error dispatching account watch request result: " + err.Error())
		}
		return
	}
	a.logger.Info("Watching for " + request.Account + " to have a payment of " + request.Amount.String() + " tokens on chain " + request.Chain)

	// create a ticker that ticks every 60 seconds
	ticker := time.NewTicker(time.Second * 60)
	defer ticker.Stop()
	if a.dev {
		ticker = time.NewTicker(time.Second * 10)
	}

	// create a timer that times out after the specified timeout
	timer := time.NewTimer(time.Second * time.Duration(request.TimeOut))
	defer timer.Stop()

	account := common.HexToAddress(request.Account)

	// start a for loop that checks the balance of the address
	canILive := true
	for canILive {
		select {
		case <-ticker.C:
			balance, err := a.queryWBSCUSDTridgeContractOnPartyChainUserAccountBalance(request.Account, client)
			if err != nil {
				a.logger.Error("occured getting balance of " + request.Account + ": " + err.Error())
				return
			}
			a.logger.Infow("check balance", "sid", request.WSClientID, "account", account, "balance", balance, "chain", request.Chain)
			// if the balance is equal to the amount, verify with the
			// second RPC server.
			if balance.Cmp(request.Amount) == 0 || balance.Cmp(request.Amount) == 1 {
				verifiedBalance, err := a.queryWBSCUSDTridgeContractOnPartyChainUserAccountBalance(request.Account, client)
				if err != nil {
					a.logger.Error("occured getting balance of " + request.Account + ": " + err.Error() + " from the secondary ETH RPC server")
					return
				}

				if verifiedBalance.Cmp(request.Amount) == 0 || verifiedBalance.Cmp(request.Amount) == 1 {
					a.logger.Info("attempting to complete order " + request.TransactionID)
					// send a complete order event
					awrr := &AccountWatchRequestResult{
						AccountWatchRequest: request,
						Result:              "success",
					}

					if err := a.Dispatch(awrr); err != nil {
						a.logger.Error("error dispatching account watch request result: " + err.Error())
					}
					canILive = false
					return
				} else {
					a.logger.Error("balance of " + request.Account + " is not equal to " + request.Amount.String())
					return
				}
			}
		case <-timer.C:
			// if the timer times out, return an error
			e := fmt.Sprintf("timeout occured waiting for " + request.Account + " to have a payment of " + request.Amount.String())
			a.logger.Info(e)
			awrr := &AccountWatchRequestResult{
				AccountWatchRequest: request,
				Result:              "error",
			}

			if err := a.Dispatch(awrr); err != nil {
				a.logger.Error("error dispatching account watch request result: " + err.Error())
			}

			canILive = false

			return
		}
	}

}

func (e *ExchangeServer) queryWGRAMSBridgeContractOnOctaSpaceUserAccountBalance(account string, rpc *ethclient.Client) (*big.Int, error) {
	e.logger.Info("querying contract " + e.wGRAMSOnOCTAContractAddress + " for balance of " + account)

	contract, err := bridge.NewPartyBridge(common.HexToAddress(e.wGRAMSOnOCTAContractAddress), rpc)
	if err != nil {
		fmt.Println("error encounterd creating contract instance: " + err.Error())
		return nil, err
	}

	balance, err := contract.BalanceOf(nil, common.HexToAddress(account))
	if err != nil {
		fmt.Println("error encounterd creating contract instance: " + err.Error())
		return nil, err
	}

	fmt.Println("balance of " + account + " is " + balance.String())

	return balance, nil
}

func (e *ExchangeServer) queryWBSCUSDTridgeContractOnPartyChainUserAccountBalance(account string, rpc *ethclient.Client) (*big.Int, error) {
	e.logger.Info("querying contract " + e.wBSCUSDTOnPartyChainContractAddress + " for balance of " + account)

	contract, err := bridge.NewPartyBridge(common.HexToAddress(e.wBSCUSDTOnPartyChainContractAddress), rpc)
	if err != nil {
		fmt.Println("error encounterd creating contract instance: " + err.Error())
		return nil, err
	}

	balance, err := contract.BalanceOf(nil, common.HexToAddress(account))
	if err != nil {
		fmt.Println("error encounterd creating contract instance: " + err.Error())
		return nil, err
	}

	fmt.Println("balance of " + account + " is " + balance.String())

	return balance, nil
}

func (e *ExchangeServer) queryWOCTABridgeContractOnPartyChainUserAccountBalance(account string, rpc *ethclient.Client) (*big.Int, error) {
	e.logger.Info("querying contract " + e.wOCTAOnPartyChainContractAddress + " for balance of " + account)

	contract, err := bridge.NewPartyBridge(common.HexToAddress(e.wOCTAOnPartyChainContractAddress), rpc)
	if err != nil {
		fmt.Println("error encounterd creating contract instance: " + err.Error())
		return nil, err
	}

	balance, err := contract.BalanceOf(nil, common.HexToAddress(account))
	if err != nil {
		fmt.Println("error encounterd creating contract instance: " + err.Error())
		return nil, err
	}

	fmt.Println("balance of " + account + " is " + balance.String())

	return balance, nil
}

func (e *ExchangeServer) sendCoreEVMAsset(fromAddress, privateKey string, toAddress string, amount *big.Int, txid string, rpcClient *ethclient.Client) error {
	// verify there are no missing or
	if toAddress == "" {
		e.logger.Error("toAddress is empty")
		return fmt.Errorf("toAddress is empty")
	}
	if amount == nil {
		e.logger.Error("amount is nil")
		return fmt.Errorf("amount is nil")
	}
	if rpcClient == nil {
		e.logger.Error("rpcClient is nil")
		return fmt.Errorf("rpcClient is nil")
	}
	if txid == "" {
		e.logger.Error("txid is empty")
		return fmt.Errorf("txid is empty")
	}

	// convert the string address to an address
	qualifiedFromAddress := common.HexToAddress(fromAddress)
	// send the currency to the buyer
	// read nonce
	nonce, err := rpcClient.PendingNonceAt(context.Background(), qualifiedFromAddress)
	if err != nil {
		e.logger.Error("cannot get nonce for " + fromAddress + ": " + err.Error())
		return err
	}

	qualifiedToAddress := common.HexToAddress(toAddress)

	// create gas params
	gasLimit := uint64(30000) // in units
	gasPrice, err := rpcClient.SuggestGasPrice(context.Background())
	if err != nil {
		e.logger.Error("error getting gas price: " + err.Error())
		return err
	}

	// create a transaction
	tx := types.NewTransaction(nonce, qualifiedToAddress, amount, gasLimit, gasPrice, nil)

	// fetch chain id
	chainID, err := rpcClient.NetworkID(context.Background())
	if err != nil {
		e.logger.Error("occured getting chain id: " + err.Error())
		return err
	}

	// convert the private key to a private key
	ecdsa, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		e.logger.Error("error converting private key to private key: " + err.Error())
		return err
	}

	// sign the transaction
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), ecdsa)
	if err != nil {
		e.logger.Error("error signing transaction: " + err.Error())
		return err
	}

	// send the transaction
	err = rpcClient.SendTransaction(context.Background(), signedTx)
	if err != nil {
		e.logger.Error("error sending transaction: " + err.Error())
		return err
	}

	e.logger.Info("tx sent: " + signedTx.Hash().Hex() + "txid: " + txid)
	return nil
}
