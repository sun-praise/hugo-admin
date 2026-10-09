// testplugin 是测试用的真 gRPC 插件二进制：实现 PluginService +
// ImageUploader + TTSGenerator，行为固定供集成测试断言。
// 构建：go build ./cmd/testplugin -o <plugin-dir>/testplugin
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	pb "github.com/svtter/hugo-admin/proto"
)

type server struct {
	pb.UnimplementedPluginServiceServer
	pb.UnimplementedImageUploaderServer
	pb.UnimplementedTTSGeneratorServer

	configJSON string
}

func (s *server) HealthCheck(ctx context.Context, e *pb.Empty) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{Healthy: true}, nil
}

func (s *server) Info(ctx context.Context, e *pb.Empty) (*pb.PluginInfo, error) {
	return &pb.PluginInfo{
		Name: "test-plugin", Version: "1.0.0",
		Description: "集成测试插件", Author: "tester",
		ProtocolVersion: "1",
		Capabilities:    []string{"image_upload", "tts_generation"},
		Priority:        10,
	}, nil
}

func (s *server) GetConfigSchema(ctx context.Context, e *pb.Empty) (*pb.ConfigSchemaResponse, error) {
	return &pb.ConfigSchemaResponse{
		SchemaJson: `{"type":"object","properties":{"api_key":{"type":"string"}}}`,
	}, nil
}

func (s *server) SetConfig(ctx context.Context, req *pb.SetConfigRequest) (*pb.SetConfigResponse, error) {
	s.configJSON = req.ConfigJson
	return &pb.SetConfigResponse{Success: true}, nil
}

func (s *server) Upload(stream pb.ImageUploader_UploadServer) error {
	var filename string
	for {
		chunk, err := stream.Recv()
		if err != nil {
			return err
		}
		if chunk.Filename != "" {
			filename = chunk.Filename
		}
		if chunk.IsLast {
			return stream.SendAndClose(&pb.ImageUploadResponse{
				Success: true,
				Url:     "https://cdn.test-plugin.example.com/" + filename,
				ImageId: "img-" + filename,
				Message: "ok",
			})
		}
	}
}

func (s *server) Delete(ctx context.Context, req *pb.ImageDeleteRequest) (*pb.ImageDeleteResponse, error) {
	return &pb.ImageDeleteResponse{Success: true, Message: "deleted " + req.ImageId}, nil
}

type ttsServer struct {
	pb.UnimplementedTTSGeneratorServer
}

func (s *ttsServer) Delete(ctx context.Context, req *pb.TTSDeleteRequest) (*pb.TTSDeleteResponse, error) {
	return &pb.TTSDeleteResponse{Success: true, Message: "deleted " + req.AudioId}, nil
}

func (s *ttsServer) Generate(req *pb.TTSRequest, stream pb.TTSGenerator_GenerateServer) error {
	if err := stream.Send(&pb.TTSResponse{Payload: &pb.TTSResponse_Progress{
		Progress: &pb.TTSProgress{Stage: "synthesizing", Percent: 50, Message: "half"},
	}}); err != nil {
		return err
	}
	return stream.Send(&pb.TTSResponse{Payload: &pb.TTSResponse_Result{
		Result: &pb.TTSResult{
			Success: true, Url: "https://cdn.test-plugin.example.com/audio.mp3",
			DurationSeconds: 12.5, AudioId: "audio-1", Format: "mp3",
		},
	}})
}

func main() {
	port := flag.Int("port", 0, "listen port")
	flag.Parse()
	if *port == 0 {
		fmt.Fprintln(os.Stderr, "--port required")
		os.Exit(1)
	}

	lis, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	grpcSrv := grpc.NewServer()
	s := &server{}
	pb.RegisterPluginServiceServer(grpcSrv, s)
	pb.RegisterImageUploaderServer(grpcSrv, s)
	pb.RegisterTTSGeneratorServer(grpcSrv, &ttsServer{})

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-stop
		grpcSrv.GracefulStop()
	}()
	if err := grpcSrv.Serve(lis); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
