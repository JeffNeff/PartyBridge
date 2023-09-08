https://cert-manager.io/docs/installation/kubectl/


kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.12.0/cert-manager.yaml



Configure Let's Encrypt issuer: With cert-manager installed, you will now need to configure an "Issuer", which is a description of the certificate authority from which we will issue certificates. Create a new file letsencrypt-issuer.yaml:

yaml
Copy code
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt
spec:
  acme:
    # Email address used for ACME registration
    email: your-email@example.com
    # Name of the ACME server to use
    server: https://acme-v02.api.letsencrypt.org/directory
    privateKeySecretRef:
      # Secret resource used to store the account's private key
      name: letsencrypt
    # Add a single challenge solver, HTTP01 using nginx
    solvers:
    - http01:
        ingress:
          class: nginx
Replace your-email@example.com with your email address. Apply it to your cluster with the command:

Copy code
kubectl apply -f letsencrypt-issuer.yaml
Issue a certificate: With your Issuer in place, you can now issue a certificate for your domain. Create a new file certificate.yaml:

yaml
Copy code
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example-com
  namespace: default
spec:
  secretName: example-com-tls
  issuerRef:
    name: letsencrypt
    kind: ClusterIssuer
  commonName: www.example.com
  dnsNames:
  - www.example.com
  - example.com
Replace example.com and www.example.com with your domain name, then apply it to your cluster:

Copy code
kubectl apply -f certificate.yaml
Update your Ingress: Now you need to modify your Ingress object to use the certificate. Make sure you specify the correct tls.secretName in your Ingress object:

yaml
Copy code

apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: partybridge-ingress
spec:
  tls:
  - hosts:
      - partybridge.io
    secretName: partybridge-io-tls
  rules:
  - host: partybridge.io
    http:
      paths:
      - backend:
          service:
            name: partybridge
            port:
              number: 8080

Remember to replace example.com and example-service with your domain name and service name.

With this setup, cert-manager will automatically renew your Let's Encrypt certificates and Kubernetes will automatically pick up the new certificates when they are renewed.