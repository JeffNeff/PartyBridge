package be

import (
	"context"
	"fmt"
	"time"
)

func (e *ExchangeServer) Dispatch(awrr *AccountWatchRequestResult) error {
	e.logger.Infof("dispatching awr %+v", awrr)
	if awrr.Result == "success" {
		// store the bridge account in the db
		if awrr.AccountWatchRequest.AssistedSellOrderInformation.Currency == "grams" || awrr.AccountWatchRequest.AssistedSellOrderInformation.Currency == "octa" || awrr.AccountWatchRequest.AssistedSellOrderInformation.Currency == "bscusdt" {
			e.logger.Infof("storing the bridge account in the db...")
			if err := e.storeBridgeAccount(*awrr); err != nil {
				e.logger.Errorw("failed to store bridge account in db", err)
				data := "There was a bridge failure. Please provide this id to support: " + awrr.AccountWatchRequest.TransactionID
				e.sendStatusMsg(awrr.AccountWatchRequest.WSClientID, "error", data)
				return err
			}
		}
	}

	e.logger.Infof("creating a new bridge request")
	if err := e.createBridgeRequest(*awrr); err != nil {
		// if the bridge request fails we should refund the buyer
		BridgeRequestsInc("failed", *awrr)
		e.logger.Errorw("failed to create bridge request", err)
		data := "There was a bridge failure. Please provide this id to support: " + awrr.AccountWatchRequest.TransactionID
		e.sendStatusMsg(awrr.AccountWatchRequest.WSClientID, "error", data)
		return err
	}

	BridgeRequestsDurationSet(*awrr)
	BridgeRequestsInc("success", *awrr)

	data := "The bridge reported a success"
	e.sendStatusMsg(awrr.AccountWatchRequest.WSClientID, "success", data)

	// remove the account watch request from the db
	if err := e.removeAccountWatchRequestFromDB(awrr.AccountWatchRequest.TransactionID); err != nil {
		e.logger.Errorw("failed to remove account watch request from db", err)
		// data := "There was a bridge failure. Please provide this id to support: " + awrr.AccountWatchRequest.TransactionID
		// e.broadcastToWebSocketClient(awrr.AccountWatchRequest.WSClientID, websocket.TextMessage, []byte(data))
		return err
	}

	return nil
}

func (e *ExchangeServer) createBridgeRequest(awrr AccountWatchRequestResult) error {
	fmt.Printf("creating bridge request for order: %+v", awrr)
	// try to mint the Wrapped asset
	switch awrr.AccountWatchRequest.AssistedSellOrderInformation.BridgeTo {
	case OCTA:
		{
			switch awrr.AccountWatchRequest.AssistedSellOrderInformation.Currency {
			case GRAMS:
				{
					e.logger.Infof("Creating a bridge request for order: %s for WGRAMS onto %s chain", awrr.AccountWatchRequest.TransactionID, awrr.AccountWatchRequest.AssistedSellOrderInformation.BridgeTo)
					return e.requestToMintWrappedCurrency(awrr)
				}
			case WOCTA:
				{
					e.logger.Infof("Creating a bridge request to unwrap WOCTA on OctaSpace for order: %+v", awrr)
					return e.requestToTransferCoinOnChainFromShim(awrr)
				}
			case BSCUSDT:
				{
					e.logger.Infof("Creating a bridge request to wrap BSCUSDT onto OctaSpace for order: %+v", awrr)
					return e.requestToMintWrappedCurrency(awrr)
				}
			default:
				{
					e.logger.Errorf("unsupported currency: %s", awrr.AccountWatchRequest.AssistedSellOrderInformation.Currency)
					return fmt.Errorf("unsupported currency: %s", awrr.AccountWatchRequest.AssistedSellOrderInformation.Currency)
				}
			}
		}
	case GRAMS:
		{
			switch awrr.AccountWatchRequest.AssistedSellOrderInformation.Currency {
			case OCTA:
				{
					// if we are briding OCTA to GRAMS, we need to mint WOCTA
					e.logger.Infof("Creating a bridge request for order: %s for WOCTA onto %s chain", awrr.AccountWatchRequest.TransactionID, awrr.AccountWatchRequest.AssistedSellOrderInformation.BridgeTo)
					return e.requestToMintWrappedCurrency(awrr)
				}
			case WGRAMS:
				{
					e.logger.Infof("Creating a bridge request to unwrap WGRAMS on PartyChain for order: %s", awrr.AccountWatchRequest.TransactionID)
					return e.requestToTransferCoinOnChainFromShim(awrr)
				}
			case BSCUSDT:
				{
					e.logger.Infof("Creating a bridge request to wrap BSCUSDT onto PartyChain for order: %s", awrr.AccountWatchRequest.TransactionID)
					return e.requestToMintWrappedCurrency(awrr)
				}
			default:
				{
					e.logger.Errorf("unsupported currency: %s", awrr.AccountWatchRequest.AssistedSellOrderInformation.Currency)
					return fmt.Errorf("unsupported currency: %s", awrr.AccountWatchRequest.AssistedSellOrderInformation.Currency)
				}
			}
		}
	case BSCUSDT:
		{
			fmt.Printf("creating bridge request for order: %+v", awrr)
			switch awrr.AccountWatchRequest.AssistedSellOrderInformation.BridgeFrom {
			case OCTA:
				{
					// if we are trying to bridge WBSCUSDT from OCTA to BSC, we need to unwrap the WBSCTUSDT on OCTA and then transfer the stored bridge asset to the user on bsc
					e.logger.Infof("Creating a bridge request to unwrap WBSCUSDT from OCTA and transfer to user on BSC for order: %s", awrr.AccountWatchRequest.TransactionID)
					return e.requestToTransferCoinOnChainFromShim(awrr)
				}
			case GRAMS:
				{
					// if we are trying to bridge WBSCUSDT from GRAMS to BSC, we need to unwrap the WBSCTUSDT on GRAMS and then transfer the stored bridge asset to the user on bsc
					e.logger.Infof("Creating a bridge request to unwrap WBSCUSDT from GRAMS and transfer to user on BSC for order: %s", awrr.AccountWatchRequest.TransactionID)
					return e.requestToTransferCoinOnChainFromShim(awrr)
				}
			default:
				{
					e.logger.Errorf("unsupported bridge from: %s", awrr.AccountWatchRequest.AssistedSellOrderInformation.BridgeFrom)
					return fmt.Errorf("unsupported bridge from: %s", awrr.AccountWatchRequest.AssistedSellOrderInformation.BridgeFrom)
				}
			}
		}

	default:
		{
			e.logger.Errorf("unsupported bridge to: %s", awrr.AccountWatchRequest.AssistedSellOrderInformation.BridgeTo)
			return fmt.Errorf("unsupported bridge to: %s", awrr.AccountWatchRequest.AssistedSellOrderInformation.BridgeTo)
		}
	}
}

func (e *ExchangeServer) watchAccount(awr *AccountWatchRequest) {
	e.logger.Infow("watching account", "sid", awr.WSClientID, "account", awr.Account)
	awr.Locked = true
	awr.LockedBy = e.podName
	awr.LockedTime = time.Now()
	// tell the database that this instance of the exchange is watching this account
	if err := e.updateAccountWatchRequestInDB(*awr); err != nil {
		e.logger.Errorw("updating account watch request in db", "sid", awr.WSClientID, "error", err.Error())
	}

	if awr.AssistedSellOrderInformation.BridgeTo != "" {
		switch awr.Chain {
		case OCTA:
			switch awr.AssistedSellOrderInformation.BridgeTo {
			case GRAMS:
				switch awr.AssistedSellOrderInformation.Currency {
				case OCTA:
					// if our chain is Octa and we are bridging Octa to PartyChain.
					// then we need to watch for native Octa on OctaChain.
					e.logger.Infow("watching account for bridge order from OCTA to GRAMS", "sid", awr.WSClientID)
					e.waitAndVerifyEVMChain(context.Background(), e.octNode.rpcClient, e.octNode.rpcClientTwo, *awr)
					return
				case "wgrams":
					// if our chain is Octa and we are briding wgrams to Partychain
					e.logger.Infow("watching account for bridge order from OCTA to GRAMS to unwrap WGRAMS into GRAMS", "sid", awr.WSClientID)
					e.waitAndVerifyWGRAMSBridgeTokenOnOctaSpace(context.Background(), e.octNode.rpcClient, e.octNode.rpcClientTwo, *awr)
				default:
					e.logger.Error("unknown currency: " + awr.AssistedSellOrderInformation.Currency)
					return
				}
			case BSCUSDT: // if our chain is Octa and we are briding BSCUSDT
				switch awr.AssistedSellOrderInformation.Currency {
				case WBSCUSDT: // if we are briding WBSCUSDT
					e.logger.Infow("watching account for bridge order from OCTA to BSC to unwrap WBSCUSDT into BSCUSDT", "sid", awr.WSClientID)
					e.waitAndVerifyWBSCUSDTBridgeTokenOnOctaSpace(context.Background(), e.octNode.rpcClient, e.octNode.rpcClientTwo, *awr)
				default:
					e.logger.Error("unknown currency: " + awr.AssistedSellOrderInformation.Currency)
					return
				}
			}
		case GRAMS:
			switch awr.AssistedSellOrderInformation.BridgeTo {
			case OCTA:
				switch awr.AssistedSellOrderInformation.Currency {
				case "wocta":
					// if our chain is PartyChain and we are bridging wocta to octaspace
					e.logger.Infow("watching account for bridge order from GRAMS to OCTA to unwrap WOCTA into OCTA", "sid", awr.WSClientID)
					// TODO: error handling
					e.waitAndVerifyWOCTABridgeTokenOnPartychain(context.Background(), e.partyChain.rpcClient, e.partyChain.rpcClientTwo, *awr)
					return
				case GRAMS:
					e.logger.Infow("watching account for bridge order from GRAMS to OCTA", "sid", awr.WSClientID)
					// TODO: error handling
					e.waitAndVerifyEVMChain(context.Background(), e.partyChain.rpcClient, e.partyChain.rpcClientTwo, *awr)
					return
				default:
					e.logger.Error("unknown currency: " + awr.AssistedSellOrderInformation.Currency)
				}
			case BSCUSDT: // if our chain is PartyChain and we are bridging BSCUSDT
				switch awr.AssistedSellOrderInformation.Currency {
				case WBSCUSDT:
					e.logger.Infow("watching account to unwrap WBSCUSDT from GRAMS and transfer to user on BSC", "sid", awr.WSClientID)
					e.waitAndVerifyWBSCUSDTBridgeTokenOnPartychain(context.Background(), e.partyChain.rpcClient, e.partyChain.rpcClientTwo, *awr)
					return
				}
			default:
				e.logger.Error("unknown chain: " + awr.Chain)
			}
		case BSCUSDT:
			switch awr.AssistedSellOrderInformation.BridgeTo {
			case OCTA:
				switch awr.AssistedSellOrderInformation.Currency {
				case BSCUSDT:
					e.logger.Infow("watching account for bridge order from BSCUSDT to OCTA", "sid", awr.WSClientID)
					e.waitAndVerifyBSCUSDT(*awr)
				}
			case GRAMS:
				switch awr.AssistedSellOrderInformation.Currency {
				case BSCUSDT:
					e.logger.Infow("watching account for bridge order from BSCUSDT to GRAMS", "sid", awr.WSClientID)
					e.waitAndVerifyBSCUSDT(*awr)
				}
			}

		default:
			e.logger.Error("unknown chain: " + awr.Chain)
		}
	}
}
