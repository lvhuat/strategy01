package main

import (
	"bytes"
	"fmt"
	"sort"
	"time"
)

func monitorPosition() {
	reportAccount()
	for {
		time.Sleep(time.Second)

		if time.Now().Sub(lastPlaceTime) < time.Second*15 || time.Now().Sub(lastPlaceTime) > time.Second*20 {
			continue
		}

		reportAccount()

		time.Sleep(time.Second * 6)
	}
}

func reportAccount() {
	accountInfo, err := client.getAccount()
	if err != nil {
		log.WithError(err).Errorln("getAccount")
		return
	}

	sort.Slice(accountInfo.Positions, func(i, j int) bool {
		return accountInfo.Positions[i].Future < accountInfo.Positions[j].Future
	})

	positions, err := client.getPositionsEx()
	if err != nil {
		log.WithError(err).Errorln("getPositionsEx")
	}

	sendDingAccount(accountInfo, positions)
}

func sendDingAccount(accountInfo *AccountInfo, positions []Position) {
	buf := bytes.NewBuffer(nil)
	fmt.Fprintln(buf, "【持仓告警】")
	fmt.Fprintln(buf, "资产总额：", accountInfo.Collateral)
	fmt.Fprintln(buf, "可用资产：", accountInfo.FreeCollateral)
	fmt.Fprintln(buf, "持仓列表：")
	if len(positions) == 0 {
		for _, pos := range accountInfo.Positions {
			switch pos.Future {
			case perpName:
				fmt.Fprintf(buf, "     - %-4v %-10v %+v \n", pos.Side, pos.Future, pos.NetSize)
			case futureName:
				fmt.Fprintf(buf, "     - %-4v %-10v %+v \n", pos.Side, pos.Future, pos.NetSize)
			}
		}
	} else {
		perpPrice := 0.0
		futuPrice := 0.0
		for _, pos := range positions {
			switch pos.Future {
			case perpName:
				perpPrice = pos.RecentAverageOpenPrice
				fmt.Fprintf(buf, "     - %-4v %-10v %+v %v\n", pos.Side, pos.Future, pos.NetSize, pos.RecentAverageOpenPrice)
			case futureName:
				futuPrice = pos.RecentAverageOpenPrice
				fmt.Fprintf(buf, "     - %-4v %-10v %+v %v\n", pos.Side, pos.Future, pos.NetSize, pos.RecentAverageOpenPrice)
			}
		}
		if perpPrice != 0 && futuPrice != 0 {
			fmt.Fprintf(buf, "期现价差：%v", 100*(futuPrice-perpPrice)/perpPrice)
		}
	}

	SendDingtalkText(ding, buf.String())
}
