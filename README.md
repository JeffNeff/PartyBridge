

## requestBridge

### GRAMS -> OCTA

curl -v "http://192.168.50.23:8080/requestBridge" \
       -X POST \
       -H "Content-Type: application/json" \
       -d '{"currency":"grams","fromChain":"grams", "amount": 10000, "bridgeTo":"octa","shippingAddress":"0x5bbfa5724260Cb175cB39b24802A04c3bfe72eb3"}'

curl -v "http://0.0.0.0:8080/requestBridge" \
       -X POST \
       -H "Content-Type: application/json" \
       -d '{"currency":"wgrams","fromChain":"octa", "amount": 10000, "bridgeTo":"grams","shippingAddress":"0x5bbfa5724260Cb175cB39b24802A04c3bfe72eb3"}'
  
### OCTA -> GRAMS

curl -v "http://0.0.0.0:8080/requestBridge" \
       -X POST \
       -H "Content-Type: application/json" \
       -d '{"currency":"octa","fromChain":"octa", "amount": 10000, "bridgeTo":"grams","shippingAddress":"0x5bbfa5724260Cb175cB39b24802A04c3bfe72eb3"}'

curl -v "http://0.0.0.0:8080/requestBridge" \
       -X POST \
       -H "Content-Type: application/json" \
       -d '{"currency":"wocta","fromChain":"grams", "amount": 10000, "bridgeTo":"octa","shippingAddress":"0x5bbfa5724260Cb175cB39b24802A04c3bfe72eb3"}'


### BSCUSDT -> GRAMS

curl -v "http://0.0.0.0:8080/requestBridge" \
       -X POST \
       -H "Content-Type: application/json" \
       -d '{"currency":"grams","fromChain":"bscusdt", "amount": 10000, "bridgeTo":"grams","shippingAddress":"0x5bbfa5724260Cb175cB39b24802A04c3bfe72eb3"}'





Generate the private key:

use the script located in /tls folder

##

```
@startuml
User -> Bridge : Connect WS
Bridge -> User: {"type":"hello","sid":"27544e25-e188-48f8-9962-6990a96e21cd","fee":12,"minimumAmount":10}
User -> Bridge: {"type": "requestBridge", "data": {"currency":"octa","amount":11000000000000000000,"fromChain":"octa","bridgeTo":"grams","shippingAddress":"0x5eb565b14b39171c187d5a260789685042e85eca"}}
Bridge -> User: {"type":"requestBridgeResponse","amount":23000000000000000000,"address":"0x1bb31C541CeEaA6ce55937BF36542aa079e04DE5"}
User -> Bridge: {"type": "confirmBridge", "tx": "0x1d4aaa350099df1c302639816e18c4163c8fad7ccde485fa8914059f1100787b"}
Bridge -> User: Disconnect WS
@enduml
```
