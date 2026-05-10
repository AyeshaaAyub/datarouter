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
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type InfluxHandler struct {
	proto.UnimplementedStorageHandlerServer
	client   influxdb2.Client
	writeAPI api.WriteAPIBlocking
	queryAPI api.QueryAPI
	org      string
	bucket   string
}

func (h *InfluxHandler) Store(ctx context.Context, req *proto.StoreRequest) (*proto.StoreResponse, error) {
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

	id := fmt.Sprintf("influx-%d", time.Now().UnixNano())
	timestamp := time.Now()
	if ts, ok := doc["timestamp"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			timestamp = parsed
		}
	}

	point := influxdb2.NewPoint(
		req.GetCollection(),
		map[string]string{"id": id},
		map[string]interface{}{"payload": string(payload)},
		timestamp,
	)

	if err := h.writeAPI.WritePoint(ctx, point); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to write point: %v", err)
	}

	return &proto.StoreResponse{Id: id, DbType: "influxdb"}, nil
}

func (h *InfluxHandler) Retrieve(ctx context.Context, req *proto.RetrieveRequest) (*proto.RetrieveResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if req.GetCollection() == "" {
		req.Collection = "default"
	}

	flux := fmt.Sprintf(`from(bucket:"%s") |> range(start: -30d) |> filter(fn: (r) => r["_measurement"] == "%s" and r["id"] == "%s" and r["_field"] == "payload") |> last()`,
		h.bucket, req.GetCollection(), req.GetId())

	result, err := h.queryAPI.Query(ctx, flux)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "query failed: %v", err)
	}
	defer result.Close()

	var payloadStr string
	found := false
	for result.Next() {
		if value := result.Record().Value(); value != nil {
			if s, ok := value.(string); ok {
				payloadStr = s
				found = true
				break
			}
		}
	}
	if result.Err() != nil {
		return nil, status.Errorf(codes.Internal, "query iteration failed: %v", result.Err())
	}
	if !found {
		return nil, status.Error(codes.NotFound, "point not found")
	}

	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(payloadStr), &doc); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to decode payload: %v", err)
	}

	return &proto.RetrieveResponse{Data: convertToProtoData(doc), DbType: "influxdb"}, nil
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
	url := os.Getenv("INFLUX_URL")
	if url == "" {
		url = "http://localhost:8086"
	}
	token := os.Getenv("INFLUX_TOKEN")
	if token == "" {
		token = "my-token"
	}
	org := os.Getenv("INFLUX_ORG")
	if org == "" {
		org = "example-org"
	}
	bucket := os.Getenv("INFLUX_BUCKET")
	if bucket == "" {
		bucket = "datarouter"
	}

	client := influxdb2.NewClient(url, token)
	writeAPI := client.WriteAPIBlocking(org, bucket)
	queryAPI := client.QueryAPI(org)

	handler := &InfluxHandler{client: client, writeAPI: writeAPI, queryAPI: queryAPI, org: org, bucket: bucket}
	lis, err := net.Listen("tcp", ":50054")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	server := grpc.NewServer()
	proto.RegisterStorageHandlerServer(server, handler)
	log.Printf("influx handler listening on :50054")
	if err := server.Serve(lis); err != nil {
		log.Fatalf("grpc server failed: %v", err)
	}
}
