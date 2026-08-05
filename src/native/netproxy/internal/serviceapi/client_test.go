package serviceapi

import (
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestStartedAtOverGRPCWeb(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != methodGetStartedAt {
			http.Error(writer, "unexpected method", http.StatusNotFound)
			return
		}
		if request.Header.Get("Authorization") != "Bearer test-secret" {
			http.Error(writer, "missing authorization", http.StatusUnauthorized)
			return
		}
		requestFrame, err := io.ReadAll(request.Body)
		if err != nil || len(requestFrame) != 5 || binary.BigEndian.Uint32(requestFrame[1:]) != 0 {
			http.Error(writer, "invalid request frame", http.StatusBadRequest)
			return
		}
		payload := protowire.AppendTag(nil, 1, protowire.VarintType)
		payload = protowire.AppendVarint(payload, 123456)
		writeTestFrame(t, writer, 0, payload)
		writeTestFrame(t, writer, 0x80, []byte("grpc-status: 0\r\n"))
	}))
	defer server.Close()

	client, err := New(server.URL, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	startedAt, err := client.StartedAt(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if startedAt.UnixMilli != 123456 {
		t.Fatalf("unexpected startedAt: %d", startedAt.UnixMilli)
	}
}

func TestStatusOverGRPCWeb(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != methodSubscribeStatus {
			http.Error(writer, "unexpected method", http.StatusNotFound)
			return
		}
		requestFrame, err := io.ReadAll(request.Body)
		if err != nil || len(requestFrame) <= 5 {
			http.Error(writer, "invalid request frame", http.StatusBadRequest)
			return
		}
		interval, length := protowire.ConsumeVarint(requestFrame[6:])
		if requestFrame[5] != byte(protowire.EncodeTag(1, protowire.VarintType)) || length < 0 || interval != uint64(time.Second) {
			http.Error(writer, "invalid status interval", http.StatusBadRequest)
			return
		}

		var payload []byte
		for number, value := range map[protowire.Number]uint64{
			1: 4096,
			2: 12,
			3: 3,
			4: 4,
			5: 1,
			6: 128,
			7: 256,
			8: 1024,
			9: 2048,
		} {
			payload = appendVarint(payload, number, value)
		}
		writeTestFrame(t, writer, 0, payload)
	}))
	defer server.Close()

	client, err := New(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	status, err := client.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status.Memory != 4096 || status.Goroutines != 12 || status.ConnectionsIn != 3 ||
		status.ConnectionsOut != 4 || !status.TrafficAvailable || status.Uplink != 128 ||
		status.Downlink != 256 || status.UplinkTotal != 1024 || status.DownlinkTotal != 2048 {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func writeTestFrame(t *testing.T, writer io.Writer, flag byte, payload []byte) {
	t.Helper()
	header := []byte{flag, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	if _, err := writer.Write(header); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
}

func TestWireCodecMarshalSelectOutbound(t *testing.T) {
	content, err := (wireCodec{}).Marshal(&selectOutboundRequest{
		Group:    "Proxy",
		Outbound: "Auto/default",
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := appendString(nil, 1, "Proxy")
	expected = appendString(expected, 2, "Auto/default")
	if !reflect.DeepEqual(content, expected) {
		t.Fatalf("unexpected protobuf payload: %x", content)
	}
}

func TestWireCodecUnmarshalGroups(t *testing.T) {
	item := appendString(nil, 1, "default/node")
	item = appendString(item, 2, "vless")
	item = protowire.AppendTag(item, 4, protowire.VarintType)
	item = protowire.AppendVarint(item, 88)

	group := appendString(nil, 1, "Select/default")
	group = appendString(group, 2, "selector")
	group = protowire.AppendTag(group, 3, protowire.VarintType)
	group = protowire.AppendVarint(group, 1)
	group = appendString(group, 4, "default/node")
	group = protowire.AppendTag(group, 6, protowire.BytesType)
	group = protowire.AppendBytes(group, item)

	content := protowire.AppendTag(nil, 1, protowire.BytesType)
	content = protowire.AppendBytes(content, group)
	var response groupsResponse
	if err := (wireCodec{}).Unmarshal(content, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Groups) != 1 || len(response.Groups[0].Items) != 1 {
		t.Fatalf("unexpected groups: %#v", response.Groups)
	}
	if response.Groups[0].Selected != "default/node" || response.Groups[0].Items[0].URLTestDelay != 88 {
		t.Fatalf("unexpected decoded group: %#v", response.Groups[0])
	}
}
