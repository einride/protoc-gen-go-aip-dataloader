package example

import (
	"context"
	"time"

	freightv1 "go.einride.tech/protoc-gen-go-dataloader/example/internal/proto/gen/einride/example/freight/v1"
	"google.golang.org/grpc"
)

func Example_dataloader() {
	ctx := context.Background()
	// Connect to service.
	conn, err := grpc.Dial("ADDRESS")
	if err != nil {
		panic(err) // TODO: Handle error.
	}
	// Create a client.
	client := freightv1.NewFreightServiceClient(conn)
	// Wrap the client in a dataloader.
	sitesDataloader := freightv1.NewSitesDataloader(
		ctx,
		client,
		&freightv1.BatchGetSitesRequest{},
		100*time.Millisecond, // wait
		1_000,                // max batch
	)
	// The dataloader can batch together Get calls into BatchGet calls and return thunks.
	// This is useful in e.g. GraphQL resolver implementations.
	thunk1 := sitesDataloader.LoadThunk("shippers/folkfood/sites/sthlm")
	thunk2 := sitesDataloader.LoadThunk("shippers/folkfood/sites/gbg")
	// Evaluate the thunks to wait for the BatchGet call to go through.
	sthlm, err := thunk1()
	if err != nil {
		panic(err)
	}
	gbg, err := thunk2()
	if err != nil {
		panic(err)
	}
	_, _ = sthlm, gbg
}
