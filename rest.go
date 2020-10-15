package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

type FtxClient struct {
	Client     *http.Client
	Api        string
	Secret     []byte
	Subaccount string
}

type OrderParam struct {
	Market     string  `json:"market"`
	Side       string  `json:"side"`
	Price      float64 `json:"price"`
	Type       string  `json:"type"`
	Size       float64 `json:"size"`
	ReduceOnly bool    `json:"reduceOnly",omitempty`
	Ioc        bool    `json:"ioc",omitempty`
	PostOnly   bool    `json:"postOnly",omitempty`
	ClientId   string  `json:"clientId,omitempty"`
}

type Market struct {
	Name           string  `json:"name"`
	Type           string  `json:"type"`
	BaseCurrency   string  `json:"baseCurrency",omitempty`
	quoteCurrency  string  `json:"quoteCurrency",omitempty`
	Underlying     string  `json:"underlying"`
	Enable         bool    `json:"enable"`
	Ask            float64 `json:"ask"`
	Bid            float64 `json:"bid"`
	Last           float64 `json:"last"`
	PriceIncrement float64 `json:"priceIncrement"`
	sizeIncrement  float64 `json:"sizeIncrement"`
	Restricted     bool    `json:"restricted"`
}

type Position struct {
	Cost                         float64 `json:"cost"`
	EntryPrice                   float64 `json:"entryPrice"`
	Future                       string  `json:"future"`
	InitialMarginRequirement     float64 `json:"initialMarginRequirement"`
	LongOrderSize                float64 `json:"longOrderSize"`
	MaintenanceMarginRequirement float64 `json:"maintenanceMarginRequirement"`
	NetSize                      float64 `json:"netSize"`
	OpenSize                     float64 `json:"openSize"`
	RealizedPnl                  float64 `json:"realizedPnl"`
	ShortOrderSize               float64 `json:"shortOrderSize"`
	Side                         string  `json:"side"`
	Size                         float64 `json:"size"`
	UnrealizedPnl                float64 `json:"unrealizedPnl"`
	RecentAverageOpenPrice       float64 `json:"recentAverageOpenPrice"`
}

type AccountInfo struct {
	BackstopProvider             bool       `json:"backstopProvider"`
	Collateral                   float64    `json:"collateral"`
	FreeCollateral               float64    `json:"freeCollateral"`
	InitialMarginRequirement     float64    `json:"initialMarginRequirement"`
	Leverage                     float64    `json:"leverage"`
	Liquidating                  bool       `json:"liquidating"`
	MaintenanceMarginRequirement float64    `json:"maintenanceMarginRequirement"`
	MakerFee                     float64    `json:"makerFee"`
	MarginFraction               float64    `json:"marginFraction"`
	OpenMarginFraction           float64    `json:"openMarginFraction"`
	Positions                    []Position `json:"positions"`
	TakerFee                     float64    `json:"takerFee"`
	TotalAccountValue            float64    `json:"totalAccountValue"`
	TotalPositionSize            float64    `json:"totalPositionSize"`
	Username                     string     `json:"username"`
}

func (client *FtxClient) sign(signaturePayload string) string {
	return sign(signaturePayload, client.Secret)
}

func sign(signaturePayload string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signaturePayload))
	return hex.EncodeToString(mac.Sum(nil))
}

var URL = "https://ftx.com/api/"

func (client *FtxClient) signRequest(method string, path string, body []byte) *http.Request {
	ts := strconv.FormatInt(time.Now().UTC().Unix()*1000, 10)
	signaturePayload := ts + method + "/api/" + path + string(body)
	signature := client.sign(signaturePayload)
	req, _ := http.NewRequest(method, URL+path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("FTX-KEY", client.Api)
	req.Header.Set("FTX-SIGN", signature)
	req.Header.Set("FTX-TS", ts)
	if client.Subaccount != "" {
		req.Header.Set("FTX-SUBACCOUNT", client.Subaccount)
	}
	return req
}

func printRequestLog(req *http.Request, err error, resp *http.Response) {
	if err != nil {
		log.Errorf("%s %v %v", req.Method, req.URL, err)
		return
	}
}

func (client *FtxClient) _do(req *http.Request) (*http.Response, error) {
	resp, err := client.Client.Do(req)
	printRequestLog(req, err, resp)
	return resp, err
}

func (client *FtxClient) _get(path string, body []byte) (*http.Response, error) {
	preparedRequest := client.signRequest("GET", path, body)
	return client._do(preparedRequest)
}

func (client *FtxClient) _post(path string, body []byte) (*http.Response, error) {
	preparedRequest := client.signRequest("POST", path, body)
	return client._do(preparedRequest)
}

func (client *FtxClient) _delete(path string, body []byte) (*http.Response, error) {
	preparedRequest := client.signRequest("DELETE", path, body)
	return client._do(preparedRequest)
}

func (client *FtxClient) getMarkets() (*http.Response, error) {
	return client._get("markets", []byte(""))
}

func (client *FtxClient) deleteOrder(orderId int64) (*http.Response, error) {
	path := "orders/" + strconv.FormatInt(orderId, 10)
	return client._delete(path, []byte(""))
}

func (client *FtxClient) deleteAllOrders() (*http.Response, error) {
	return client._delete("orders", []byte(""))
}

func (client *FtxClient) placeOrder(clientId string, market string, side string, price float64, _type string, size float64, reduce bool, post bool) (*http.Response, error) {
	newOrder := OrderParam{Market: market, Side: side, Price: price, Type: _type, Size: size, ReduceOnly: reduce, ClientId: clientId, PostOnly: post}
	body, _ := json.Marshal(newOrder)
	resp, err := client._post("orders", body)
	return resp, err
}

func (client *FtxClient) getFutures() (*http.Response, error) {
	return client._get("futures", []byte(""))
}

func (client *FtxClient) getPositions() (*http.Response, error) {
	return client._get("positions?showAvgPrice=true", []byte(""))
}

type Order struct {
	CreatedAt     time.Time `json:"createdAt"`
	FilledSize    float64   `json:"filledSize"`
	Future        string    `json:"future"`
	ID            int64     `json:"id"`
	Market        string    `json:"market"`
	Price         float64   `json:"price"`
	AvgFillPrice  float64   `json:"avgFillPrice"`
	RemainingSize float64   `json:"remainingSize"`
	Side          string    `json:"side"`
	Size          float64   `json:"size"`
	Status        string    `json:"status"`
	Type          string    `json:"type"`
	ReduceOnly    bool      `json:"reduceOnly"`
	Ioc           bool      `json:"ioc"`
	PostOnly      bool      `json:"postOnly"`
	ClientID      string    `json:"clientId,omitempty"`
}

func (client *FtxClient) getOrders(market string) ([]*Order, error) {
	rsp, err := client._get("orders?market="+market, []byte(""))
	var data []*Order
	err = parseResultWrap(err, rsp, &data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (client *FtxClient) getOrderByClient(clientId string) (*Order, error) {
	rsp, err := client._get("orders/by_client_id/"+clientId, []byte(""))
	var data Order
	err = parseResultWrap(err, rsp, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func (client *FtxClient) getPositionsEx() ([]Position, error) {
	rsp, err := client._get("positions?showAvgPrice=true", []byte(""))
	var data []Position
	err = parseResultWrap(err, rsp, &data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (client *FtxClient) getFuture(market string) (*http.Response, error) {
	return client._get("futures/"+market, []byte(""))
}

func (client *FtxClient) getAccount() (*AccountInfo, error) {
	rsp, err := client._get("account", []byte(""))
	var accountInfo AccountInfo
	err = parseResultWrap(err, rsp, &accountInfo)
	if err != nil {
		return nil, err
	}

	return &accountInfo, nil
}

type MarketItem struct {
	Ask            float64 `json:"ask"`
	Bid            float64 `json:"bid"`
	Enabled        bool    `json:"enabled"`
	Last           float64 `json:"last"`
	Name           string  `json:"name"`
	PriceIncrement float64 `json:"priceIncrement"`
	Restricted     bool    `json:"restricted"`
	SizeIncrement  float64 `json:"sizeIncrement"`
	Type           string  `json:"type"`
	Underlying     string  `json:"underlying"`
}

type FuturesItem struct {
	Ask            float64 `json:"ask"`
	Bid            float64 `json:"bid"`
	Name           string  `json:"name"`
	PriceIncrement float64 `json:"priceIncrement"`
	SizeIncrement  float64 `json:"sizeIncrement"`
	Type           string  `json:"type"`
	Underlying     string  `json:"underlying"`
}

type Result struct {
	Success bool            `json:"success"`
	Error   string          `json:"error"`
	Result  json.RawMessage `json:"result"`
}

func parseResult(r *http.Response, out interface{}) error {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	var result Result
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("%s", string(body))
	}

	if err := json.Unmarshal(result.Result, out); err != nil {
		return err
	}

	return nil
}

func parseResultWrap(err error, r *http.Response, out interface{}) error {
	if err != nil {
		return err
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	fmt.Println(string(body))
	var result Result
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("%s", result.Error)
	}

	if err := json.Unmarshal(result.Result, out); err != nil {
		return err
	}

	return nil
}

func simplePrintResponse(rsp *http.Response) {
	defer rsp.Body.Close()
	b, _ := ioutil.ReadAll(rsp.Body)
	log.Infoln(string(b))
}
