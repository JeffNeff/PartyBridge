package be

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
)

type MintRequest struct {
	ToAddress string   `json:"toAddress"`
	Amount    *big.Int `json:"amount"`
	FromPk    string   `json:"fromPk"`
	SID       string   `json:"sid"`
}

func (e *ExchangeServer) requestToMintWrappedCurrency(awrr AccountWatchRequestResult) error {
	mintRequest := MintRequest{
		ToAddress: awrr.AccountWatchRequest.AssistedSellOrderInformation.SellerShippingAddress,
		Amount:    awrr.AccountWatchRequest.Amount,
		SID:       awrr.AccountWatchRequest.WSClientID,
	}

	jsn, err := json.Marshal(mintRequest)
	if err != nil {
		return err
	}

	var shimServerAddress string
	switch awrr.AccountWatchRequest.AssistedSellOrderInformation.Currency {
	case OCTA:
		shimServerAddress = e.octaShimServerAddress
	case GRAMS:
		shimServerAddress = e.gramsShimServerAddress
	case BSCUSDT:
		// we need to know what chain we are bridging to
		switch awrr.AccountWatchRequest.AssistedSellOrderInformation.BridgeTo {
		case GRAMS:
			shimServerAddress = e.bscUSDTOnPartyChainShimServerAddress
		case OCTA:
			shimServerAddress = e.bscUSDTOnOctaSpaceShimServerAddress
		default:
			return fmt.Errorf("bridge to chain does not support BSCUSDT: %s", awrr.AccountWatchRequest.AssistedSellOrderInformation.BridgeTo)
		}
	}

	// Load client certificate and key pair
	cert, err := tls.LoadX509KeyPair(e.shimCertLocation+"/client.crt", e.shimCertLocation+"/client.key")
	if err != nil {
		log.Fatal(err)
	}

	// Load the server's CA certificate
	caCert, err := ioutil.ReadFile(e.shimCertLocation + "/ca.crt")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Create a new HTTP client with a custom TLS configuration
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
				RootCAs:      caCertPool,
			},
		},
	}

	// Create HTTPS POST request to the WGRAMS PartyShim
	req, err := http.NewRequest("POST", "https://"+shimServerAddress+"/mint", bytes.NewBuffer(jsn))
	if err != nil {
		return err
	}

	// Set the content type
	req.Header.Set("Content-Type", "application/json")

	// Add the client certificate to the request
	req.TLS = &tls.ConnectionState{
		HandshakeComplete: true,
	}
	// Send the request
	res, err := client.Do(req)
	if err != nil {
		return err
	}

	// Check the response if the header contains 500 then return an error
	if res.StatusCode == http.StatusInternalServerError {
		// TODO: this should emit metircs, logs, failure retry, etc.
		return errors.New("failed to mint")
	}

	return nil
}

func (e *ExchangeServer) requestToTransferCoinOnChainFromShim(awr AccountWatchRequestResult) error {
	if awr.AccountWatchRequest.Amount == nil {
		e.logger.Errorf("amount is nil")
		return errors.New("amount is nil")
	}
	// fetch the private key from the database
	bs, err := e.retrieveBridgeAccount(awr)
	if err != nil {
		e.logger.Errorf("failed to retrieve bridge account: %v", err)
		return err
	}

	e.logger.Infof("awr %+v", awr)

	var transferRequest MintRequest
	if bs == nil {
		transferRequest = MintRequest{
			ToAddress: awr.AccountWatchRequest.AssistedSellOrderInformation.SellerShippingAddress,
			Amount:    awr.AccountWatchRequest.Amount,
			SID:       awr.AccountWatchRequest.WSClientID,
		}
	} else {
		transferRequest = MintRequest{
			ToAddress: awr.AccountWatchRequest.AssistedSellOrderInformation.SellerShippingAddress,
			Amount:    awr.AccountWatchRequest.Amount,
			FromPk:    bs.PrivateKey,
			SID:       awr.AccountWatchRequest.WSClientID,
		}
	}

	jsn, err := json.Marshal(transferRequest)
	if err != nil {
		return err
	}

	var shimServerAddress string
	var shimEndpoint string
	switch awr.AccountWatchRequest.AssistedSellOrderInformation.BridgeTo {
	case GRAMS:
		shimServerAddress = e.gramsShimServerAddress
		shimEndpoint = "/transfer"
	case OCTA:
		shimServerAddress = e.octaShimServerAddress
		shimEndpoint = "/transfer"
	case BSCUSDT:
		// if the bridge to is BSCUSDT then we need to know what chain the request is coming from
		if awr.AccountWatchRequest.AssistedSellOrderInformation.BridgeFrom == GRAMS {
			shimServerAddress = e.bscUSDTOnPartyChainShimServerAddress
			shimEndpoint = "/transferBSCUSDT"
		} else if awr.AccountWatchRequest.AssistedSellOrderInformation.BridgeFrom == OCTA {
			shimEndpoint = "/transferBSCUSDT"
			shimServerAddress = e.bscUSDTOnOctaSpaceShimServerAddress
		}
	}

	// Load client certificate and key pair
	cert, err := tls.LoadX509KeyPair(e.shimCertLocation+"/client.crt", e.shimCertLocation+"/client.key")
	if err != nil {
		log.Fatal(err)
	}

	// Load the server's CA certificate
	caCert, err := ioutil.ReadFile(e.shimCertLocation + "/ca.crt")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Create a new HTTP client with a custom TLS configuration
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
				RootCAs:      caCertPool,
			},
		},
	}

	e.logger.Infof("requesting from %+v shim: %s", jsn, shimServerAddress)
	// create http post request to the WGRAMS PartyShim
	req, err := http.NewRequest("POST", "https://"+shimServerAddress+shimEndpoint, bytes.NewBuffer(jsn))
	if err != nil {
		// if the response contains "insufficient balance" then we need to throw an error and retrieve another bridge account
		// and try again
		if strings.Contains(err.Error(), "insufficient balance") {
			e.logger.Errorf("failed to create request to transfer coin on chain from shim: %v", err)
			return e.requestToTransferCoinOnChainFromShim(awr)
		}
		return err
	}

	// set the content type
	req.Header.Set("Content-Type", "application/json")
	// send the request
	req.TLS = &tls.ConnectionState{
		HandshakeComplete: true,
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}

	// check the response if the header contains 500 then return an error
	if res.StatusCode == http.StatusInternalServerError {
		e.logger.Errorf("failed to transfer native asset on chain")
		return errors.New("failed to transfer native asset  on chain")
	}

	// remove the bridge account from the db
	e.logger.Infof("Updating the bridge account in the db...")
	// if err := e.removeBridgeAccountFromDB(awrr.AccountWatchRequest.TransactionID); err != nil {
	// 	e.logger.Errorw("failed to remove bridge account from db", err)
	// 	// data := "There was a bridge failure. Please provide this id to support: " + awrr.AccountWatchRequest.TransactionID
	// }
	if err := e.updateBridgeAccountInDB(bs.ID, awr.AccountWatchRequest.Amount); err != nil {
		e.logger.Errorw("failed to update bridge account in db", err)
		// data := "There was a bridge failure. Please provide this id to support: " + awrr.AccountWatchRequest.TransactionID
	}

	return nil
}

// func (e *ExchangeServer) requestToTransferGRAMSOnPartyChain(awr AccountWatchRequestResult) error {
// 	if awr.AccountWatchRequest.Amount == nil {
// 		return errors.New("amount is nil")
// 	}
// 	// we need to fetch the private key from the database
// 	bs, err := e.retrieveBridgeAccount(awr.AccountWatchRequest.Amount)
// 	if err != nil {
// 		return err
// 	}

// 	if bs == nil {
// 		// todo we should use a default bridge account to fund here
// 		return errors.New("no bridge account found")
// 	}

// 	transferRequset := MintRequest{
// 		ToAddress: awr.AccountWatchRequest.AssistedSellOrderInformation.SellerShippingAddress,
// 		Amount:    awr.AccountWatchRequest.Amount,
// 		FromPk:    bs.PrivateKey,
// 	}

// 	jsn, err := json.Marshal(transferRequset)
// 	if err != nil {
// 		return err
// 	}

// 	// create http post request to the WGRAMS PartyShim
// 	req, err := http.NewRequest("POST", "http://"+e.gramsShimServerAddress+"/transfer", bytes.NewBuffer(jsn))
// 	if err != nil {
// 		return err
// 	}

// 	// set the content type
// 	req.Header.Set("Content-Type", "application/json")
// 	// send the request
// 	client := &http.Client{}
// 	res, err := client.Do(req)
// 	if err != nil {
// 		return err
// 	}

// 	// check the response if the header contains 500 then return an error
// 	if res.StatusCode == http.StatusInternalServerError {
// 		return errors.New("failed to transfer GRAMS on the party chain")
// 	}

// 	return nil
// }

// func (e *ExchangeServer) requestToMintWGRAMSOnOctaSpace(awrr AccountWatchRequestResult) error {
// 	mintRequest := MintRequest{
// 		ToAddress: awrr.AccountWatchRequest.AssistedSellOrderInformation.SellerShippingAddress,
// 		Amount:    awrr.AccountWatchRequest.Amount,
// 	}

// 	jsn, err := json.Marshal(mintRequest)
// 	if err != nil {
// 		return err
// 	}

// 	// create http post request to the WGRAMS PartyShim
// 	req, err := http.NewRequest("POST", "http://"+e.gramsShimServerAddress+"/mint", bytes.NewBuffer(jsn))
// 	if err != nil {
// 		return err
// 	}

// 	// set the content type
// 	req.Header.Set("Content-Type", "application/json")
// 	// send the request
// 	client := &http.Client{}
// 	res, err := client.Do(req)
// 	if err != nil {
// 		return err
// 	}

// 	// check the response if the header contains 500 then return an error
// 	if res.StatusCode == http.StatusInternalServerError {
// 		// TODO: this should emit metircs, logs, failure retry, etc.
// 		return errors.New("failed to mint WGRAMS on the party chain")
// 	}

// 	return nil
// }
