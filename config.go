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

	grids = []*TradeGrid{
		// 说明
		// 注意1: 开仓触发价差必须大于最大平仓触发价差，且必须升序排列
		// 注意2: 平仓触发价差必须小于最小开仓触发价格，且必须逆序排列
		// 注意3: 请根据实际账号情况配置，本程序暂无异常处理，始终认为下单即成功
		// 注意4: 最大仓量是当前价差网格前面所有仓量的总和，严控仓位。
		// 支持以下设置：
		// 	- 开仓触发价差
		//  - 平仓触发价差
		//  - 开仓初始机会
		//  - 平仓初始机会
		//  - 每次下单量，机会触发时，将分多次进行下单，每次下单的单量
		//  - 仅平仓，忽略开仓
		//  - 仅开仓，忽略平仓
		//  - 一次性开平，开仓会增加平仓机会，平仓不会增加开仓机会，但初始平仓机会依然有效

		//{OpenDiff: 1.70, CloseDiff: 1.30, OpenChance: 0, CloseChance: 55, Qty: 2, CloseOnly: false, OpenOnly: false, OneShoot: false},
		//{OpenDiff: 2.00, CloseDiff: 1.1, OpenChance: 30, CloseChance: 0, Qty: 2},
		//{OpenDiff: 2.30, CloseDiff: 0.3, OpenChance: 30, CloseChance: 0, Qty: 2},
		//{OpenDiff: 3.00, CloseDiff: 0.2, OpenChance: 30, CloseChance: 0, Qty: 2},
	}

	orderMap = NewOrderMap()

	client *FtxClient

	lastPlaceTime time.Time
)

type OrderMap struct {
	// mutex  sync.Mutex
	orders map[string]*GridOrder
}

func NewOrderMap() *OrderMap {
	return &OrderMap{
		orders: map[string]*GridOrder{},
	}
}

func (orderm *OrderMap) add(order *GridOrder) {
	// orderm.mutex.Lock()
	// defer orderm.mutex.Unlock()
	orderm.orders[order.ClientId] = order
}

func (orderm *OrderMap) RangeOver(fn func(order *GridOrder) bool) {
	// orderm.mutex.Lock()
	// defer orderm.mutex.Unlock()
	for _, order := range orderm.orders {
		if !fn(order) {
			break
		}
	}
}

func (orderm *OrderMap) remove(clientId string) {
	// orderm.mutex.Lock()
	// defer orderm.mutex.Unlock()
	delete(orderm.orders, clientId)
}

func (orderm *OrderMap) get(clientId string) (*GridOrder, bool) {
	// orderm.mutex.Lock()
	// defer orderm.mutex.Unlock()
	order, found := orderm.orders[clientId]
	return order, found
}

func debugGrid() {
	log.Println("PerpName:", perpName)
	log.Println("FutureName:", futureName)
	log.Println("Grids Bellow:")
	var totalQty float64
	for index, grid := range grids {
		gridQty := float64(grid.CloseChance+grid.OpenChance) * grid.Qty
		totalQty += gridQty
		log.Printf("[%03d] %v %v %v %v %v %v %v %v -- gridQty=%v accQty=%v distance=%0.6v", index,
			grid.OpenAt, grid.CloseAt, grid.OpenChance, grid.CloseChance, grid.Qty,
			grid.CloseOnly, grid.OpenOnly, grid.OneShoot, gridQty, totalQty,
			grid.OpenAt-grid.CloseAt,
		)
	}
}

func place(clientId string, market string, side string, price float64, _type string, size float64, reduce bool, post bool) {
	log.Infoln("PlaceOrder", market, side, price, _type, size, "reduce", reduce, "postonly", post)
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
	fmt.Fprintf(buf, "openDiff,closeDiff,openChance,closeChance,qty,closeOnly,openOnly,oneShoot\n")
	for _, grid := range grids {
		fmt.Fprintf(buf, "%v,%v,%v,%v,%v,%v,%v,%v\n", grid.OpenAt, grid.CloseAt, grid.OpenChance, grid.CloseChance, grid.Qty, excelBool(grid.CloseOnly), excelBool(grid.OpenOnly), excelBool(grid.OneShoot))
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
			OpenAt:      mustFloat(records[row][0]),
			CloseAt:     mustFloat(records[row][1]),
			OpenChance:  mustFloat(records[row][2]),
			CloseChance: mustFloat(records[row][3]),
			Qty:         mustFloat(records[row][4]),
			CloseOnly:   mustBool(records[row][5]),
			OpenOnly:    mustBool(records[row][6]),
			OneShoot:    mustBool(records[row][7]),
			openOrders:  NewOrderMap(),
			closeOrders: NewOrderMap(),
		})
	}
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
	DeleteAt   time.Time
	Grid       *TradeGrid
	Side       string
}

type TradeGrid struct {
	OpenAt      float64
	CloseAt     float64
	OpenChance  float64
	CloseChance float64
	Qty         float64
	CloseOnly   bool
	OpenOnly    bool
	OneShoot    bool

	openOrders  *OrderMap
	closeOrders *OrderMap
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
