package main

import (
	"encoding/json"
	"flag"
	"time"

	"github.com/lvhuat/textformatter"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

var (
	log = logrus.WithFields(logrus.Fields{})
)

var gridFile = flag.String("grid", "grid.csv", "网格文件")
var cfgFile = flag.String("cfg", "config.json", "基本配置文件")
var testMode = flag.Bool("test", false, "仅打印不会下单，不会执行网格")
var mf = flag.Bool("mf", false, "仅监控保证金率")

type EventRejectOrder struct {
	ClientId string
	Side     string
}

func main() {
	logrus.SetFormatter(&textformatter.TextFormatter{})

	flag.Parse()

	if *cfgFile != "" {
		loadBaseConfigAndAssign(*cfgFile)
	}
	wsclient := WebsocketClient{
		apiKey:     apiKey,
		secret:     []byte(secretKey),
		subAccount: subAccount,
	}
	wsclient.dial(false)
	wsclient.ping()
	wsclient.login()
	wsclient.subOrder()
	eventChan := make(chan interface{}, 1000)
	wsclient.onOrderChange = func(body []byte) {
		order := Order{}
		raw := gjson.GetBytes(body, "data").Raw
		json.Unmarshal([]byte(raw), &order)
		if order.ClientID == "" {
			return
		}
		eventChan <- order
	}

	RejectOrder = func(clientId, side string) {
		eventChan <- &EventRejectOrder{
			ClientId: clientId,
			Side:     side,
		}
	}

	if *mf {
		mfLoop()
		return
	}

	if *gridFile != "" {
		loadGridConfigAndAssign(*gridFile)
	}

	// 打印网格配置
	debugGrid()
	// 打印持仓
	debugPositions()

	writeGridCurrent()
	for i := 3; i > 0; i-- {
		log.Infoln("Counting ", i)
		time.Sleep(time.Second)
	}

	log.Infoln("Good luck!")

	// go monitorPosition()
	// go mfLoop()

	// 执行网格
	wait := checkInterval
	lastSyncOrderTime := time.Now()
	for {
		select {
		case <-time.After(wait):
			check()
			writeGridCurrent()
			wait = quickRecheckInterval
		case event := <-eventChan:
			switch event.(type) {
			case *Order:
				onOrderChange(event.(*Order))
			case *EventRejectOrder:
				data := event.(*EventRejectOrder)
				onRejectOrder(data.ClientId, data.Side)
			}
			continue
		}

		if time.Now().Sub(lastSyncOrderTime) < time.Second*5 {
			time.Sleep(wait)
			continue
		}

		// 定时刷新订单状态
		orders, err := client.getOrders(perpName)
		if err != nil {
			logrus.WithError(err).Errorln("GetOpenOrders")
			continue
		}
		for _, order := range orders {
			onOrderChange(order)
		}

		// 未能及时同步的订单，将采用单个同步的方式同步
		orderMap.RangeOver(func(order *GridOrder) bool {
			if time.Now().Sub(order.UpdateTime) < time.Second*20 {
				return true
			}
			ftxOrder, err := client.getOrderByClient(order.ClientId)
			if err != nil {
				switch err.Error() {
				case "Order not found":
					RejectOrder(order.ClientId, order.Side)
				}

				logrus.WithError(err).Errorln("GetOrder", order.ClientId)
				return true
			}
			onOrderChange(ftxOrder)
			return true
		})

	}
}
