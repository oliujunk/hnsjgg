package main

import (
	"log"
	"oliujunk/hnsjgg/data_exchange"
)

func init() {
	// 日志信息添加文件名行号
	log.SetFlags(log.Lshortfile | log.LstdFlags)
}

func main() {

	data_exchange.Start()

	select {}
}
