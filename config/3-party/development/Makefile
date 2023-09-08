stop:
	@kubectl delete -f 0-bridgebarrel.yaml && kubectl delete -f 3-partybridge.yaml 
start:
	@kubectl apply -f 0-bridgebarrel.yaml && kubectl apply -f 3-partybridge.yaml
watch:
	@kubectl logs -l  app=partybridge -f
