# protoc-gen-go-dataloader

Generate [GraphQL][graphql] Go [dataloaders][dataloader] for [AIP][aip]
[BatchGet][batch-get] methods.

Heavily inspired by [vektah/dataloaden][dataloaden].

[graphql]: https://graphql.org
[dataloader]: https://github.com/graphql/dataloader
[aip]: https://google.aip.dev
[batch-get]: https://google.aip.dev/231
[dataloaden]: https://github.com/vektah/dataloaden

## How to

### Step 1: Declare a Batch method in your API

```proto
  // Batch get sites.
  // See: https://google.aip.dev/231 (Batch methods: Get).
  rpc BatchGetSites(BatchGetSitesRequest) returns (BatchGetSitesResponse) {
    option (google.api.http) = {
      get: "/v1/{parent=shippers/*}/sites:batchGet"
    };
  }
```

### Step 2: Install the plugin

```bash
$ GOBIN=$PWD/build go install go.einride.tech/protoc-gen-go-dataloader
```

### Step 3: Configure code generation

The following example uses a [buf generate][buf-generate] template to
configure the CLI generator.

[buf-generate]: https://docs.buf.build/generate/usage

[buf.gen.example.yaml](./proto/buf.gen.example.yaml):

```yaml
version: v1

managed:
  enabled: true
  go_package_prefix:
    default: go.einride.tech/protoc-gen-go-dataloader/example/internal/proto/gen
    except:
      - buf.build/googleapis/googleapis

plugins:
  # Dataloaders require the stubs generated by protoc-gen-go.
  - name: go
    out: example/internal/proto/gen
    opt: module=go.einride.tech/protoc-gen-go-dataloader/example/internal/proto/gen
    path: ./build/protoc-gen-go

  # Dataloaders require the stubs generated by protoc-gen-go-grpc.
  - name: go-grpc
    out: example/internal/proto/gen
    opt: module=go.einride.tech/protoc-gen-go-dataloader/example/internal/proto/gen
    path: ./build/protoc-gen-go-grpc

  # Generate dataloaders for Batch methods.
  - name: go-dataloader
    out: example/internal/proto/gen
    opt: module=go.einride.tech/protoc-gen-go-dataloader/example/internal/proto/gen
    path: ./build/protoc-gen-go-dataloader
```

### Step 4: Generate the code

```bash
$ buf generate buf.build/einride/aip \
  --template buf.gen.example.yaml \
  --path einride/example/freight
```

### Step 5: Use the dataloader

```go
package main

import (
	"context"
	"time"

	freightv1 "go.einride.tech/protoc-gen-go-dataloader/example/internal/proto/gen/einride/example/freight/v1"
	"google.golang.org/grpc"
)

func main() {
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
```
