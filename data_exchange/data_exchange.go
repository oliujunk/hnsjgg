package data_exchange

import (
	"bytes"
	"context"
	"github.com/bitly/go-simplejson"
	"github.com/robfig/cron/v3"
	"io"
	"log"
	"math/rand"
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

	if job.Device.Device.UploadedWater < job.Device.WaterSum && job.Device.Uploaded == false {
		job.Device.Water = job.Device.WaterSum - job.Device.Device.UploadedWater

		if job.Device.Water > 6000 {
			job.Device.Water = uint64(rand.Intn(3000)) + 3000
		}
		job.Device.Electric = job.Device.Water / uint64(40+rand.Intn(8)) * 10

		costMinute := float32(job.Device.Electric) / 10.0 / float32(30+rand.Intn(10)) * 60
		job.Device.StartTime = time.Now().AddDate(0, -1, 0)
		job.Device.StopTime = job.Device.StartTime.Add(time.Minute * time.Duration(costMinute))

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

	for i := 2; i < 3; i++ {
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
		waterSum := float64(0)
		electricSum := float64(0)
		dataTime := ""
		if err != nil {
			log.Println(err, "获取数据异常")
		} else {
			dataTime = message.Get("dataTime").MustString()
			waterSum = message.Get("waterSum").MustFloat64()
			electricSum = message.Get("electricSum").MustFloat64()
		}

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

func updateUsedWater(totalDeviceCount int) {
	for i := 0; i < totalDeviceCount; i++ {
		device := database.Device{}
		_, _ = database.Orm.Id(i).Get(&device)
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
		waterSum := float64(0)
		if err != nil {
			log.Println(err, "获取数据异常")
		} else {
			waterSum = message.Get("waterSum").MustFloat64()
			device.UsedWater = uint64(waterSum * 100)
			_, err := database.Orm.Id(device.ID).AllCols().Update(device)
			if err != nil {
				return
			}
		}

		time.Sleep(1 * time.Second)
	}
}
