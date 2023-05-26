package data_exchange

import (
	"context"
	"log"
	"oliujunk/hnsjgg/database"
	"oliujunk/hnsjgg/protocol"
)

func Start() {
	log.Println("设备模拟")

	device := database.Device{}
	_, _ = database.Orm.Id(1).Get(&device)

	statefulDevice, err := protocol.NewStatefulDevice(&device)
	if err != nil {
		log.Println(err)
		return
	}

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

}
