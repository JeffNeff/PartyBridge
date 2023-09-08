package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

type MintRequest struct {
	Amount    uint64 `json:"amount"`
	ToAddress string `json:"toAddress"`
}

func main() {
	// Load client certificate and key pair
	cert, err := tls.LoadX509KeyPair("client.crt", "client.key")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Load the server's CA certificate
	caCert, err := ioutil.ReadFile("ca.crt")
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

	// Create a MintRequest struct with the request data
	mintRequest := MintRequest{
		Amount:    10000,
		ToAddress: "0x5dd4039c32F6EEF427D6F67600D8920c9631D59D",
	}
	jsn, err := json.Marshal(mintRequest)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Send a POST request to the server
	url := "https://192.168.50.148:8080/mint"
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsn))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer res.Body.Close()

	// Process the response
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if res.StatusCode == http.StatusOK {
		var txid map[string]string
		err := json.Unmarshal(body, &txid)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Printf("Transaction ID: %s\n", txid["txid"])
	} else {
		fmt.Printf("Failed to mint wrapped currency: %s\n", res.Status)
	}
}
