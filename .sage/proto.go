package main

import (
	"context"

	"go.einride.tech/sage/sg"
	"go.einride.tech/sage/sgtool"
	"go.einride.tech/sage/tools/sgbuf"
)

type Proto sg.Namespace

func (Proto) All(ctx context.Context) error {
	sg.Deps(ctx, Proto.BufGenerateExample)
	return nil
}

func (Proto) ProtocGenGo(ctx context.Context) error {
	sg.Logger(ctx).Println("installing...")
	_, err := sgtool.GoInstallWithModfile(ctx, "google.golang.org/protobuf/cmd/protoc-gen-go", sg.FromGitRoot("go.mod"))
	return err
}

func (Proto) ProtocGenGoGRPC(ctx context.Context) error {
	sg.Logger(ctx).Println("installing...")
	_, err := sgtool.GoInstall(ctx, "google.golang.org/grpc/cmd/protoc-gen-go-grpc", "v1.2.0")
	return err
}

func (Proto) ProtocGenGoAIPDataloader(ctx context.Context) error {
	sg.Logger(ctx).Println("building binary...")
	return sg.Command(ctx, "go", "build", "-o", sg.FromBinDir("protoc-gen-go-aip-dataloader"), ".").Run()
}

func (Proto) BufGenerateExample(ctx context.Context) error {
	sg.Deps(ctx, Proto.ProtocGenGo, Proto.ProtocGenGoGRPC, Proto.ProtocGenGoAIPDataloader)
	sg.Logger(ctx).Println("generating example proto stubs...")
	return sgbuf.Command(
		ctx,
		"generate",
		"buf.build/einride/aip",
		"--template",
		"buf.gen.example.yaml",
		"--path",
		"einride/example/freight",
	).Run()
}
