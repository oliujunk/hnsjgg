package database

import (
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/go-xorm/xorm"
	"log"
	"oliujunk/hnsjgg/config"
	"time"
)

var (
	// Orm orm引擎
	Orm *xorm.Engine
)

type Device struct {
	ID             int64     `xorm:"id pk" json:"id"`
	CreateTime     time.Time `json:"createTime"`
	UpdateTime     time.Time `json:"updateTime"`
	Sn             string    `xorm:"sn" json:"sn"`
	Number         int       `json:"number"`
	AreaCode       int64     `json:"areaCode"`
	Town           string    `json:"town"`
	Village        string    `json:"village"`
	Longitude      float64   `json:"longitude"`
	Latitude       float64   `json:"latitude"`
	RegisterNumber string    `json:"registerNumber"`
	Registered     bool      `json:"registered"`
}

func init() {
	// 数据库
	var err error
	Orm, err = xorm.NewEngine("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/hnsjgg",
		config.GlobalConfiguration.Database.Username,
		config.GlobalConfiguration.Database.Password,
		config.GlobalConfiguration.Database.Host,
		config.GlobalConfiguration.Database.Port,
	))

	if err != nil {
		log.Fatal(err)
	}

	//Orm.ShowSQL(true)
}
