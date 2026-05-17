package handler

import (
	"strings"
	"testing"
)

func TestActivityStreamsCompressionSQLUsesTimeOrderColumn(t *testing.T) {
	if strings.Contains(activityStreamsCompressionSQL, "timestamp") {
		t.Fatalf("compression SQL must not order by timestamp: %s", activityStreamsCompressionSQL)
	}
	if !strings.Contains(activityStreamsCompressionSQL, "timescaledb.compress_orderby = 'time DESC'") {
		t.Fatalf("compression SQL must order by time DESC: %s", activityStreamsCompressionSQL)
	}
}

func TestActivityStreamsDecompressionSQLCastsChunkNameToRegclass(t *testing.T) {
	if !strings.Contains(activityStreamsDecompressionSQL, "$1::regclass") {
		t.Fatalf("decompression SQL must cast chunk names to regclass: %s", activityStreamsDecompressionSQL)
	}
}
