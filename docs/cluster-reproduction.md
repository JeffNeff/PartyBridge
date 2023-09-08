1. provision cluster



1. install ingress-nginx
```
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.1/deploy/static/provider/cloud/deploy.yaml
```

1. retrieve ingress-nginx-controller external ip
```
kubectl get service ingress-nginx-controller --namespace=ingress-nginx | awk '{print $4}' | tail -n 1
```

1. update dns record
```
A record: partybridge.io -> ingress-nginx-controller external ip
```

1. install partybridge
```
kubectl apply -f config/3-party/staging/shims
kubectl expose deployment partyshim-wgrams --type=LoadBalancer --name=partyshim-wgrams
kubectl expose deployment partyshim-octaspace-bscusdt --type=LoadBalancer --name=partyshim-octaspace-bscusdt
kubectl expose deployment partyshim-partychain-bscusdt --type=LoadBalancer --name=partyshim-partychain-bscusdt
kubectl expose deployment partyshim-partychain-wocta --type=LoadBalancer --name=partyshim-partychain-wocta
kubectl apply -f config/3-party/staging/
```

1. install cert-manager
```
kubectl apply -f config/3-party/staging/cert/config
```

1. configure cert-manager
```
kubectl apply -f config/3-party/staging/cert
```

1. expose partybridge
```
kubectl  expose deployment partybridge --type=LoadBalancer --name=partybridgelb-prod