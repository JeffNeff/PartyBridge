package main

import (
	pkgadapter "knative.dev/eventing/pkg/adapter/v2"

	be "github.com/TeaPartyCrypto/partybridge/pkg"
)

func main() {
	pkgadapter.Main("partybridge-adapter", be.EnvAccessorCtor, be.NewAdapter)
}
