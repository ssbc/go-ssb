package main

import (
	"context"
	"fmt"

	"cryptoscope.co/go/muxrpc"
	"github.com/pkg/errors"
)

func (edp *whoAmIEndpoint) WhoAmI(ctx context.Context) ([]byte, error) {
	v, err := edp.Async(ctx, []byte{}, []string{"whoami"})
	if err != nil {
		return nil, errors.Wrap(err, "error doing async call")
	}

	key, ok := v.([]byte)
	if !ok {
		return nil, errors.Errorf("expected type %T, got %T", key, v)
	}

	return key, nil
}

type whoAmI struct {
	PubKey []byte
}

func (whoAmI) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {
	remote := &whoAmIEndpoint{edp}
	fmt.Println("calling whoami")
	key, err := remote.WhoAmI(ctx)
	if err != nil {
		fmt.Println(errors.Wrap(err, "error calling whoami on remote"))
		return
	}

	fmt.Printf("connected to %x\n", key)
	return
}

func (wami whoAmI) HandleCall(ctx context.Context, req *muxrpc.Request) {
	fmt.Println("incoming call")

	err := req.Return(ctx, wami.PubKey)
	if err != nil {
		fmt.Println("error returning value:", err)
		return
	}

	fmt.Printf("sending %x\n", wami.PubKey)
	return
}
