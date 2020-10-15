package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
)

var (
	dingInitOnce sync.Once

	dingAsyncBuffer *bytes.Buffer
	dingBufferMutex sync.Mutex
)

type TextMessage struct {
	MsgType string `json:"msgtype"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
}

func SendDingtalkText(url string, text string) {
	SendDingtalk(url, "", text)
}

func SendDingtalk(url string, title string, text string) {
	msg := &TextMessage{
		MsgType: "text",
	}

	log.Println(text)
	msg.Text.Content = string(text)

	b, err := json.Marshal(&msg)
	if err != nil {
		panic(err)
	}

	httpClient := http.Client{
		Timeout: time.Second * 10,
	}
	rsp, err := httpClient.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return
	}
	r, _ := ioutil.ReadAll(rsp.Body)
	log.Println("Ding result", string(r))
	defer rsp.Body.Close()
}

func SendDingTalkAsync(text string) {
	dingInitOnce.Do(func() {
		dingAsyncBuffer = bytes.NewBuffer(nil)
		go func() {
			for {
				time.Sleep(time.Second * 10)
				func() {
					dingBufferMutex.Lock()
					defer dingBufferMutex.Unlock()
					if dingAsyncBuffer.Len() == 0 {
						return
					}
					SendDingtalk(ding, "", fmt.Sprintf("----告警----\n%s", dingAsyncBuffer.Bytes()))
					dingAsyncBuffer.Reset()
				}()
			}
		}()
	})

	dingBufferMutex.Lock()
	defer dingBufferMutex.Unlock()

	dingAsyncBuffer.WriteString(text + "\n")
}
