package evm

import (
	"github.com/ethereum/go-ethereum/ethclient"
)

// NewClient creates a new Ethereum client
func NewClient(rpcURL string) (*ethclient.Client, error) {
	return ethclient.Dial(rpcURL)
}
