#!/usr/bin/env python3

import requests
from web3 import Web3
import os
import json
import logging
import sys
import configparser
import time
import websocket
import argparse
from threading import Thread

stdout_handler = logging.StreamHandler(stream=sys.stdout)
logging.basicConfig(
    level=logging.INFO,
    format='[%(asctime)s] {%(filename)s:%(lineno)d} %(levelname)s - %(message)s',
    handlers=[stdout_handler]
)
logger = logging.getLogger(__name__)

config = configparser.ConfigParser()
config.read(os.path.join(os.path.abspath(os.path.dirname(__file__)), 'config.ini'))

parser = argparse.ArgumentParser(description="uptime calculation")
parser.add_argument("-t", "--tests", help="comma separated list of tests", default=False)
args = parser.parse_args()

with open('bridge_abi.json') as abi_json:
    bridge_abi = json.load(abi_json)

def on_message(ws, message):
    logger.info(f"WebSocket message received: {message}")

def on_error(ws, error):
    logger.error(f"WebSocket error: {error}")

def on_close(ws, close_status_code, close_reason):
    logger.info("WebSocket closed")

def tests():
    return config['common']['tests'].split(',')

def websocket_listen(url, client_id):
    global ws
    print(f"Connecting to WebSocket: {url}?id={client_id}")
    ws = websocket.WebSocketApp(
        f"{url}?id={client_id}",
        on_message=on_message,
        on_error=on_error,
        on_close=on_close,
    )
    ws.run_forever()

def stop():
    ws.close()

def request_bridge(currency, from_chain, to_chain, amount, shipping_address, client_id):
    data = {
        'currency': currency,
        'fromChain': from_chain,
        'bridgeTo': to_chain,
        'amount': amount,
        'shippingAddress': shipping_address,
    }
    url = f"{config['common']['bridge']}?id={client_id}"
    r = requests.post(url, json=data)
    if r.status_code == 200:
        return r.json()
    else:
        sys.exit(r)

def balance_coin(rpc, address):
    w3 = Web3(Web3.HTTPProvider(rpc))
    address = w3.to_checksum_address(address)
    return w3.eth.get_balance(address)

def balance_contract(rpc, contract, address):
    w3 = Web3(Web3.HTTPProvider(rpc))
    contract = w3.eth.contract(address=contract, abi=bridge_abi)
    return contract.functions.balanceOf(w3.to_checksum_address(address)).call()

def tx(rpc, _a, _b, amount, pk, tx_type, token_contract=None):
    w3 = Web3(Web3.HTTPProvider(rpc))
    chain_id = w3.eth.chain_id
    a = w3.to_checksum_address(_a)
    b = w3.to_checksum_address(_b)

    nonce = w3.eth.get_transaction_count(a, "pending")

    if token_contract:
        token_contract_address = w3.to_checksum_address(token_contract)
        token_contract_instance = w3.eth.contract(address=token_contract_address, abi=bridge_abi)

        print(pk)

        build_tx = token_contract_instance.functions.transfer(b, amount).build_transaction({
            'from': a,
            'nonce': nonce,
            'gasPrice': w3.to_wei(250, 'gwei')
        })

        print("im here")

        signed_tx = w3.eth.account.sign_transaction(build_tx, pk)

    else:
        gas = w3.eth.estimate_gas({
            'from': a,
            'to': b,
            'value': amount
        })

        if tx_type == 2:
            signed_tx = w3.eth.account.sign_transaction(
                dict(
                    nonce                = nonce,
                    to                   = b,
                    value                = amount,
                    gas                  = gas,
                    chainId              = chain_id,
                    maxFeePerGas         = 3000000000,
                    maxPriorityFeePerGas = 2000000000
                ),
                pk
            )
        elif tx_type == 1:
            signed_tx = w3.eth.account.sign_transaction(
                dict(
                    nonce    = nonce,
                    to       = b,
                    value    = amount,
                    gas      = gas,
                    gasPrice = w3.to_wei(1, 'gwei'),
                    chainId  = chain_id
                ),
                pk
            )

    txn = w3.eth.send_raw_transaction(signed_tx.rawTransaction)

    logger.info("tx sent, a: {}, b: {}, amount: {}({}), tx: {}, nonce: {}".format(a, b, amount, Web3.from_wei(amount, 'ether'), txn.hex(), nonce))
    logger.info("wait for receipt, tx: {}".format(txn.hex()))
    rcpt = w3.eth.wait_for_transaction_receipt(txn)
    logger.info("tx mined, tx: {}, block: {}".format(txn.hex(), rcpt['blockNumber']))

def run(c):
    # Start a new thread to listen for WebSocket messages
    ws_thread = Thread(target=websocket_listen, args=(c['websocket_url'], c['client_id']))
    ws_thread.start()

    time.sleep(1)
    
    token_contract = c.get('token_contract', None)

    reply = request_bridge(c['currency'], c['from_chain'], c['to_chain'], int(c['amount']), c['shipping_address'], c['client_id'])
    deposit_address = reply['address']
    logger.info("deposit_address: {}".format(deposit_address))

    if token_contract:
        balance = balance_coin(c['rpc2'], c['shipping_address'])
    else:
        balance = balance_contract(c['rpc2'], c['bridge_sc'], c['shipping_address'])

    logger.info("balance of {} is {}({})".format(c['shipping_address'], balance, Web3.from_wei(balance, 'ether')))
    
    tx(c['rpc1'], c['shipping_address'], deposit_address, int(c['amount']), c['shipping_address_pk'], int(c['tx_type']), token_contract=token_contract)

    balance_should_be = balance + int(c['amount'])
    # subtract .1 from balance_should_be to account for gas fees
    balance_should_be = balance_should_be - .1 * 10**18
    logger.info("waiting of balance update, should be {}({})".format(balance_should_be, Web3.from_wei(balance_should_be, 'ether')))

    is_test_passed = False
    for i in range(180):
        if token_contract:
            n = balance_coin(c['rpc2'], c['shipping_address'])
        else:
            n = balance_contract(c['rpc2'], c['bridge_sc'], c['shipping_address'])
        if n >= balance_should_be:
            logger.info("new balance {}".format(n))
            is_test_passed = True
            break
        else:
            time.sleep(1)
    # Stop the WebSocket listener thread
    stop()

    if is_test_passed:
        logger.info("TEST PASSED")
    else:
        logger.error("TEST FAILED")

def run_tests(tests):
    for t in tests:
        if t in config:
            logging.info("run {}".format(t))
            run(config[t])
        else:
            logging.warning("test {} not found".format(t))

if __name__ == '__main__':
    if args.tests:
        run_tests(args.tests.split(','))
    else:
        run_tests(config['common']['tests'].split(','))
