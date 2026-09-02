package auxcloud

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/service/models"
	"golang.org/x/sync/singleflight"
)

const (
	httpTimeout     = 10 * time.Second
	minRequestGap   = time.Second
	maxResponseSize = 1 << 20
)

type Config struct {
	Email    string
	Password string
	Region   string
	HTTP     *http.Client
}

type Device struct {
	EndpointID   string
	FriendlyName string
	ProductID    string
	Mac          string
	DevSession   string
	Cookie       string
}

type Client struct {
	httpClient *http.Client
	baseURL    string
	email      string
	password   string

	mu           sync.Mutex
	loginSession string
	userID       string
	devices      map[string]Device
	deviceLocks  map[string]*sync.Mutex
	caps         map[string]models.DeviceCapabilities

	reqMu      sync.Mutex
	lastReq    time.Time
	requestGap time.Duration

	loginGroup singleflight.Group
}

func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.Email) == "" || cfg.Password == "" {
		return nil, ErrEmptyCredentials
	}

	region := strings.ToLower(strings.TrimSpace(cfg.Region))
	if region == "" {
		region = "eu"
	}
	baseURL, ok := apiServers[region]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownRegion, cfg.Region)
	}

	httpClient := cfg.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: httpTimeout}
	}

	return &Client{
		httpClient:  httpClient,
		baseURL:     baseURL,
		email:       cfg.Email,
		password:    cfg.Password,
		devices:     make(map[string]Device),
		deviceLocks: make(map[string]*sync.Mutex),
		caps:        make(map[string]models.DeviceCapabilities),
		requestGap:  minRequestGap,
	}, nil
}

func (c *Client) Login(ctx context.Context) error {
	_, err, _ := c.loginGroup.Do("login", func() (any, error) {
		return nil, c.doLogin(ctx)
	})
	return err
}

func (c *Client) doLogin(ctx context.Context) error {
	ts := time.Now().Unix()
	payload := struct {
		Email     string `json:"email"`
		Password  string `json:"password"`
		CompanyID string `json:"companyid"`
		LID       string `json:"lid"`
	}{
		Email:     c.email,
		Password:  hashPassword(c.password),
		CompanyID: companyID,
		LID:       licenseID,
	}

	jsonPayload, err := compactJSON(payload)
	if err != nil {
		return err
	}

	body, err := encryptLoginBody(string(jsonPayload), ts)
	if err != nil {
		return err
	}

	headers := c.baseHeaders()
	headers["timestamp"] = fmt.Sprintf("%d", ts)
	headers["token"] = loginToken(string(jsonPayload))
	headers["Content-Type"] = "application/x-java-serialized-object"

	var resp loginResponse
	if err = c.doJSON(ctx, http.MethodPost, "/account/login", body, headers, &resp); err != nil {
		return err
	}
	if resp.LoginSession == "" || resp.UserID == "" {
		return fmt.Errorf("aux cloud login failed: status=%d error=%d", resp.Status, resp.Error)
	}

	c.mu.Lock()
	c.loginSession = resp.LoginSession
	c.userID = resp.UserID
	c.mu.Unlock()

	slog.InfoContext(ctx, "aux cloud login succeeded", slog.String("region", c.baseURL))
	return nil
}

func (c *Client) ensureLogin(ctx context.Context) error {
	c.mu.Lock()
	loggedIn := c.loginSession != "" && c.userID != ""
	c.mu.Unlock()
	if loggedIn {
		return nil
	}
	return c.Login(ctx)
}

func (c *Client) relogin(ctx context.Context) error {
	c.mu.Lock()
	c.loginSession = ""
	c.userID = ""
	c.mu.Unlock()
	return c.Login(ctx)
}

func (c *Client) DiscoverDevices(ctx context.Context) ([]Device, error) {
	if err := c.ensureLogin(ctx); err != nil {
		return nil, err
	}

	families, err := c.getFamilies(ctx)
	if err != nil {
		if isAuthError(0, err.Error()) {
			if reloginErr := c.relogin(ctx); reloginErr != nil {
				return nil, reloginErr
			}
			families, err = c.getFamilies(ctx)
		}
		if err != nil {
			return nil, err
		}
	}

	discovered := make([]Device, 0)
	for _, family := range families {
		devices, err := c.getDevices(ctx, family)
		if err != nil {
			return nil, err
		}
		discovered = append(discovered, devices...)
	}

	c.replaceDevices(discovered)
	return discovered, nil
}

func (c *Client) Device(mac string) (Device, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	device, ok := c.devices[mac]
	return device, ok
}

func (c *Client) Capabilities(mac string) models.DeviceCapabilities {
	c.mu.Lock()
	defer c.mu.Unlock()
	if caps, ok := c.caps[mac]; ok {
		return caps
	}
	return models.DefaultCloudCapabilities()
}

func (c *Client) GetDeviceParams(ctx context.Context, mac string, params []string) (map[string]int, error) {
	return c.withDeviceRetry(ctx, mac, func(ctx context.Context, device Device) (map[string]int, error) {
		return c.getDeviceParams(ctx, device, params)
	})
}

func (c *Client) SetDeviceParams(ctx context.Context, mac string, params map[string]int) error {
	_, err := c.withDeviceRetry(ctx, mac, func(ctx context.Context, device Device) (map[string]int, error) {
		return nil, c.setDeviceParams(ctx, device, params)
	})
	return err
}

func (c *Client) ProbeCapabilities(ctx context.Context, mac string) (models.DeviceCapabilities, error) {
	params, err := c.GetDeviceParams(ctx, mac, nil)
	if err != nil {
		return models.DeviceCapabilities{}, err
	}
	caps := capabilitiesFromParams(params)
	c.mu.Lock()
	c.caps[mac] = caps
	c.mu.Unlock()
	return caps, nil
}

func (c *Client) withDeviceRetry(ctx context.Context, mac string, fn func(context.Context, Device) (map[string]int, error)) (map[string]int, error) {
	if err := c.ensureLogin(ctx); err != nil {
		return nil, err
	}

	lock := c.deviceLock(mac)
	lock.Lock()
	defer lock.Unlock()

	device, ok := c.Device(mac)
	if !ok {
		return nil, ErrDeviceNotFound
	}

	result, err := fn(ctx, device)
	if err == nil {
		return result, nil
	}
	if !errorsIsAuthOrSession(err) {
		return nil, err
	}

	if isAuthError(0, err.Error()) || errors.Is(err, ErrUnauthorized) {
		if reloginErr := c.relogin(ctx); reloginErr != nil {
			return nil, reloginErr
		}
	}

	if _, refreshErr := c.DiscoverDevices(ctx); refreshErr != nil {
		return nil, err
	}
	device, ok = c.Device(mac)
	if !ok {
		return nil, ErrDeviceNotFound
	}
	return fn(ctx, device)
}

func errorsIsAuthOrSession(err error) bool {
	return errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrDeviceSession) || isAuthError(0, err.Error())
}

func (c *Client) getFamilies(ctx context.Context) ([]string, error) {
	var resp familyResponse
	if err := c.doAuthedJSON(ctx, http.MethodPost, "/appsync/group/member/getfamilylist", []byte(""), nil, &resp); err != nil {
		return nil, err
	}
	if resp.Status != 0 {
		return nil, fmt.Errorf("aux cloud get families failed: status=%d", resp.Status)
	}

	ids := make([]string, 0, len(resp.Data.FamilyList))
	for _, family := range resp.Data.FamilyList {
		if family.FamilyID != "" {
			ids = append(ids, family.FamilyID)
		}
	}
	return ids, nil
}

func (c *Client) getDevices(ctx context.Context, familyID string) ([]Device, error) {
	var resp devicesResponse
	headers := map[string]string{"familyid": familyID}
	if err := c.doAuthedJSON(ctx, http.MethodPost, "/appsync/group/dev/query?action=select", []byte(`{"pids":[]}`), headers, &resp); err != nil {
		return nil, err
	}
	if resp.Status != 0 {
		return nil, fmt.Errorf("aux cloud get devices failed: status=%d", resp.Status)
	}

	devices := make([]Device, 0, len(resp.Data.Endpoints))
	for _, endpoint := range resp.Data.Endpoints {
		if endpoint.ProductID == "" || endpoint.Mac == "" {
			continue
		}
		mac, err := models.NormalizeMac(endpoint.Mac)
		if err != nil {
			slog.WarnContext(ctx, "skipping aux cloud device with invalid mac",
				slog.String("endpointId", endpoint.EndpointID),
				slog.String("name", endpoint.FriendlyName))
			continue
		}
		devices = append(devices, Device{
			EndpointID:   endpoint.EndpointID,
			FriendlyName: endpoint.FriendlyName,
			ProductID:    endpoint.ProductID,
			Mac:          mac,
			DevSession:   endpoint.DevSession,
			Cookie:       endpoint.Cookie,
		})
	}
	return devices, nil
}

func (c *Client) getDeviceParams(ctx context.Context, device Device, params []string) (map[string]int, error) {
	vals := make([][]controlVal, 0)
	if len(params) == 1 {
		vals = [][]controlVal{{{Val: 0, Idx: 1}}}
	}

	body, err := c.controlBody(device, "get", params, vals)
	if err != nil {
		return nil, err
	}

	var resp controlResponse
	if err = c.doAuthedJSON(ctx, http.MethodPost, "/device/control/v2/sdkcontrol?license="+url.QueryEscape(license), body, nil, &resp); err != nil {
		return nil, err
	}
	if err = checkControlResponse(resp); err != nil {
		return nil, err
	}

	return parseParamData(resp.Event.Payload.Data)
}

func (c *Client) setDeviceParams(ctx context.Context, device Device, params map[string]int) error {
	keys := make([]string, 0, len(params))
	vals := make([][]controlVal, 0, len(params))
	idx := 1
	for key, val := range params {
		keys = append(keys, key)
		vals = append(vals, []controlVal{{Idx: idx, Val: val}})
		idx++
	}

	body, err := c.controlBody(device, "set", keys, vals)
	if err != nil {
		return err
	}

	var resp controlResponse
	if err = c.doAuthedJSON(ctx, http.MethodPost, "/device/control/v2/sdkcontrol?license="+url.QueryEscape(license), body, nil, &resp); err != nil {
		return err
	}
	return checkControlResponse(resp)
}

func (c *Client) controlBody(device Device, act string, params []string, vals [][]controlVal) ([]byte, error) {
	if device.Cookie == "" || device.DevSession == "" {
		return nil, ErrMissingCookie
	}

	rawCookieBytes, err := base64.StdEncoding.DecodeString(device.Cookie)
	if err != nil {
		return nil, fmt.Errorf("aux cloud cookie decode: %w", err)
	}

	var rawCookie struct {
		TerminalID string `json:"terminalid"`
		AESKey     string `json:"aeskey"`
	}
	if err = json.Unmarshal(rawCookieBytes, &rawCookie); err != nil {
		return nil, fmt.Errorf("aux cloud cookie parse: %w", err)
	}

	mapped, err := compactJSON(map[string]any{
		"device": map[string]any{
			"id":         rawCookie.TerminalID,
			"key":        rawCookie.AESKey,
			"devSession": device.DevSession,
			"aeskey":     rawCookie.AESKey,
			"did":        device.EndpointID,
			"pid":        device.ProductID,
			"mac":        device.Mac,
		},
	})
	if err != nil {
		return nil, err
	}

	request := controlRequest{
		Directive: controlDirective{
			Header: controlHeader{
				Namespace:        "DNA.KeyValueControl",
				Name:             "KeyValueControl",
				InterfaceVersion: "2",
				SenderID:         "sdk",
				MessageID:        fmt.Sprintf("%s-%d", device.EndpointID, time.Now().UnixMilli()),
			},
			Endpoint: controlEndpoint{
				DevicePairedInfo: controlPairedInfo{
					DID:            device.EndpointID,
					PID:            device.ProductID,
					Mac:            device.Mac,
					DeviceTypeFlag: 0,
					Cookie:         base64.StdEncoding.EncodeToString(mapped),
				},
				EndpointID: device.EndpointID,
				Cookie:     map[string]any{},
				DevSession: device.DevSession,
			},
			Payload: controlPayload{
				Act:    act,
				Params: params,
				Vals:   vals,
				DID:    device.EndpointID,
			},
		},
	}
	return compactJSON(request)
}

func checkControlResponse(resp controlResponse) error {
	if resp.Event.Header.Name == "ErrorResponse" {
		msg := resp.Event.Payload.Message
		if isAuthError(0, msg+" "+resp.Event.Payload.Type) {
			if strings.Contains(strings.ToLower(msg), "device") {
				return fmt.Errorf("%w: %s", ErrDeviceSession, msg)
			}
			return fmt.Errorf("%w: %s", ErrUnauthorized, msg)
		}
		return fmt.Errorf("%w: %s (%s)", ErrControlFailed, resp.Event.Payload.Type, msg)
	}
	if resp.Event.Payload.Data == "" {
		return ErrControlFailed
	}
	return nil
}

func parseParamData(data string) (map[string]int, error) {
	var parsed struct {
		Params []string `json:"params"`
		Vals   [][]struct {
			Val int `json:"val"`
		} `json:"vals"`
	}
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		return nil, err
	}

	result := make(map[string]int, len(parsed.Params))
	for i, key := range parsed.Params {
		value := 0
		if i < len(parsed.Vals) && len(parsed.Vals[i]) > 0 {
			value = parsed.Vals[i][0].Val
		}
		result[key] = value
	}
	return result, nil
}

func (c *Client) replaceDevices(devices []Device) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.devices = make(map[string]Device, len(devices))
	for _, device := range devices {
		c.devices[device.Mac] = device
	}
}

func (c *Client) deviceLock(mac string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.deviceLocks[mac] == nil {
		c.deviceLocks[mac] = &sync.Mutex{}
	}
	return c.deviceLocks[mac]
}

func (c *Client) baseHeaders() map[string]string {
	return map[string]string{
		"Content-Type":    "application/x-java-serialized-object",
		"licenseId":       licenseID,
		"lid":             licenseID,
		"language":        "en",
		"appVersion":      spoofAppVersion,
		"User-Agent":      spoofUserAgent,
		"system":          spoofSystem,
		"appPlatform":     spoofAppPlatform,
		"Accept":          "application/json",
		"Accept-Encoding": "gzip, deflate",
		"Connection":      "keep-alive",
	}
}

func (c *Client) authHeaders() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	headers := c.baseHeaders()
	headers["loginsession"] = c.loginSession
	headers["userid"] = c.userID
	return headers
}

func (c *Client) doAuthedJSON(ctx context.Context, method, path string, body []byte, extra map[string]string, dest any) error {
	headers := c.authHeaders()
	for key, value := range extra {
		headers[key] = value
	}
	return c.doJSON(ctx, method, path, body, headers, dest)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body []byte, headers map[string]string, dest any) error {
	if err := c.waitGap(ctx); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := c.httpClient.Do(req)
	c.markReq()
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxResponseSize)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: http %d", ErrUnauthorized, resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("aux cloud http %d", resp.StatusCode)
	}
	if dest == nil {
		return nil
	}
	if err = json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("aux cloud decode: %w", err)
	}
	return nil
}

func (c *Client) waitGap(ctx context.Context) error {
	c.reqMu.Lock()
	wait := c.requestGap - time.Since(c.lastReq)
	c.reqMu.Unlock()
	if c.requestGap <= 0 || wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) markReq() {
	c.reqMu.Lock()
	c.lastReq = time.Now()
	c.reqMu.Unlock()
}

type loginResponse struct {
	Status       int    `json:"status"`
	Error        int    `json:"error"`
	LoginSession string `json:"loginsession"`
	UserID       string `json:"userid"`
}

type familyResponse struct {
	Status int `json:"status"`
	Data   struct {
		FamilyList []struct {
			FamilyID string `json:"familyid"`
			Name     string `json:"name"`
		} `json:"familyList"`
	} `json:"data"`
}

type devicesResponse struct {
	Status int `json:"status"`
	Data   struct {
		Endpoints []struct {
			EndpointID   string `json:"endpointId"`
			FriendlyName string `json:"friendlyName"`
			ProductID    string `json:"productId"`
			Mac          string `json:"mac"`
			DevSession   string `json:"devSession"`
			Cookie       string `json:"cookie"`
		} `json:"endpoints"`
	} `json:"data"`
}

type controlRequest struct {
	Directive controlDirective `json:"directive"`
}

type controlDirective struct {
	Header   controlHeader   `json:"header"`
	Endpoint controlEndpoint `json:"endpoint"`
	Payload  controlPayload  `json:"payload"`
}

type controlHeader struct {
	Namespace        string `json:"namespace"`
	Name             string `json:"name"`
	InterfaceVersion string `json:"interfaceVersion"`
	SenderID         string `json:"senderId"`
	MessageID        string `json:"messageId"`
}

type controlEndpoint struct {
	DevicePairedInfo controlPairedInfo `json:"devicePairedInfo"`
	EndpointID       string            `json:"endpointId"`
	Cookie           map[string]any    `json:"cookie"`
	DevSession       string            `json:"devSession"`
}

type controlPairedInfo struct {
	DID            string `json:"did"`
	PID            string `json:"pid"`
	Mac            string `json:"mac"`
	DeviceTypeFlag int    `json:"devicetypeflag"`
	Cookie         string `json:"cookie"`
}

type controlPayload struct {
	Act    string         `json:"act"`
	Params []string       `json:"params"`
	Vals   [][]controlVal `json:"vals"`
	DID    string         `json:"did"`
}

type controlVal struct {
	Val int `json:"val"`
	Idx int `json:"idx"`
}

type controlResponse struct {
	Event struct {
		Header struct {
			Name string `json:"name"`
		} `json:"header"`
		Payload struct {
			Data    string `json:"data"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"payload"`
	} `json:"event"`
}
