package main

import (
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type PriceDiff struct {
	premium   bool
	opendiff  float64
	closediff float64
	diffPrice float64
	perp      string
	future    string
}

func check() bool {
	since := time.Now()

	perp := &FuturesItem{}
	resp, err := client.getFuture(perpName)
	if err != nil {
		log.Println("getMarkets:", err)
		return false
	}
	if err := parseResult(resp, &perp); err != nil {
		log.Println("parseResult:", err)
		return false
	}

	// 高延迟行情不处理
	takeTime := time.Now().Sub(since)
	if takeTime > time.Millisecond*3000 {
		return false
	}

	bid1 := perp.Bid

	changed := false
	for index, grid := range grids {
		// 买入仅仅当行情大于格子价格才会形成挂单
		if grid.OpenChance > 0.0 && grid.OpenAt <= bid1 {
			clientId := uuid.New().String()
			place(clientId, perpName, "buy", grid.OpenAt, "limit", grid.OpenChance, false, true)
			order := &GridOrder{
				ClientId: clientId,
				Qty:      grid.Qty,
				CreateAt: time.Now(),
				Grid:     grid,
				Side:     "buy",
			}
			grid.openOrders.add(order)
			orderMap.add(order)
			grid.OpenChance -= grid.Qty
		}

		if grid.CloseChance > 0.0 {
			clientId := uuid.New().String()
			place(clientId, perpName, "sell", grid.CloseAt, "limit", grid.CloseChance, false, false)
			order := &GridOrder{
				ClientId: clientId,
				Qty:      grid.Qty,
				CreateAt: time.Now(),
				Grid:     grid,
				Side:     "sell",
			}
			grid.closeOrders.add(order)
			orderMap.add(order)
			grid.CloseChance -= grid.Qty
		}

		if changed {
			log.WithFields(logrus.Fields{
				"openChance":  grid.OpenChance,
				"closeChance": grid.CloseChance,
			}).Infoln("Grid triggered", index)
			break
		}
	}

	return changed
}

func onOrderChange(order *Order) {
	gridOrder, found := orderMap.get(order.ClientID)
	if !found {
		return
	}

	delta := order.FilledSize - gridOrder.EQty
	closed := order.Status == "closed"
	grid := gridOrder.Grid // 订单归属网格

	if gridOrder.Id == 0 {
		gridOrder.Id = order.ID
	}
	gridOrder.UpdateTime = time.Now()

	// 订单未处理成交部分
	if delta > 0.0 {
		if order.Side == "buy" {
			grid.CloseChance += delta
		} else {
			grid.OpenChance += delta
		}
	}

	// 订单关闭处理未成交部分
	if closed {
		if order.Side == "buy" {
			grid.OpenChance += order.Size - order.FilledSize
			grid.openOrders.remove(order.ClientID)
		} else {
			grid.CloseChance += order.Size - order.FilledSize
			grid.closeOrders.remove(order.ClientID)
		}

		// 从全局订单表中移除订单
		orderMap.remove(order.ClientID)
	}
}

func onRejectOrder(clientId, side string) {
	logrus.Infoln("RejectOrder", clientId, side)
	gridOrder, found := orderMap.get(clientId)
	if !found {
		return
	}
	grid := gridOrder.Grid // 订单归属网格

	if side == "buy" {
		grid.OpenChance += gridOrder.Qty
		grid.openOrders.remove(clientId)
	} else {
		grid.CloseChance += gridOrder.Qty
		grid.closeOrders.remove(clientId)
	}
	orderMap.remove(clientId)
}

var RejectOrder func(clientId, side string)
