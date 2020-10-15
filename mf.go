package main

import (
	"bytes"
	"fmt"
	"sort"
	"time"
)

func mfLoop() {
	for {
		time.Sleep(time.Second * 10)
		account, err := client.getAccount()
		if err != nil {
			log.WithError(err).Errorln("getAccount")
			continue
		}

		log.WithField("mf", account.MarginFraction).Info("PrintMF")
		if account.MarginFraction <= 0 || account.MarginFraction > 0.1 {
			continue
		}

		sort.Slice(account.Positions, func(i, j int) bool {
			return account.Positions[i].Future < account.Positions[j].Future
		})

		sendDingMF(account)
	}
}

func sendDingMF(accountInfo *AccountInfo) {
	buf := bytes.NewBuffer(nil)
	fmt.Fprintln(buf, "【保证金告警】")
	fmt.Fprintln(buf, "资产总额：", accountInfo.Collateral)
	fmt.Fprintln(buf, "可用资产：", accountInfo.FreeCollateral)
	fmt.Fprintln(buf, "保证金率：", accountInfo.MarginFraction)
	fmt.Fprintln(buf, "持仓列表：")
	for _, pos := range accountInfo.Positions {
		if pos.NetSize == 0 {
			continue
		}
		fmt.Fprintf(buf, "     - %-4v %-10v net:%+v pnl:%+v\n", pos.Side, pos.Future, pos.NetSize, pos.UnrealizedPnl)
	}

	SendDingtalkText(ding, buf.String())
}
