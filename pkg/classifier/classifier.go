package classifier

import (
	"strconv"
	"time"
)

// DBType enum
type DBType string

const (
	DBPostgres DBType = "postgres"
	DBMongo    DBType = "mongodb"
	DBRedis    DBType = "redis"
	DBInflux   DBType = "influxdb"
)

// Classify assesses data nature
func Classify(data map[string]interface{}) DBType {
	if hasTimestamp(data) {
		return DBInflux // Time-series
	}
	if len(data) <= 2 && isSimpleKV(data) {
		return DBRedis // Key-value
	}
	if isStructured(data) {
		return DBPostgres // Fixed schema (structured)
	}
	return DBMongo // Flexible documents by default
}

func hasTimestamp(data map[string]interface{}) bool {
	if _, ok := data["timestamp"].(time.Time); ok {
		return true
	}
	if val, ok := data["timestamp"].(string); ok {
		_, err := parseTimestamp(val)
		return err == nil
	}
	return false
}

func parseTimestamp(value string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, strconv.ErrSyntax
}

func isSimpleKV(data map[string]interface{}) bool {
	if len(data) == 0 {
		return false
	}
	for _, value := range data {
		switch value.(type) {
		case string, bool, int, int32, int64, float32, float64:
			continue
		default:
			return false
		}
	}
	return true
}

func isStructured(data map[string]interface{}) bool {
	return len(data) >= 4
}
