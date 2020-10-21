package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/google/uuid"
)

var (
	perpName   = ""
	futureName = ""

	// 权限
	apiKey     = ""
	secretKey  = ""
	subAccount = ""

	// 目标钉钉群
	ding = ""
	// 通知会带这个讯息，表明身份
	myName = ""

	// 加速价格检查间隔，发生在存在触发网格之后
	quickRecheckInterval = time.Millisecond * 500
	// 常规价格检查间隔
	checkInterval = time.Millisecond * 1500

	grids = []*TradeGrid{}

	orderMap = NewOrderMap()

	client *FtxClient

	lastPlaceTime time.Time

	ask1 float64
	bid1 float64

	profitTotal float64
)

type PersistData struct {
	Grids []*TradeGrid
}

type OrderMap struct {
	Orders map[string]*GridOrder
}

func NewOrderMap() *OrderMap {
	return &OrderMap{
		Orders: map[string]*GridOrder{},
	}
}

func (orderm *OrderMap) add(order *GridOrder) {
	// orderm.mutex.Lock()
	// defer orderm.mutex.Unlock()
	orderm.Orders[order.ClientId] = order
}

func (orderm *OrderMap) RangeOver(fn func(order *GridOrder) bool) {
	// orderm.mutex.Lock()
	// defer orderm.mutex.Unlock()
	for _, order := range orderm.Orders {
		if !fn(order) {
			break
		}
	}
}

func (orderm *OrderMap) remove(clientId string) {
	// orderm.mutex.Lock()
	// defer orderm.mutex.Unlock()
	delete(orderm.Orders, clientId)
}

func (orderm *OrderMap) get(clientId string) (*GridOrder, bool) {
	// orderm.mutex.Lock()
	// defer orderm.mutex.Unlock()
	order, found := orderm.Orders[clientId]
	return order, found
}

func debugGrid() {
	log.Println("PerpName:", perpName)
	log.Println("FutureName:", futureName)
	log.Println("Grids Bellow:")
	var totalQty float64
	for index, grid := range grids {
		gridQty := float64(grid.CloseChance + grid.OpenChance)
		totalQty += gridQty
		log.Printf("[%03d] %v %v %v %v -- gridQty=%v accQty=%v distance=%0.6v", index,
			grid.OpenAt, grid.CloseAt, grid.OpenChance, grid.CloseChance, gridQty, totalQty,
			grid.OpenAt-grid.CloseAt,
		)
	}
}

func place(clientId string, market string, side string, price float64, _type string, size float64, reduce bool, post bool) {
	log.Infoln("PlaceOrder", clientId, market, side, price, _type, size, "reduce", reduce, "postonly", post)
	if *testMode {
		return
	}

	lastPlaceTime = time.Now()

	resp, err := client.placeOrder(clientId, market, side, price, _type, size, reduce, post)
	if err != nil {
		log.Errorln("PlaceError", err)
		SendDingTalkAsync(fmt.Sprintln("发送订单失败:", market, side, price, _type, size, reduce, "原因：", err))
		return
	}
	defer resp.Body.Close()
	b, _ := ioutil.ReadAll(resp.Body)

	var result Result
	json.Unmarshal(b, &result)

	if result.Error != "" {
		RejectOrder(clientId, side)
		SendDingTalkAsync(fmt.Sprintln("发送订单失败:", market, side, price, _type, size, reduce, "原因：", result.Error))
	}

	log.Infoln("PlaceResult", string(b))
}

func mustFloat(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		panic("invalid float " + s)
	}
	return f
}

func mustInt(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		panic("invalid int " + s)
	}
	return n
}

func mustBool(s string) bool {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		panic("invalid int " + s)
	}

	return n != 0
}

func loadConfigAndAssign() {
	if *gridFile != "" {
		loadGridConfigAndAssign(*gridFile)
	}

	if *cfgFile != "" {
		loadBaseConfigAndAssign(*cfgFile)
	}
}

func debugPositions() {
	rsp, err := client.getPositions()
	if err != nil {
		log.Println("getPositions", err)
		return
	}
	simplePrintResponse(rsp)
}

func excelBool(b bool) int {
	if b {
		return 1
	}
	return 0
}
func writeGridCurrent() {
	buf := bytes.NewBuffer(nil)
	fmt.Fprintf(buf, "%s,%s,,,,,,\n", perpName, futureName)
	fmt.Fprintf(buf, "openDiff,closeDiff,openChance,closeChance\n")
	for _, grid := range grids {
		fmt.Fprintf(buf, "%v,%v,%v,%v\n", grid.OpenAt, grid.CloseAt, grid.OpenChance, grid.CloseChance)
	}

	ioutil.WriteFile(perpName+"_grid_runtime.csv", buf.Bytes(), 0666)
}

func loadGridConfigAndAssign(file string) {
	f, err := os.Open(file)
	if err != nil {
		log.Fatalln("open file:", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		log.Fatalln("read csv file:", err)
	}

	perpName = records[0][0]
	futureName = records[0][1]
	grids = grids[:0]
	for row := 2; row < len(records); row++ {
		grids = append(grids, &TradeGrid{
			Uuid:        uuid.New().String(),
			OpenAt:      mustFloat(records[row][0]),
			CloseAt:     mustFloat(records[row][1]),
			OpenChance:  mustFloat(records[row][2]),
			CloseChance: mustFloat(records[row][3]),
			OpenOrders:  NewOrderMap(),
			CloseOrders: NewOrderMap(),
		})
	}
}

func loadFromSaveFile(file string) error {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	var persistItem GridPersistItem
	if err := yaml.Unmarshal(b, &persistItem); err != nil {
		return err
	}

	perpName = persistItem.Symbol
	grids = persistItem.Grids
	for _, grid := range grids {
		profitTotal += grid.CloseTotal * (grid.CloseAt - grid.OpenAt)

		for _, order := range grid.OpenOrders.Orders {
			orderMap.add(order)
			order.Grid = grid
		}

		for _, order := range grid.CloseOrders.Orders {
			orderMap.add(order)
			order.Grid = grid
		}
	}

	return nil
}

func loadBaseConfigAndAssign(file string) {
	content, err := ioutil.ReadFile(file)
	if err != nil {
		log.Fatalln("read config:", err)
	}
	var config Config
	if err := json.Unmarshal(content, &config); err != nil {
		log.Fatalln("parse config:", err)
	}

	apiKey = config.ApiKey
	secretKey = config.SecretKey
	subAccount = config.SubAccount
	myName = config.MyName
	ding = config.Ding

	checkInterval = time.Duration(config.CheckInterval) * time.Millisecond
	if checkInterval == time.Duration(0) {
		checkInterval = time.Second * 3
	}

	quickRecheckInterval = time.Duration(config.QuickRecheckInterval) * time.Millisecond
	if quickRecheckInterval == time.Duration(0) {
		quickRecheckInterval = time.Second * 1
	}

	client = &FtxClient{
		Client:     &http.Client{},
		Api:        apiKey,
		Secret:     []byte(secretKey),
		Subaccount: subAccount,
	}
}

type GridOrder struct {
	ClientId   string
	Id         int64
	Qty        float64
	EQty       float64
	CreateAt   time.Time
	UpdateTime time.Time
	DeleteAt   time.Time  `yaml:"-"`
	Grid       *TradeGrid `yaml:"-"`
	Side       string
}

type TradeGrid struct {
	Uuid        string
	OpenAt      float64
	CloseAt     float64
	OpenChance  float64
	CloseChance float64

	OpenTotal   float64
	CloseTotal  float64
	OpenOrders  *OrderMap
	CloseOrders *OrderMap
}

type Config struct {
	ApiKey               string `json:"apiKey"`
	SecretKey            string `json:"secretKey"`
	SubAccount           string `json:"subAccount"`
	Ding                 string `json:"ding"`
	MyName               string `json:"myName"`
	QuickRecheckInterval int    `json:"quickRecheckInterval"`
	CheckInterval        int    `json:"checkInterval"`
}

func NewDefaultConfig() *Config {
	return &Config{
		QuickRecheckInterval: 500,
		CheckInterval:        1500,
	}
}
