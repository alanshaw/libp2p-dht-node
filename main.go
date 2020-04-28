package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	leveldb "github.com/ipfs/go-ds-leveldb"
	"github.com/ipfs/go-ipns"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/peer"
	crypto "github.com/libp2p/go-libp2p-crypto"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	record "github.com/libp2p/go-libp2p-record"
	"github.com/multiformats/go-multiaddr"
)

const (
	lowWater    = 1200
	highWater   = 1800
	gracePeriod = time.Minute
)

var bootstrapPeers = []string{
	"/dnsaddr/sjc-2.bootstrap.libp2p.io/p2p/QmZa1sAxajnQjVM8WjWXoMbmPd7NsWhfKsPkErzpm9wGkp",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
	"/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
}

func main() {
	ctx := context.Background()
	cmgr := connmgr.NewConnManager(lowWater, highWater, gracePeriod)

	priv, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 0)
	if err != nil {
		panic(err)
	}

	addr, _ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/0")

	libp2pOpts := []libp2p.Option{
		libp2p.UserAgent(""),
		libp2p.ListenAddrs(addr),
		libp2p.ConnectionManager(cmgr),
		libp2p.Identity(priv),
		// libp2p.EnableNATService(),
		// libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport),
		// libp2p.AutoNATServiceRateLimit(0, 3, time.Minute),
	}

	node, err := libp2p.New(ctx, libp2pOpts...)
	if err != nil {
		panic(err)
	}

	for _, addr := range node.Addrs() {
		fmt.Printf("Listening on: %v\n", addr)
	}

	ds, err := leveldb.NewDatastore("./data", nil)
	if err != nil {
		panic(err)
	}

	dhtOpts := []dht.Option{
		dht.Mode(dht.ModeServer),
		dht.BucketSize(20),
		dht.Datastore(ds),
		dht.QueryFilter(dht.PublicQueryFilter),
		dht.RoutingTableFilter(dht.PublicRoutingTableFilter),
		dht.V1CompatibleMode(true),
		dht.Validator(record.NamespacedValidator{
			"pk":   record.PublicKeyValidator{},
			"ipns": ipns.Validator{KeyBook: node.Peerstore()},
		}),
	}

	dhtNode, err := dht.New(ctx, node, dhtOpts...)
	if err != nil {
		panic(err)
	}

	// bootstrap in the background
	// it's safe to start doing this _before_ establishing any connections
	// as we'll trigger a boostrap round as soon as we get a connection anyways.
	dhtNode.Bootstrap(ctx)

	go func() {
		var wg sync.WaitGroup
		wg.Add(len(bootstrapPeers))
		for _, addr := range bootstrapPeers {
			go func(addr string) {
				defer wg.Done()
				ma, _ := multiaddr.NewMultiaddr(addr)
				ai, _ := peer.AddrInfoFromP2pAddr(ma)
				if err := node.Connect(context.Background(), *ai); err != nil {
					fmt.Printf("failed to bootstrap to %v: %v\n", addr, err)
					return
				}
				node.ConnManager().Protect(ai.ID, "bootstrap-peer")
			}(addr)
		}
		wg.Wait()
	}()
}
