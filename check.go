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

	bid1, ask1 = perp.Bid, perp.Ask

	// 撤掉离盘口太远的订单
	for _, grid := range grids {
		// 低于当前盘口太远的买档位撤销
		for _, order := range grid.OpenOrders.Orders {
			if grid.OpenAt < bid1 {
				if grid.OpenAt < bid1*0.92 && time.Now().Sub(order.DeleteAt) > time.Second*20 {
					client.deleteOrder(order.Id)
					order.DeleteAt = time.Now()
				}
			}
		}

		// 高于当前盘口太远的卖盘不挂
		for _, order := range grid.CloseOrders.Orders {
			if grid.CloseAt > ask1 {
				if grid.CloseAt > ask1*1.08 && time.Now().Sub(order.DeleteAt) > time.Second*20 {
					client.deleteOrder(order.Id)
					order.DeleteAt = time.Now()
				}
			}
		}
	}

	changed := false
	for index, grid := range grids {
		// 买入仅仅当行情大于格子价格才会形成挂单

		if grid.OpenChance >= perp.SizeIncrement && grid.OpenAt > (bid1*0.95) {
			clientId := uuid.New().String()
			order := &GridOrder{
				ClientId: clientId,
				Qty:      grid.OpenChance,
				CreateAt: time.Now(),
				Grid:     grid,
				Side:     "buy",
			}
			qty := grid.OpenChance
			grid.OpenChance -= grid.OpenChance
			grid.OpenOrders.add(order)
			persistGrids() // 提前持久话避免崩溃丢失

			place(clientId, perpName, "buy", grid.OpenAt, "limit", qty, false, true)
			orderMap.add(order)
		}

		if grid.CloseChance >= perp.SizeIncrement && grid.CloseAt >= ask1 && grid.CloseAt < ask1*1.05 {
			clientId := uuid.New().String()
			order := &GridOrder{
				ClientId: clientId,
				Qty:      grid.CloseChance,
				CreateAt: time.Now(),
				Grid:     grid,
				Side:     "sell",
			}
			grid.CloseOrders.add(order)
			qty := grid.CloseChance
			grid.CloseChance -= grid.CloseChance
			persistGrids() // 提前持久话避免崩溃丢失

			orderMap.add(order)

			place(clientId, perpName, "sell", grid.CloseAt, "limit", qty, false, false)
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
		gridOrder.EQty = order.FilledSize
		if order.Side == "buy" {
			grid.CloseChance += delta
			grid.OpenTotal += delta
		} else {
			grid.OpenChance += delta
			grid.CloseTotal += delta

			profitTotal += delta * (grid.CloseAt - grid.OpenAt)
		}
	}

	// 订单关闭处理未成交部分
	if closed {
		if order.Side == "buy" {
			grid.OpenChance += order.Size - order.FilledSize
			grid.OpenOrders.remove(order.ClientID)
		} else {
			grid.CloseChance += order.Size - order.FilledSize
			grid.CloseOrders.remove(order.ClientID)
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
		grid.OpenOrders.remove(clientId)
	} else {
		grid.CloseChance += gridOrder.Qty
		grid.CloseOrders.remove(clientId)
	}
	orderMap.remove(clientId)
}

var RejectOrder func(clientId, side string)
