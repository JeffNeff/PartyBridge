#!/bin/bash
set -e

# Create openssl.cnf
cat <<EOF >openssl.cnf
[ req ]
default_bits        = 2048
default_keyfile     = server-key.pem
distinguished_name  = req_distinguished_name
req_extensions      = req_ext
prompt = no

[ req_distinguished_name ]
countryName                 = AU
stateOrProvinceName         = Some-State
localityName               = City
organizationName           = company
commonName                 = Internet Widgits Pty Ltd

[ req_ext ]
subjectAltName          = @alt_names

[alt_names]
DNS.1   = partyshim-wgrams
DNS.2   = partyshim-partychain-wocta
DNS.3   = partyshim-partychain-bscusdt
DNS.4   = partyshim-octaspace-bscusdt
DNS.5   = partybridge

EOF

# Generate keys and certificates
openssl genrsa -out ca.key 2048
openssl req -new -x509 -days 3650 -key ca.key -out ca.crt

openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr -config openssl.cnf
openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -extensions req_ext -extfile openssl.cnf

openssl genrsa -out client.key 2048
openssl req -new -key client.key -out client.csr
openssl x509 -req -days 365 -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out client.crt

# Create the Kubernetes secret YAML
cat <<EOF >mtls-secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: mtls-secret
type: Opaque
data:
  ca.crt: $(cat ca.crt | base64 | tr -d '\n')
  client.crt: $(cat client.crt | base64 | tr -d '\n')
  client.key: $(cat client.key | base64 | tr -d '\n')
  server.crt: $(cat server.crt | base64 | tr -d '\n')
  server.key: $(cat server.key | base64 | tr -d '\n')
EOF

echo "Created mtls-secret.yaml"
