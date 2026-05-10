package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/AyeshaaAyub/datarouter/proto"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type RedisHandler struct {
	proto.UnimplementedStorageHandlerServer
	client *redis.Client
}

func (h *RedisHandler) Store(ctx context.Context, req *proto.StoreRequest) (*proto.StoreResponse, error) {
	if req.GetCollection() == "" {
		req.Collection = "default"
	}

	doc, err := decodeProtoData(req.Data)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid payload: %v", err)
	}

	payload, err := json.Marshal(doc)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal payload: %v", err)
	}

	id := fmt.Sprintf("redis-%d", time.Now().UnixNano())
	key := fmt.Sprintf("%s:%s", req.GetCollection(), id)

	if err := h.client.Set(ctx, key, payload, 0).Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to store value: %v", err)
	}

	return &proto.StoreResponse{Id: key, DbType: "redis"}, nil
}

func (h *RedisHandler) Retrieve(ctx context.Context, req *proto.RetrieveRequest) (*proto.RetrieveResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	payload, err := h.client.Get(ctx, req.GetId()).Bytes()
	if err == redis.Nil {
		return nil, status.Error(codes.NotFound, "key not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read from redis: %v", err)
	}

	var doc map[string]interface{}
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to decode payload: %v", err)
	}

	return &proto.RetrieveResponse{Data: convertToProtoData(doc), DbType: "redis"}, nil
}

func decodeProtoData(data *proto.Data) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(data.GetFields()))
	for key, raw := range data.GetFields() {
		var value interface{}
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		result[key] = value
	}
	return result, nil
}

func convertToProtoData(data map[string]interface{}) *proto.Data {
	fields := make(map[string][]byte, len(data))
	for key, value := range data {
		if bytes, err := json.Marshal(value); err == nil {
			fields[key] = bytes
		}
	}
	return &proto.Data{Fields: fields}
}

func main() {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	client := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Fatalf("failed to connect to Redis: %v", err)
	}

	handler := &RedisHandler{client: client}
	lis, err := net.Listen("tcp", ":50053")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	server := grpc.NewServer()
	proto.RegisterStorageHandlerServer(server, handler)
	log.Printf("redis handler listening on :50053")
	if err := server.Serve(lis); err != nil {
		log.Fatalf("grpc server failed: %v", err)
	}
}
