package api

import (
	"bytes"
	"encoding/json"
	"github.com/bitly/go-simplejson"
	"io"
	"log"
	"net/http"
	"time"
)

func RNGetToken(username, password string) string {
	// 超时时间：5秒
	client := &http.Client{Timeout: 5 * time.Second}
	loginParam := map[string]string{"username": username, "password": password}
	jsonStr, _ := json.Marshal(loginParam)
	resp, err := client.Post("http://101.34.116.221:8005/login", "application/json", bytes.NewBuffer(jsonStr))
	if err != nil {
		log.Println(err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	result, _ := io.ReadAll(resp.Body)
	buf := bytes.NewBuffer(result)
	message, err := simplejson.NewFromReader(buf)
	if err != nil {
		log.Println(err)
		return ""
	}
	token := message.Get("token").MustString()
	return token
}
