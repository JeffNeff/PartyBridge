package be

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"

	uuid "github.com/google/uuid"
	"go.uber.org/zap"
)

// updateAccountWatchRequestInDB updates the account watch request in the database
// so that it can be recovered in the event of a crash.
func (e *ExchangeServer) updateAccountWatchRequestInDB(request AccountWatchRequest) error {
	// retrieve the list of current account watch requests
	requests, _ := e.redisClient.Get(context.Background(), "accountwatchrequests").Result()
	// if err != nil {
	// 	return err
	// }``
	// unmarshal the list of account watch requests
	var currentRequests []AccountWatchRequest
	if requests != "" {
		err := json.Unmarshal([]byte(requests), &currentRequests)
		if err != nil {
			return err
		}
	}
	// if the request exists in the list, update it
	var found bool
	for i, r := range currentRequests {
		u1, err := uuid.Parse(r.AWRID)
		if err != nil {
			return err
		}
		u2, err := uuid.Parse(request.AWRID)
		if err != nil {
			return err
		}

		if u1 == u2 {
			currentRequests[i] = request
			found = true
		}
	}
	// if the request does not exist in the list, add it
	if !found {
		currentRequests = append(currentRequests, request)
	}
	// marshal the list of account watch requests
	crjs, err := json.Marshal(currentRequests)
	if err != nil {

		return err
	}
	// store the new list of account watch requests
	err = e.redisClient.Set(context.Background(), "accountwatchrequests", crjs, 0).Err()
	if err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

// update all account watch requests to reflect that the exchange server has crashed
// so we need to unlock them so that they can be processed again by another exchange server
func (e *ExchangeServer) updateAccountWatchRequestsOnCrash() error {
	// retrieve the list of current account watch requests
	requests, _ := e.redisClient.Get(context.Background(), "accountwatchrequests").Result()
	// if err != nil {
	// 	return err
	// }

	// unmarshal the list of account watch requests
	var currentRequests []AccountWatchRequest
	if requests != "" {
		err := json.Unmarshal([]byte(requests), &currentRequests)
		if err != nil {
			return err
		}
	}

	// compare the pod name of each request to the lockedby param
	// if they match, unlock the request
	for i, r := range currentRequests {
		if r.LockedBy == e.podName {
			currentRequests[i].Locked = false
			currentRequests[i].LockedBy = ""
		}
	}

	// marshal the list of account watch requests
	crjs, err := json.Marshal(currentRequests)
	if err != nil {
		return err
	}

	// store the new list of account watch requests
	err = e.redisClient.Set(context.Background(), "accountwatchrequests", crjs, 0).Err()
	if err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

// removeAccountWatchRequestFromDB removes the account watch request from the database
// after it has been processed.
func (e *ExchangeServer) removeAccountWatchRequestFromDB(requestid string) error {
	e.logger.Info("removing account watch request from db", zap.String("requestid", requestid))
	// fetch the list of current account watch requests
	requests, _ := e.redisClient.Get(context.Background(), "accountwatchrequests").Result()
	// unmarshal the list of account watch requests
	var currentRequests []AccountWatchRequest
	if requests != "" {
		err := json.Unmarshal([]byte(requests), &currentRequests)
		if err != nil {
			return err
		}
	}
	// search for the request in the list by the transaction ID
	// if it is found, remove it from the list
	for i, r := range currentRequests {
		if r.TransactionID == requestid {
			currentRequests = append(currentRequests[:i], currentRequests[i+1:]...)
		}
	}

	// marshal the list of account watch requests
	crjs, err := json.Marshal(currentRequests)
	if err != nil {
		return err
	}

	// store the new list of account watch requests
	err = e.redisClient.Set(context.Background(), "accountwatchrequests", crjs, 0).Err()
	if err != nil {
		fmt.Println(err)
		return err
	}

	e.logger.Info("removed account watch request from db", zap.String("requestid", requestid))

	return nil
}

// retrieveAccountWatchRequestsFromDB retrieves the account watch requests from the database
// so that they can be processed.
func (e *ExchangeServer) retrieveAccountWatchRequestsFromDB() ([]AccountWatchRequest, error) {
	// Fetch the list of current account watch requests
	requests, _ := e.redisClient.Get(context.Background(), "accountwatchrequests").Result()

	if len(requests) == 0 {
		return make([]AccountWatchRequest, 0), nil
	}

	var currentRequests []AccountWatchRequest
	err := json.Unmarshal([]byte(requests), &currentRequests)
	if err != nil {
		return nil, err
	}

	return currentRequests, nil
}

type BridgeStorage struct {
	Chain      string   `json:"chain"`
	PrivateKey string   `json:"privatekey"`
	Amount     *big.Int `json:"amount"`
	Asset      string   `json:"asset"`
	ID         string   `json:"id"`
	BridgeFrom string   `json:"bridgefrom"`
	BridgeTo   string   `json:"bridgeto"`
}

// storeBridgeAccount we need a function to store the private keys and the amount of each coin held in the database
// so that when users try to bring a wrapped asset back on the chain, we can fund the transaction
// without having to move funds around.
func (e *ExchangeServer) storeBridgeAccount(awrr AccountWatchRequestResult) error {
	fmt.Println("Storing bridge account")
	// create a bridge storage object out of the account watch request result
	bs := BridgeStorage{
		Chain:      awrr.AccountWatchRequest.Chain,
		PrivateKey: awrr.AccountWatchRequest.AssistedSellOrderInformation.SellersEscrowWallet.PrivateKey,
		Amount:     awrr.AccountWatchRequest.AssistedSellOrderInformation.Amount,
		Asset:      awrr.AccountWatchRequest.AssistedSellOrderInformation.Currency,
		ID:         awrr.AccountWatchRequest.TransactionID,
		BridgeFrom: awrr.AccountWatchRequest.AssistedSellOrderInformation.BridgeFrom,
		BridgeTo:   awrr.AccountWatchRequest.AssistedSellOrderInformation.BridgeTo,
	}

	// retrieve the list of bridge accounts
	accounts, _ := e.redisClient.Get(context.Background(), "bridgeaccounts").Result()

	// unmarshal the list of bridge accounts
	var currentAccounts []BridgeStorage
	if accounts != "" {
		err := json.Unmarshal([]byte(accounts), &currentAccounts)
		if err != nil {
			return err
		}
	}

	var found bool
	for i, a := range currentAccounts {
		if a.ID == bs.ID {
			currentAccounts[i] = bs
			found = true
		}
	}

	if !found {
		currentAccounts = append(currentAccounts, bs)
	}

	// marshal the updated list of bridge accounts
	updatedAccounts, err := json.Marshal(currentAccounts)
	if err != nil {
		return err
	}

	// store the updated list of bridge accounts in the database
	err = e.redisClient.Set(context.Background(), "bridgeaccounts", updatedAccounts, 0).Err()
	if err != nil {
		return err
	}

	return nil
}

// BridgeStorageSlice is a custom type to implement sort.Interface
type BridgeStorageSlice []BridgeStorage

func (b BridgeStorageSlice) Len() int           { return len(b) }
func (b BridgeStorageSlice) Less(i, j int) bool { return b[i].Amount.Cmp(b[j].Amount) == -1 }
func (b BridgeStorageSlice) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

func (e *ExchangeServer) retrieveBridgeAccount(awr AccountWatchRequestResult) (*BridgeStorage, error) {
	e.logger.Infof("retrieving bridge account for %+v", awr)

	if awr.AccountWatchRequest.Amount == nil {
		return nil, fmt.Errorf("valueSearch cannot be nil")
	}

	// Retrieve the list of bridge accounts
	accounts, _ := e.redisClient.Get(context.Background(), "bridgeaccounts").Result()

	// Unmarshal the list of bridge accounts
	var currentAccounts BridgeStorageSlice
	if accounts != "" {
		err := json.Unmarshal([]byte(accounts), &currentAccounts)
		if err != nil {
			return nil, err
		}
	}

	e.logger.Info("current accounts", zap.Any("accounts", currentAccounts))

	// Check that valueSearch is not nil and that the list of accounts is not empty
	if awr.AccountWatchRequest.Amount == nil || len(currentAccounts) == 0 {
		return nil, nil
	}

	switch awr.AccountWatchRequest.AssistedSellOrderInformation.Currency {
	case "wgrams":
		awr.AccountWatchRequest.AssistedSellOrderInformation.Currency = "grams"
	case "wocta":
		awr.AccountWatchRequest.AssistedSellOrderInformation.Currency = "octa"
	case "wbscusdt":
		awr.AccountWatchRequest.AssistedSellOrderInformation.Currency = "bscusdt"
	}

	// Filter accounts by asset and BridgeFrom, then sort them by amount
	var filteredAccounts BridgeStorageSlice
	for _, a := range currentAccounts {
		if a.Asset == awr.AccountWatchRequest.AssistedSellOrderInformation.Currency && a.BridgeTo == awr.AccountWatchRequest.AssistedSellOrderInformation.BridgeFrom {
			filteredAccounts = append(filteredAccounts, a)
		}
	}

	e.logger.Info("filteredAccounts1", zap.Any("filteredAccounts", filteredAccounts))

	// // Sort filteredAccounts by amount
	// sort.Slice(filteredAccounts, func(i, j int) bool {
	// 	return filteredAccounts[i].Amount.Cmp(filteredAccounts[j].Amount) < 0 // Ensure ascending order
	// })

	// if len(filteredAccounts) == 0 {
	// 	return nil, fmt.Errorf("no suitable bridge accounts found.")
	// }

	// e.logger.Info("filteredAccounts", zap.Any("filteredAccounts", filteredAccounts))

	// for _, a := range filteredAccounts {
	// 	if a.BridgeTo == awr.AccountWatchRequest.AssistedSellOrderInformation.BridgeFrom {
	// 		filteredAccounts = append(filteredAccounts, a)
	// 	}
	// }

	// e.logger.Info("filteredAccounts", zap.Any("filteredAccounts", filteredAccounts))

	// Perform binary search to find the closest account
	index := sort.Search(len(filteredAccounts), func(i int) bool {
		return filteredAccounts[i].Amount.Cmp(awr.AccountWatchRequest.AssistedSellOrderInformation.Amount) >= 0
	})

	e.logger.Info("index", zap.Int("index", index))

	var closestAccount *BridgeStorage
	if index == 0 {
		closestAccount = &filteredAccounts[index]
	} else if index == len(filteredAccounts) {
		closestAccount = &filteredAccounts[index-1]
	} else {
		left := &filteredAccounts[index-1]
		right := &filteredAccounts[index]
		leftDiff := new(big.Int).Sub(awr.AccountWatchRequest.AssistedSellOrderInformation.Amount, left.Amount)
		rightDiff := new(big.Int).Sub(right.Amount, awr.AccountWatchRequest.AssistedSellOrderInformation.Amount)

		if leftDiff.Cmp(rightDiff) <= 0 {
			closestAccount = left
		} else {
			closestAccount = right
		}
	}

	return closestAccount, nil
}

// updateBridgeAccountInDB updates the bridge account in the database after a transfer
// has been made so that the balance is correct
func (e *ExchangeServer) updateBridgeAccountInDB(id string, amount *big.Int) error {
	// fetch the list of current bridge accounts
	accounts, _ := e.redisClient.Get(context.Background(), "bridgeaccounts").Result()
	// unmarshal the list of bridge accounts
	var currentAccounts []BridgeStorage
	if accounts != "" {
		err := json.Unmarshal([]byte(accounts), &currentAccounts)
		if err != nil {
			return err
		}
	}

	// find the account with the specified id
	for i, a := range currentAccounts {
		if strings.Contains(a.ID, id) {
			e.logger.Info("Updating bridge account", zap.String("id", id), zap.String("amount", amount.String()))
			currentAccounts[i].Amount = new(big.Int).Sub(a.Amount, amount)
		}
	}

	// marshal the list of bridge accounts
	cas, err := json.Marshal(currentAccounts)
	if err != nil {
		return err
	}

	// store the new list of bridge accounts
	err = e.redisClient.Set(context.Background(), "bridgeaccounts", cas, 0).Err()
	if err != nil {
		return err
	}

	return nil
}

// storeFailedAccountWatchRequest stores the failed account watch request in the database
// so that it can be processed later.
func (e *ExchangeServer) storeFailedAccountWatchRequest(awr AccountWatchRequest) error {
	// fetch the list of current failed account watch requests
	requests, _ := e.redisClient.Get(context.Background(), "failedaccountwatchrequests").Result()
	// unmarshal the list of failed account watch requests
	var currentRequests []AccountWatchRequest
	if requests != "" {
		err := json.Unmarshal([]byte(requests), &currentRequests)
		if err != nil {
			return err
		}
	}

	// add the new failed account watch request to the list
	currentRequests = append(currentRequests, awr)

	// marshal the list of failed account watch requests
	crjs, err := json.Marshal(currentRequests)
	if err != nil {
		return err
	}

	// store the new list of failed account watch requests
	err = e.redisClient.Set(context.Background(), "failedaccountwatchrequests", crjs, 0).Err()
	if err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}
