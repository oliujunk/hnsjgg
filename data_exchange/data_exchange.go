package data_exchange

import (
	"bytes"
	"context"
	"github.com/bitly/go-simplejson"
	"github.com/robfig/cron/v3"
	"io"
	"log"
	"net/http"
	"oliujunk/hnsjgg/api"
	"oliujunk/hnsjgg/database"
	"oliujunk/hnsjgg/protocol"
	"time"
)

var (
	Job   *cron.Cron
	Token string
)

type SwipingJob struct {
	Name   string
	Device *protocol.StatefulDevice
}

func (job *SwipingJob) Run() {
	log.Printf("[%s]: 设备刷卡, 当前状态: [%s]", job.Device.Device.Sn, job.Device.FSM.Current())

	card := database.Card{}
	_, _ = database.Orm.Where("area_code = ?", job.Device.Device.AreaCode).Get(&card)
	job.Device.Card = &card

	if job.Device.Device.UploadedWater < job.Device.WaterSum || job.Device.Device.UploadedElectric < job.Device.ElectricSum {
		job.Device.Water = job.Device.WaterSum - job.Device.Device.UploadedWater
		job.Device.Electric = job.Device.ElectricSum - job.Device.Device.UploadedElectric

		if job.Device.FSM.Current() == protocol.POWER_ON {
			err := job.Device.FSM.Event(context.Background(), protocol.NOT_STARTED_SWIPING_CARD)
			if err != nil {
				log.Println(err)
				return
			}
		} else if job.Device.FSM.Current() == protocol.STARTED {
			err := job.Device.FSM.Event(context.Background(), protocol.STARTED_SWIPING_CARD)
			if err != nil {
				log.Println(err)
				return
			}
		}
	} else {
		log.Printf("[%s]: 无需刷卡", job.Device.Device.Sn)
	}
}

func Start() {
	log.Println("设备模拟")

	updateToken()

	Job = cron.New(
		cron.WithSeconds(),
		cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))

	Job.Start()

	total, err := database.Orm.Count(&database.Device{})

	if err != nil {
		log.Println(err)
		return
	}
	log.Println(total)
	//for i := 0; i < int(total); i++ {
	for i := 0; i < 100; i++ {
		device := database.Device{}
		_, _ = database.Orm.Id(i + 1).Get(&device)

		client := &http.Client{Timeout: 5 * time.Second}
		req, err := http.NewRequest("GET", "http://101.34.116.221:8005/well_irrigations/data/current/"+device.Sn, nil)
		if err != nil {
			log.Println(err)
			continue
		}
		req.Header.Set("token", Token)
		resp, err := client.Do(req)
		if err != nil {
			log.Println(err, "获取数据异常")
			continue
		}
		result, _ := io.ReadAll(resp.Body)
		buf := bytes.NewBuffer(result)
		message, err := simplejson.NewFromReader(buf)
		if err != nil {
			log.Println(err, "获取数据异常")
			continue
		}
		dataTime := message.Get("dataTime").MustString()
		waterSum := message.Get("waterSum").MustFloat64()
		electricSum := message.Get("electricSum").MustFloat64()
		log.Println(device.Sn, dataTime, waterSum, electricSum)

		statefulDevice, err := protocol.NewStatefulDevice(&device)
		if err != nil {
			log.Println(err)
			return
		}
		statefulDevice.WaterSum = uint64(waterSum * 100)
		statefulDevice.ElectricSum = uint64(electricSum * 100)

		if statefulDevice.Device.Registered {
			err = statefulDevice.FSM.Event(context.Background(), protocol.REGISTERED_INIT)
			if err != nil {
				log.Println(err)
				return
			}
		} else {
			err = statefulDevice.FSM.Event(context.Background(), protocol.UNREGISTERED_INIT)
			if err != nil {
				log.Println(err)
				return
			}
		}

		//_, _ = Job.AddFunc("*/10 * * * * *", statefulDevice.CheckState)
		_, _ = Job.AddFunc("@every 20s", statefulDevice.CheckState)
		_, _ = Job.AddJob("@every 37s", &SwipingJob{Name: statefulDevice.Device.Sn, Device: statefulDevice})

		time.Sleep(1 * time.Second)
	}

}

func updateToken() {
	Token = api.RNGetToken("minquan", "88888888")
}
