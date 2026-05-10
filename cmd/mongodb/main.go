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
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MongoHandler struct {
	proto.UnimplementedStorageHandlerServer
	client *mongo.Client
}

func (h *MongoHandler) Store(ctx context.Context, req *proto.StoreRequest) (*proto.StoreResponse, error) {
	if req.GetCollection() == "" {
		req.Collection = "default"
	}

	doc, err := decodeProtoData(req.Data)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid payload: %v", err)
	}

	coll := h.client.Database("datarouter").Collection(req.GetCollection())
	result, err := coll.InsertOne(ctx, doc)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to store document: %v", err)
	}

	return &proto.StoreResponse{Id: fmt.Sprintf("%v", result.InsertedID), DbType: "mongodb"}, nil
}

func (h *MongoHandler) Retrieve(ctx context.Context, req *proto.RetrieveRequest) (*proto.RetrieveResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if req.GetCollection() == "" {
		req.Collection = "default"
	}

	coll := h.client.Database("datarouter").Collection(req.GetCollection())
	filter := bson.M{"_id": req.GetId()}
	if oid, err := primitive.ObjectIDFromHex(req.GetId()); err == nil {
		filter = bson.M{"_id": oid}
	}

	var doc bson.M
	err := coll.FindOne(ctx, filter).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, status.Error(codes.NotFound, "document not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve document: %v", err)
	}

	if _, ok := doc["_id"]; ok {
		doc["_id"] = fmt.Sprintf("%v", doc["_id"])
	}

	return &proto.RetrieveResponse{Data: convertToProtoData(doc), DbType: "mongodb"}, nil
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
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatalf("failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("failed to ping MongoDB: %v", err)
	}

	handler := &MongoHandler{client: client}
	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	server := grpc.NewServer()
	proto.RegisterStorageHandlerServer(server, handler)
	log.Printf("mongodb handler listening on :50052")
	if err := server.Serve(lis); err != nil {
		log.Fatalf("grpc server failed: %v", err)
	}
}
