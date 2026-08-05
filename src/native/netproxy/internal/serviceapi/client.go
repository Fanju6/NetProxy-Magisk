package serviceapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

const (
	methodGetVersion         = "/daemon.StartedService/GetVersion"
	methodGetStartedAt       = "/daemon.StartedService/GetStartedAt"
	methodSubscribeStatus    = "/daemon.StartedService/SubscribeStatus"
	methodGetClashModeStatus = "/daemon.StartedService/GetClashModeStatus"
	methodSetClashMode       = "/daemon.StartedService/SetClashMode"
	methodSelectOutbound     = "/daemon.StartedService/SelectOutbound"
	methodURLTest            = "/daemon.StartedService/URLTest"
	methodSubscribeGroups    = "/daemon.StartedService/SubscribeGroups"
)

// Client is the minimal native Service API bridge used by the NetProxy shell
// control layer. It intentionally implements only the pinned protobuf wire
// messages needed by the module, avoiding a dependency on the full daemon.
type Client struct {
	baseURL    string
	secret     string
	httpClient *http.Client
}

type Version struct {
	Version    string `json:"version"`
	APIVersion int32  `json:"api_version"`
}

type StartedAt struct {
	UnixMilli int64 `json:"unix_milli"`
}

type Status struct {
	Memory           uint64 `json:"memory"`
	Goroutines       int32  `json:"goroutines"`
	ConnectionsIn    int32  `json:"connections_in"`
	ConnectionsOut   int32  `json:"connections_out"`
	TrafficAvailable bool   `json:"traffic_available"`
	Uplink           int64  `json:"uplink"`
	Downlink         int64  `json:"downlink"`
	UplinkTotal      int64  `json:"uplink_total"`
	DownlinkTotal    int64  `json:"downlink_total"`
}

type Mode struct {
	Available []string `json:"available"`
	Current   string   `json:"current"`
}

type GroupItem struct {
	Tag          string `json:"tag"`
	Type         string `json:"type"`
	URLTestTime  int64  `json:"url_test_time,omitempty"`
	URLTestDelay int32  `json:"url_test_delay,omitempty"`
}

type Group struct {
	Tag        string      `json:"tag"`
	Type       string      `json:"type"`
	Selectable bool        `json:"selectable"`
	Selected   string      `json:"selected,omitempty"`
	Expanded   bool        `json:"expanded"`
	Items      []GroupItem `json:"items"`
}

type emptyMessage struct{}
type versionResponse struct {
	Version    string
	APIVersion int32
}
type startedAtResponse struct{ UnixMilli int64 }
type statusRequest struct{ Interval int64 }
type statusResponse Status
type clashModeStatusResponse struct {
	Available []string
	Current   string
}
type clashModeRequest struct{ Mode string }
type selectOutboundRequest struct {
	Group    string
	Outbound string
}
type urlTestRequest struct{ Outbound string }
type groupsResponse struct{ Groups []Group }

func New(address string, secret string) (*Client, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, errors.New("Service API address is empty")
	}
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	serverURL, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("parse Service API address: %w", err)
	}
	if serverURL.Scheme != "http" {
		return nil, fmt.Errorf("unsupported Service API scheme %q", serverURL.Scheme)
	}
	if serverURL.Host == "" {
		return nil, errors.New("Service API host is empty")
	}
	if serverURL.Path != "" && serverURL.Path != "/" {
		return nil, errors.New("Service API address must not contain a path")
	}
	return &Client{
		baseURL:    strings.TrimRight(serverURL.String(), "/"),
		secret:     secret,
		httpClient: &http.Client{},
	}, nil
}

func (c *Client) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}

func (c *Client) invoke(ctx context.Context, method string, request any, response any) error {
	payload, err := (wireCodec{}).Marshal(request)
	if err != nil {
		return err
	}
	content, err := c.doRequest(ctx, method, payload, false)
	if err != nil {
		return err
	}
	return (wireCodec{}).Unmarshal(content, response)
}

func (c *Client) Version(ctx context.Context) (Version, error) {
	var response versionResponse
	if err := c.invoke(ctx, methodGetVersion, &emptyMessage{}, &response); err != nil {
		return Version{}, err
	}
	return Version{Version: response.Version, APIVersion: response.APIVersion}, nil
}

// Ready verifies that StartedService is attached to a running sing-box
// instance and that Clash mode control is available.
func (c *Client) Ready(ctx context.Context) (StartedAt, error) {
	startedAt, err := c.StartedAt(ctx)
	if err != nil {
		return StartedAt{}, err
	}
	if startedAt.UnixMilli <= 0 {
		return StartedAt{}, errors.New("Service API has no active sing-box instance")
	}
	if _, err = c.Mode(ctx); err != nil {
		return StartedAt{}, err
	}
	return startedAt, nil
}

func (c *Client) StartedAt(ctx context.Context) (StartedAt, error) {
	var response startedAtResponse
	if err := c.invoke(ctx, methodGetStartedAt, &emptyMessage{}, &response); err != nil {
		return StartedAt{}, err
	}
	return StartedAt{UnixMilli: response.UnixMilli}, nil
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	payload, err := (wireCodec{}).Marshal(&statusRequest{Interval: int64(time.Second)})
	if err != nil {
		return Status{}, err
	}
	content, err := c.doRequest(ctx, methodSubscribeStatus, payload, true)
	if err != nil {
		return Status{}, err
	}
	var response statusResponse
	if err = (wireCodec{}).Unmarshal(content, &response); err != nil {
		return Status{}, err
	}
	return Status(response), nil
}

func (c *Client) Mode(ctx context.Context) (Mode, error) {
	var response clashModeStatusResponse
	if err := c.invoke(ctx, methodGetClashModeStatus, &emptyMessage{}, &response); err != nil {
		return Mode{}, err
	}
	return Mode{Available: response.Available, Current: response.Current}, nil
}

func (c *Client) SetMode(ctx context.Context, mode string) error {
	return c.invoke(ctx, methodSetClashMode, &clashModeRequest{Mode: mode}, &emptyMessage{})
}

func (c *Client) Select(ctx context.Context, group string, outbound string) error {
	return c.invoke(ctx, methodSelectOutbound, &selectOutboundRequest{Group: group, Outbound: outbound}, &emptyMessage{})
}

func (c *Client) URLTest(ctx context.Context, outbound string) error {
	return c.invoke(ctx, methodURLTest, &urlTestRequest{Outbound: outbound}, &emptyMessage{})
}

func (c *Client) Groups(ctx context.Context) ([]Group, error) {
	payload, err := (wireCodec{}).Marshal(&emptyMessage{})
	if err != nil {
		return nil, err
	}
	content, err := c.doRequest(ctx, methodSubscribeGroups, payload, true)
	if err != nil {
		return nil, err
	}
	var response groupsResponse
	if err = (wireCodec{}).Unmarshal(content, &response); err != nil {
		return nil, err
	}
	return response.Groups, nil
}

const maxFrameSize = 32 << 20

func (c *Client) doRequest(ctx context.Context, method string, payload []byte, firstDataFrameOnly bool) ([]byte, error) {
	var requestBody bytes.Buffer
	requestBody.WriteByte(0)
	if err := binary.Write(&requestBody, binary.BigEndian, uint32(len(payload))); err != nil {
		return nil, err
	}
	requestBody.Write(payload)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+method, &requestBody)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/grpc-web+proto")
	request.Header.Set("Accept", "application/grpc-web+proto")
	request.Header.Set("X-Grpc-Web", "1")
	request.Header.Set("X-User-Agent", "netproxyctl/1")
	if c.secret != "" {
		request.Header.Set("Authorization", "Bearer "+c.secret)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("Service API HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var firstData []byte
	for {
		var header [5]byte
		if _, err = io.ReadFull(response.Body, header[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				if firstDataFrameOnly && firstData != nil {
					return firstData, nil
				}
				return nil, errors.New("Service API response ended without gRPC status")
			}
			return nil, err
		}
		length := binary.BigEndian.Uint32(header[1:])
		if length > maxFrameSize {
			return nil, fmt.Errorf("Service API frame is too large: %d", length)
		}
		content := make([]byte, length)
		if _, err = io.ReadFull(response.Body, content); err != nil {
			return nil, err
		}
		if header[0]&0x80 != 0 {
			if err = parseTrailer(content); err != nil {
				return nil, err
			}
			return firstData, nil
		}
		if header[0] != 0 {
			return nil, fmt.Errorf("unsupported Service API frame flag: 0x%02x", header[0])
		}
		if firstData == nil {
			firstData = content
			if firstDataFrameOnly {
				return firstData, nil
			}
		}
	}
}

func parseTrailer(content []byte) error {
	statusCode := -1
	statusMessage := ""
	for _, line := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "grpc-status":
			statusCode, _ = strconv.Atoi(strings.TrimSpace(value))
		case "grpc-message":
			statusMessage, _ = url.PathUnescape(strings.TrimSpace(value))
		}
	}
	if statusCode < 0 {
		return errors.New("Service API response has no grpc-status")
	}
	if statusCode != 0 {
		if statusMessage == "" {
			statusMessage = "unknown error"
		}
		return fmt.Errorf("gRPC status %d: %s", statusCode, statusMessage)
	}
	return nil
}

type wireCodec struct{}

func (wireCodec) Name() string { return "proto" }

func (wireCodec) Marshal(value any) ([]byte, error) {
	var output []byte
	switch message := value.(type) {
	case *emptyMessage:
		return nil, nil
	case *clashModeRequest:
		output = appendString(output, 3, message.Mode)
	case *selectOutboundRequest:
		output = appendString(output, 1, message.Group)
		output = appendString(output, 2, message.Outbound)
	case *urlTestRequest:
		output = appendString(output, 1, message.Outbound)
	case *statusRequest:
		output = appendVarint(output, 1, uint64(message.Interval))
	default:
		return nil, fmt.Errorf("unsupported Service API request %T", value)
	}
	return output, nil
}

func (wireCodec) Unmarshal(content []byte, value any) error {
	switch message := value.(type) {
	case *emptyMessage:
		return nil
	case *versionResponse:
		return decodeVersion(content, message)
	case *startedAtResponse:
		return decodeStartedAt(content, message)
	case *statusResponse:
		return decodeStatus(content, message)
	case *clashModeStatusResponse:
		return decodeMode(content, message)
	case *groupsResponse:
		return decodeGroups(content, message)
	default:
		return fmt.Errorf("unsupported Service API response %T", value)
	}
}

func appendString(output []byte, number protowire.Number, value string) []byte {
	if value == "" {
		return output
	}
	output = protowire.AppendTag(output, number, protowire.BytesType)
	return protowire.AppendString(output, value)
}

func appendVarint(output []byte, number protowire.Number, value uint64) []byte {
	output = protowire.AppendTag(output, number, protowire.VarintType)
	return protowire.AppendVarint(output, value)
}

func consumeFields(content []byte, handle func(protowire.Number, protowire.Type, []byte) (int, error)) error {
	for len(content) > 0 {
		number, wireType, tagLength := protowire.ConsumeTag(content)
		if tagLength < 0 {
			return protowire.ParseError(tagLength)
		}
		content = content[tagLength:]
		fieldLength, err := handle(number, wireType, content)
		if err != nil {
			return err
		}
		if fieldLength == 0 {
			fieldLength = protowire.ConsumeFieldValue(number, wireType, content)
			if fieldLength < 0 {
				return protowire.ParseError(fieldLength)
			}
		}
		content = content[fieldLength:]
	}
	return nil
}

func consumeString(content []byte, wireType protowire.Type) (string, int, error) {
	if wireType != protowire.BytesType {
		return "", 0, errors.New("invalid protobuf string wire type")
	}
	value, length := protowire.ConsumeString(content)
	if length < 0 {
		return "", 0, protowire.ParseError(length)
	}
	return value, length, nil
}

func consumeVarint(content []byte, wireType protowire.Type) (uint64, int, error) {
	if wireType != protowire.VarintType {
		return 0, 0, errors.New("invalid protobuf varint wire type")
	}
	value, length := protowire.ConsumeVarint(content)
	if length < 0 {
		return 0, 0, protowire.ParseError(length)
	}
	return value, length, nil
}

func consumeBytes(content []byte, wireType protowire.Type) ([]byte, int, error) {
	if wireType != protowire.BytesType {
		return nil, 0, errors.New("invalid protobuf bytes wire type")
	}
	value, length := protowire.ConsumeBytes(content)
	if length < 0 {
		return nil, 0, protowire.ParseError(length)
	}
	return value, length, nil
}

func decodeVersion(content []byte, message *versionResponse) error {
	return consumeFields(content, func(number protowire.Number, wireType protowire.Type, field []byte) (int, error) {
		switch number {
		case 1:
			value, length, err := consumeString(field, wireType)
			message.Version = value
			return length, err
		case 2:
			value, length, err := consumeVarint(field, wireType)
			message.APIVersion = int32(value)
			return length, err
		default:
			return 0, nil
		}
	})
}

func decodeStartedAt(content []byte, message *startedAtResponse) error {
	return consumeFields(content, func(number protowire.Number, wireType protowire.Type, field []byte) (int, error) {
		if number != 1 {
			return 0, nil
		}
		value, length, err := consumeVarint(field, wireType)
		message.UnixMilli = int64(value)
		return length, err
	})
}

func decodeStatus(content []byte, message *statusResponse) error {
	return consumeFields(content, func(number protowire.Number, wireType protowire.Type, field []byte) (int, error) {
		if number < 1 || number > 9 {
			return 0, nil
		}
		value, length, err := consumeVarint(field, wireType)
		if err != nil {
			return length, err
		}
		switch number {
		case 1:
			message.Memory = value
		case 2:
			message.Goroutines = int32(value)
		case 3:
			message.ConnectionsIn = int32(value)
		case 4:
			message.ConnectionsOut = int32(value)
		case 5:
			message.TrafficAvailable = value != 0
		case 6:
			message.Uplink = int64(value)
		case 7:
			message.Downlink = int64(value)
		case 8:
			message.UplinkTotal = int64(value)
		case 9:
			message.DownlinkTotal = int64(value)
		}
		return length, nil
	})
}

func decodeMode(content []byte, message *clashModeStatusResponse) error {
	return consumeFields(content, func(number protowire.Number, wireType protowire.Type, field []byte) (int, error) {
		if number != 1 && number != 2 {
			return 0, nil
		}
		value, length, err := consumeString(field, wireType)
		if err != nil {
			return length, err
		}
		switch number {
		case 1:
			message.Available = append(message.Available, value)
		case 2:
			message.Current = value
		}
		return length, nil
	})
}

func decodeGroups(content []byte, message *groupsResponse) error {
	return consumeFields(content, func(number protowire.Number, wireType protowire.Type, field []byte) (int, error) {
		if number != 1 {
			return 0, nil
		}
		value, length, err := consumeBytes(field, wireType)
		if err != nil {
			return length, err
		}
		var group Group
		if err = decodeGroup(value, &group); err != nil {
			return length, err
		}
		message.Groups = append(message.Groups, group)
		return length, nil
	})
}

func decodeGroup(content []byte, message *Group) error {
	return consumeFields(content, func(number protowire.Number, wireType protowire.Type, field []byte) (int, error) {
		switch number {
		case 1, 2, 4:
			value, length, err := consumeString(field, wireType)
			if number == 1 {
				message.Tag = value
			} else if number == 2 {
				message.Type = value
			} else {
				message.Selected = value
			}
			return length, err
		case 3, 5:
			value, length, err := consumeVarint(field, wireType)
			if number == 3 {
				message.Selectable = value != 0
			} else {
				message.Expanded = value != 0
			}
			return length, err
		case 6:
			value, length, err := consumeBytes(field, wireType)
			if err != nil {
				return length, err
			}
			var item GroupItem
			if err = decodeGroupItem(value, &item); err != nil {
				return length, err
			}
			message.Items = append(message.Items, item)
			return length, nil
		default:
			return 0, nil
		}
	})
}

func decodeGroupItem(content []byte, message *GroupItem) error {
	return consumeFields(content, func(number protowire.Number, wireType protowire.Type, field []byte) (int, error) {
		switch number {
		case 1, 2:
			value, length, err := consumeString(field, wireType)
			if number == 1 {
				message.Tag = value
			} else {
				message.Type = value
			}
			return length, err
		case 3, 4:
			value, length, err := consumeVarint(field, wireType)
			if number == 3 {
				message.URLTestTime = int64(value)
			} else {
				message.URLTestDelay = int32(value)
			}
			return length, err
		default:
			return 0, nil
		}
	})
}
