package protocol

import (
	"bytes"
	"context"
	"encoding/hex"
	"github.com/looplab/fsm"
	"log"
	"net"
	"oliujunk/hnsjgg/database"
	"time"
)

func byteToBcd(value byte) byte {
	bcdHigh := 0
	for {
		if value < 10 {
			break
		}
		bcdHigh++
		value -= 10
	}
	return byte((bcdHigh << 4) | int(value))
}

func bcdToByte(value byte) byte {
	tmp := 0
	tmp = int(((value & 0xF0) >> 4) * 10)
	return byte(tmp + (int(value) & 0x0F))
}

func crc8(buf []byte, len int) uint8 {
	var crc uint8 = 0

	if len == 0 {
		return 0
	}

	for i := 0; i < len; i++ {
		crc ^= buf[i]
		for j := 8; j > 0; j-- {
			if (crc & 0x80) != 0 {
				crc = (crc << 1) ^ 0x07
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

func generateOrderNumber(number uint64) uint64 {
	var orderNumber uint64 = 0
	orderNumber += (uint64(time.Now().Year() - 2000)) * 100000000000000
	orderNumber += (uint64(time.Now().Month())) * 1000000000000
	orderNumber += (uint64(time.Now().Day())) * 10000000000
	orderNumber += (uint64(time.Now().Hour())) * 100000000
	orderNumber += (uint64(time.Now().Minute())) * 1000000
	orderNumber += (uint64(time.Now().Second())) * 10000
	orderNumber += number
	return orderNumber
}

// 设备状态
const (
	INITIAL                 string = "initial"                 // 初始状态
	UNREGISTERED            string = "unregistered"            // 未注册
	UNREGISTERED_CONFIRMED  string = "unregistered_confirmed"  // 待注册确认
	REGISTERED              string = "registered"              // 已注册
	POWER_ON                string = "power_on"                // 已开机
	VALID_USER              string = "valid_user"              // 有效用户
	INVALID_USER            string = "invalid_user"            // 无效用户
	STARTED                 string = "started"                 // 正在灌溉
	NOT_STARTED_SWIPED_CARD string = "not_started_swiped_card" // 未灌溉已刷卡
	STARTED_SWIPED_CARD     string = "started_swiped_card"     // 灌溉中已刷卡

)

// 设备事件
const (
	UNREGISTERED_INIT         string = "unregistered_init"         //未注册设备初始化
	REGISTERED_INIT           string = "registered_init"           //已注册设备初始化
	REGISTER_REPLY            string = "register_reply"            // 注册回复
	REGISTER_CONFIRMED_REPLY  string = "register_confirmed_reply"  // 注册确认回复
	POWER_ON_REPLY            string = "power_on_reply"            // 开机回复
	SEARCH_USER_VALID_REPLY   string = "search_user_valid_reply"   // 查询用户有效回复
	SEARCH_USER_INVALID_REPLY string = "search_user_invalid_reply" // 查询用户无效回复
	OPEN_WELL_REPLY           string = "open_well_reply"           // 开井回复
	OPEN_WELL_DATA_REPLY      string = "open_well_data_reply"      // 开井实时报回复
	CLOSE_WELL_REPLY          string = "close_well_reply"          // 关井回复
	NOT_STARTED_SWIPING_CARD  string = "not_started_swiping_card"  // 未灌溉刷卡
	STARTED_SWIPING_CARD      string = "started_swiping_card"      // 灌溉中刷卡
)

type StatefulDevice struct {
	Device      *database.Device
	FSM         *fsm.FSM
	Conn        net.Conn
	OrderCount  uint64
	OrderNumber uint64
	Card        *database.Card
	Water       uint64
	Electric    uint64
	WaterSum    uint64
	ElectricSum uint64
}

func NewStatefulDevice(device *database.Device) (*StatefulDevice, error) {
	statefulDevice := &StatefulDevice{
		Device:      device,
		OrderCount:  0,
		OrderNumber: 0,
	}

	//conn, err := net.Dial("tcp", "127.0.0.1:8888")
	conn, err := net.Dial("tcp", "newreceive.hnsjgg.com:9999")
	if err != nil {
		log.Println(err)
		return nil, err
	}

	statefulDevice.Conn = conn

	statefulDevice.FSM = fsm.NewFSM(
		INITIAL,
		fsm.Events{
			{Name: UNREGISTERED_INIT, Src: []string{INITIAL}, Dst: UNREGISTERED},
			{Name: REGISTERED_INIT, Src: []string{INITIAL}, Dst: REGISTERED},
			{Name: REGISTER_REPLY, Src: []string{UNREGISTERED}, Dst: UNREGISTERED_CONFIRMED},
			{Name: REGISTER_CONFIRMED_REPLY, Src: []string{UNREGISTERED_CONFIRMED}, Dst: REGISTERED},
			{Name: POWER_ON_REPLY, Src: []string{REGISTERED}, Dst: POWER_ON},
			{Name: NOT_STARTED_SWIPING_CARD, Src: []string{POWER_ON}, Dst: NOT_STARTED_SWIPED_CARD},
			{Name: SEARCH_USER_VALID_REPLY, Src: []string{NOT_STARTED_SWIPED_CARD}, Dst: VALID_USER},
			{Name: SEARCH_USER_INVALID_REPLY, Src: []string{NOT_STARTED_SWIPED_CARD}, Dst: INVALID_USER},
			{Name: OPEN_WELL_REPLY, Src: []string{VALID_USER}, Dst: STARTED},
			{Name: OPEN_WELL_DATA_REPLY, Src: []string{STARTED}, Dst: STARTED},
			{Name: CLOSE_WELL_REPLY, Src: []string{STARTED_SWIPED_CARD}, Dst: POWER_ON},
			{Name: STARTED_SWIPING_CARD, Src: []string{STARTED}, Dst: STARTED_SWIPED_CARD},
		},
		fsm.Callbacks{
			"enter_state": func(_ context.Context, e *fsm.Event) { statefulDevice.enterState(e) },
		},
	)

	return statefulDevice, nil
}

func (statefulDevice *StatefulDevice) CheckState() {
	switch statefulDevice.FSM.Current() {
	case POWER_ON:
		dlt645Heartbeat(statefulDevice)
		break

	case STARTED:
		dlt645OpenWellData(statefulDevice)
		break

	default:
		break
	}
}

func (statefulDevice *StatefulDevice) enterState(e *fsm.Event) {
	log.Printf("[%s]: 事件: [%s]", statefulDevice.Device.Sn, e.Event)
	switch e.Event {
	case UNREGISTERED_INIT:
		dlt645Register(statefulDevice)
		break

	case REGISTERED_INIT:
		dlt645PowerON(statefulDevice)
		break

	case REGISTER_REPLY:
		dlt645RegisterConfirm(statefulDevice)
		break

	case REGISTER_CONFIRMED_REPLY:
		dlt645PowerON(statefulDevice)
		break

	case POWER_ON_REPLY:
		break

	case SEARCH_USER_VALID_REPLY:
		dlt645OpenWell(statefulDevice)
		break

	case SEARCH_USER_INVALID_REPLY:
		break

	case OPEN_WELL_REPLY:
		break

	case OPEN_WELL_DATA_REPLY:
		break

	case CLOSE_WELL_REPLY:
		break

	case NOT_STARTED_SWIPING_CARD:
		dlt645SearchCard(statefulDevice)
		break

	case STARTED_SWIPING_CARD:
		dlt645CloseWell(statefulDevice)
		break

	default:
		break
	}
}

func dlt645RecvProcess(buf []byte, statefulDevice *StatefulDevice) {
	log.Println(hex.EncodeToString(buf))

	switch buf[4] {
	case 0x83:
		statefulDevice.Device.RegisterNumber = hex.EncodeToString(buf[5 : 5+16])

		err := statefulDevice.FSM.Event(context.Background(), REGISTER_REPLY)
		if err != nil {
			log.Println(err)
			return
		}
		break

	case 0x84:
		statefulDevice.Device.Registered = true
		_, err := database.Orm.Id(statefulDevice.Device.ID).AllCols().Update(statefulDevice.Device)
		if err != nil {
			log.Println(err)
			return
		}
		err = statefulDevice.FSM.Event(context.Background(), REGISTER_CONFIRMED_REPLY)
		if err != nil {
			log.Println(err)
			return
		}
		break

	case 0x86:
		err := statefulDevice.FSM.Event(context.Background(), POWER_ON_REPLY)
		if err != nil {
			log.Printf("[%s]: %s", statefulDevice.Device.Sn, err)
			return
		}
		break

	case 0x87:
		status := buf[5]
		if status != 0 {
			err := statefulDevice.FSM.Event(context.Background(), SEARCH_USER_INVALID_REPLY)
			if err != nil {
				log.Printf("[%s]: %s", statefulDevice.Device.Sn, err)
				return
			}
		} else {
			err := statefulDevice.FSM.Event(context.Background(), SEARCH_USER_VALID_REPLY)
			if err != nil {
				log.Printf("[%s]: %s", statefulDevice.Device.Sn, err)
				return
			}
		}
		break

	case 0x88:
		err := statefulDevice.FSM.Event(context.Background(), OPEN_WELL_REPLY)
		if err != nil {
			log.Println(err)
			return
		}
		break

	case 0x89:
		err := statefulDevice.FSM.Event(context.Background(), OPEN_WELL_DATA_REPLY)
		if err != nil {
			log.Println(err)
			return
		}
		break

	case 0x90:

		statefulDevice.Device.UploadedWater = statefulDevice.WaterSum
		statefulDevice.Device.UploadedElectric = statefulDevice.ElectricSum
		_, err := database.Orm.Id(statefulDevice.Device.ID).AllCols().Update(statefulDevice.Device)
		if err != nil {
			log.Println(err)
			return
		}

		err = statefulDevice.FSM.Event(context.Background(), CLOSE_WELL_REPLY)
		if err != nil {
			log.Println(err)
			return
		}
		break

	default:
		break
	}
}

func dlt645Register(statefulDevice *StatefulDevice) {
	var buf bytes.Buffer
	buf.WriteByte(0x68)
	buf.WriteByte(0)
	buf.WriteByte(0x68)
	buf.WriteByte(0x01)
	buf.WriteByte(byteToBcd(byte(statefulDevice.Device.AreaCode / 10000000000)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.Device.AreaCode / 100000000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.Device.AreaCode / 1000000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.Device.AreaCode / 10000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.Device.AreaCode / 100 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.Device.AreaCode % 100)))
	buf.WriteByte(byte(statefulDevice.Device.Number & 0xFF))
	buf.WriteByte(byte((statefulDevice.Device.Number >> 8) & 0xFF))
	buf.WriteByte(byte((statefulDevice.Device.Number >> 16) & 0xFF))
	buf.WriteByte(0x83)
	dataLen := byte(buf.Len() - 3)
	data := buf.Bytes()
	data[1] = dataLen
	buf.WriteByte(crc8(data, buf.Len()))
	buf.WriteByte(0x16)

	_, err := statefulDevice.Conn.Write(buf.Bytes())
	if err != nil {
		log.Println(err)
		return
	}
	log.Printf("[%s]: 设备注册", statefulDevice.Device.Sn)

	_ = statefulDevice.Conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	recvBuffer := make([]byte, 512)
	n, err := statefulDevice.Conn.Read(recvBuffer)
	if err != nil {
		log.Println(err)
		return
	}

	dlt645RecvProcess(recvBuffer[:n], statefulDevice)
}

func dlt645RegisterConfirm(statefulDevice *StatefulDevice) {
	var buf bytes.Buffer
	buf.WriteByte(0x68)
	buf.WriteByte(0)
	buf.WriteByte(0x68)
	buf.WriteByte(0x01)
	buf.WriteByte(byteToBcd(byte(statefulDevice.Device.AreaCode / 10000000000)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.Device.AreaCode / 100000000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.Device.AreaCode / 1000000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.Device.AreaCode / 10000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.Device.AreaCode / 100 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.Device.AreaCode % 100)))
	buf.WriteByte(byte(statefulDevice.Device.Number & 0xFF))
	buf.WriteByte(byte((statefulDevice.Device.Number >> 8) & 0xFF))
	buf.WriteByte(byte((statefulDevice.Device.Number >> 16) & 0xFF))
	buf.WriteByte(0x84)
	decodeString, err := hex.DecodeString(statefulDevice.Device.RegisterNumber)
	if err != nil {
		log.Print(err)
		return
	}
	buf.Write(decodeString)
	dataLen := byte(buf.Len() - 3)
	data := buf.Bytes()
	data[1] = dataLen
	buf.WriteByte(crc8(data, buf.Len()))
	buf.WriteByte(0x16)

	_, err = statefulDevice.Conn.Write(buf.Bytes())
	if err != nil {
		log.Print(err)
		return
	}
	log.Printf("[%s]: 设备注册确认", statefulDevice.Device.Sn)

	_ = statefulDevice.Conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	recvBuffer := make([]byte, 512)
	n, err := statefulDevice.Conn.Read(recvBuffer)
	if err != nil {
		log.Println(err)
		return
	}

	dlt645RecvProcess(recvBuffer[:n], statefulDevice)
}

func dlt645Heartbeat(statefulDevice *StatefulDevice) {
	var buf bytes.Buffer
	buf.WriteByte(0x68)
	buf.WriteByte(0)
	buf.WriteByte(0x68)
	buf.WriteByte(0x01)
	decodeString, err := hex.DecodeString(statefulDevice.Device.RegisterNumber)
	if err != nil {
		log.Print(err)
		return
	}
	buf.Write(decodeString)

	buf.WriteByte(byte(statefulDevice.Device.Number & 0xFF))
	buf.WriteByte(byte((statefulDevice.Device.Number >> 8) & 0xFF))
	buf.WriteByte(byte((statefulDevice.Device.Number >> 16) & 0xFF))

	buf.WriteByte(0x85)

	buf.WriteByte(0x00)
	buf.WriteByte(0x00)
	buf.WriteByte(0x00)
	buf.WriteByte(0x00)
	buf.WriteByte(0x00)
	buf.WriteByte(0x00)
	buf.WriteByte(0x00)
	buf.WriteByte(0x00)

	dataLen := byte(buf.Len() - 3)
	data := buf.Bytes()
	data[1] = dataLen
	buf.WriteByte(crc8(data, buf.Len()))
	buf.WriteByte(0x16)

	_, err = statefulDevice.Conn.Write(buf.Bytes())
	if err != nil {
		log.Print(err)
		return
	}
	log.Printf("[%s]: 设备心跳", statefulDevice.Device.Sn)
}

func dlt645PowerON(statefulDevice *StatefulDevice) {
	var buf bytes.Buffer
	buf.WriteByte(0x68)
	buf.WriteByte(0)
	buf.WriteByte(0x68)
	buf.WriteByte(0x01)
	decodeString, err := hex.DecodeString(statefulDevice.Device.RegisterNumber)
	if err != nil {
		log.Print(err)
		return
	}
	buf.Write(decodeString)

	buf.WriteByte(byte(statefulDevice.Device.Number & 0xFF))
	buf.WriteByte(byte((statefulDevice.Device.Number >> 8) & 0xFF))
	buf.WriteByte(byte((statefulDevice.Device.Number >> 16) & 0xFF))

	buf.WriteByte(0x86)

	longitude := int(statefulDevice.Device.Longitude * 1000000)
	buf.WriteByte(byteToBcd(byte(longitude / 100000000 % 100)))
	buf.WriteByte(byteToBcd(byte(longitude / 1000000 % 100)))
	buf.WriteByte(byteToBcd(byte(longitude / 10000 % 100)))
	buf.WriteByte(byteToBcd(byte(longitude / 100 % 100)))
	buf.WriteByte(byteToBcd(byte(longitude % 100)))

	latitude := int(statefulDevice.Device.Latitude * 1000000)
	buf.WriteByte(byteToBcd(byte(latitude / 100000000 % 100)))
	buf.WriteByte(byteToBcd(byte(latitude / 1000000 % 100)))
	buf.WriteByte(byteToBcd(byte(latitude / 10000 % 100)))
	buf.WriteByte(byteToBcd(byte(latitude / 100 % 100)))
	buf.WriteByte(byteToBcd(byte(latitude % 100)))

	dataLen := byte(buf.Len() - 3)
	data := buf.Bytes()
	data[1] = dataLen
	buf.WriteByte(crc8(data, buf.Len()))
	buf.WriteByte(0x16)

	_, err = statefulDevice.Conn.Write(buf.Bytes())
	if err != nil {
		log.Print(err)
		return
	}
	log.Printf("[%s]: 设备开机", statefulDevice.Device.Sn)

	_ = statefulDevice.Conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	recvBuffer := make([]byte, 512)
	n, err := statefulDevice.Conn.Read(recvBuffer)
	if err != nil {
		log.Println(err)
		return
	}

	dlt645RecvProcess(recvBuffer[:n], statefulDevice)
}

func dlt645SearchCard(statefulDevice *StatefulDevice) {
	var buf bytes.Buffer
	buf.WriteByte(0x68)
	buf.WriteByte(0)
	buf.WriteByte(0x68)
	buf.WriteByte(0x01)
	decodeString, err := hex.DecodeString(statefulDevice.Device.RegisterNumber)
	if err != nil {
		log.Print(err)
		return
	}
	buf.Write(decodeString)

	buf.WriteByte(byte(statefulDevice.Device.Number & 0xFF))
	buf.WriteByte(byte((statefulDevice.Device.Number >> 8) & 0xFF))
	buf.WriteByte(byte((statefulDevice.Device.Number >> 16) & 0xFF))

	buf.WriteByte(0x87)

	decodeString, err = hex.DecodeString(statefulDevice.Card.CardRegisterNumber)
	if err != nil {
		log.Print(err)
		return
	}
	buf.Write(decodeString)

	dataLen := byte(buf.Len() - 3)
	data := buf.Bytes()
	data[1] = dataLen
	buf.WriteByte(crc8(data, buf.Len()))
	buf.WriteByte(0x16)

	_, err = statefulDevice.Conn.Write(buf.Bytes())
	if err != nil {
		log.Print(err)
		return
	}
	log.Printf("[%s]: 查询用户信息", statefulDevice.Device.Sn)

	_ = statefulDevice.Conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	recvBuffer := make([]byte, 512)
	n, err := statefulDevice.Conn.Read(recvBuffer)
	if err != nil {
		log.Println(err)
		return
	}

	dlt645RecvProcess(recvBuffer[:n], statefulDevice)
}

func dlt645OpenWell(statefulDevice *StatefulDevice) {
	var buf bytes.Buffer
	buf.WriteByte(0x68)
	buf.WriteByte(0)
	buf.WriteByte(0x68)
	buf.WriteByte(0x01)
	decodeString, err := hex.DecodeString(statefulDevice.Device.RegisterNumber)
	if err != nil {
		log.Print(err)
		return
	}
	buf.Write(decodeString)

	buf.WriteByte(byte(statefulDevice.Device.Number & 0xFF))
	buf.WriteByte(byte((statefulDevice.Device.Number >> 8) & 0xFF))
	buf.WriteByte(byte((statefulDevice.Device.Number >> 16) & 0xFF))

	buf.WriteByte(0x88)

	decodeString, err = hex.DecodeString(statefulDevice.Card.CardRegisterNumber)
	if err != nil {
		log.Print(err)
		return
	}
	buf.Write(decodeString)

	buf.WriteByte(byteToBcd(byte(time.Now().Year() - 2000)))
	buf.WriteByte(byteToBcd(byte(time.Now().Month())))
	buf.WriteByte(byteToBcd(byte(time.Now().Day())))
	buf.WriteByte(byteToBcd(byte(time.Now().Hour())))
	buf.WriteByte(byteToBcd(byte(time.Now().Minute())))
	buf.WriteByte(byteToBcd(byte(time.Now().Second())))

	orderNumber := generateOrderNumber(statefulDevice.OrderCount)
	statefulDevice.OrderCount++
	statefulDevice.OrderNumber = orderNumber

	buf.WriteByte(byteToBcd(byte(orderNumber / 100000000000000 % 100)))
	buf.WriteByte(byteToBcd(byte(orderNumber / 1000000000000 % 100)))
	buf.WriteByte(byteToBcd(byte(orderNumber / 10000000000 % 100)))
	buf.WriteByte(byteToBcd(byte(orderNumber / 100000000 % 100)))
	buf.WriteByte(byteToBcd(byte(orderNumber / 1000000 % 100)))
	buf.WriteByte(byteToBcd(byte(orderNumber / 10000 % 100)))
	buf.WriteByte(byteToBcd(byte(orderNumber / 100 % 100)))
	buf.WriteByte(byteToBcd(byte(orderNumber / 1 % 100)))

	dataLen := byte(buf.Len() - 3)
	data := buf.Bytes()
	data[1] = dataLen
	buf.WriteByte(crc8(data, buf.Len()))
	buf.WriteByte(0x16)

	_, err = statefulDevice.Conn.Write(buf.Bytes())
	if err != nil {
		log.Print(err)
		return
	}
	log.Printf("[%s]: 开井", statefulDevice.Device.Sn)

	_ = statefulDevice.Conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	recvBuffer := make([]byte, 512)
	n, err := statefulDevice.Conn.Read(recvBuffer)
	if err != nil {
		log.Println(err)
		return
	}

	dlt645RecvProcess(recvBuffer[:n], statefulDevice)
}

func dlt645OpenWellData(statefulDevice *StatefulDevice) {
	var buf bytes.Buffer
	buf.WriteByte(0x68)
	buf.WriteByte(0)
	buf.WriteByte(0x68)
	buf.WriteByte(0x01)
	decodeString, err := hex.DecodeString(statefulDevice.Device.RegisterNumber)
	if err != nil {
		log.Print(err)
		return
	}
	buf.Write(decodeString)

	buf.WriteByte(byte(statefulDevice.Device.Number & 0xFF))
	buf.WriteByte(byte((statefulDevice.Device.Number >> 8) & 0xFF))
	buf.WriteByte(byte((statefulDevice.Device.Number >> 16) & 0xFF))

	buf.WriteByte(0x89)

	decodeString, err = hex.DecodeString(statefulDevice.Card.CardRegisterNumber)
	if err != nil {
		log.Print(err)
		return
	}
	buf.Write(decodeString)

	buf.WriteByte(byteToBcd(byte(time.Now().Year() - 2000)))
	buf.WriteByte(byteToBcd(byte(time.Now().Month())))
	buf.WriteByte(byteToBcd(byte(time.Now().Day())))
	buf.WriteByte(byteToBcd(byte(time.Now().Hour())))
	buf.WriteByte(byteToBcd(byte(time.Now().Minute())))
	buf.WriteByte(byteToBcd(byte(time.Now().Second())))

	buf.WriteByte(byteToBcd(byte(statefulDevice.OrderNumber / 100000000000000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.OrderNumber / 1000000000000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.OrderNumber / 10000000000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.OrderNumber / 100000000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.OrderNumber / 1000000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.OrderNumber / 10000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.OrderNumber / 100 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.OrderNumber / 1 % 100)))

	buf.WriteByte(byte(statefulDevice.Water & 0xFF))
	buf.WriteByte(byte((statefulDevice.Water >> 8) & 0xFF))
	buf.WriteByte(byte((statefulDevice.Water >> 16) & 0xFF))

	buf.WriteByte(byte(statefulDevice.Electric & 0xFF))
	buf.WriteByte(byte((statefulDevice.Electric >> 8) & 0xFF))
	buf.WriteByte(byte((statefulDevice.Electric >> 16) & 0xFF))

	dataLen := byte(buf.Len() - 3)
	data := buf.Bytes()
	data[1] = dataLen
	buf.WriteByte(crc8(data, buf.Len()))
	buf.WriteByte(0x16)

	_, err = statefulDevice.Conn.Write(buf.Bytes())
	if err != nil {
		log.Print(err)
		return
	}
	log.Printf("[%s]: 开井实时报", statefulDevice.Device.Sn)

	_ = statefulDevice.Conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	recvBuffer := make([]byte, 512)
	n, err := statefulDevice.Conn.Read(recvBuffer)
	if err != nil {
		log.Println(err)
		return
	}

	dlt645RecvProcess(recvBuffer[:n], statefulDevice)
}

func dlt645CloseWell(statefulDevice *StatefulDevice) {
	var buf bytes.Buffer
	buf.WriteByte(0x68)
	buf.WriteByte(0)
	buf.WriteByte(0x68)
	buf.WriteByte(0x01)
	decodeString, err := hex.DecodeString(statefulDevice.Device.RegisterNumber)
	if err != nil {
		log.Print(err)
		return
	}
	buf.Write(decodeString)

	buf.WriteByte(byte(statefulDevice.Device.Number & 0xFF))
	buf.WriteByte(byte((statefulDevice.Device.Number >> 8) & 0xFF))
	buf.WriteByte(byte((statefulDevice.Device.Number >> 16) & 0xFF))

	buf.WriteByte(0x90)

	decodeString, err = hex.DecodeString(statefulDevice.Card.CardRegisterNumber)
	if err != nil {
		log.Print(err)
		return
	}
	buf.Write(decodeString)

	buf.WriteByte(byteToBcd(byte(time.Now().Year() - 2000)))
	buf.WriteByte(byteToBcd(byte(time.Now().Month())))
	buf.WriteByte(byteToBcd(byte(time.Now().Day())))
	buf.WriteByte(byteToBcd(byte(time.Now().Hour())))
	buf.WriteByte(byteToBcd(byte(time.Now().Minute())))
	buf.WriteByte(byteToBcd(byte(time.Now().Second())))

	buf.WriteByte(byteToBcd(byte(statefulDevice.OrderNumber / 100000000000000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.OrderNumber / 1000000000000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.OrderNumber / 10000000000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.OrderNumber / 100000000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.OrderNumber / 1000000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.OrderNumber / 10000 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.OrderNumber / 100 % 100)))
	buf.WriteByte(byteToBcd(byte(statefulDevice.OrderNumber / 1 % 100)))

	buf.WriteByte(byte(statefulDevice.Water & 0xFF))
	buf.WriteByte(byte((statefulDevice.Water >> 8) & 0xFF))
	buf.WriteByte(byte((statefulDevice.Water >> 16) & 0xFF))

	buf.WriteByte(byte(statefulDevice.Electric & 0xFF))
	buf.WriteByte(byte((statefulDevice.Electric >> 8) & 0xFF))
	buf.WriteByte(byte((statefulDevice.Electric >> 16) & 0xFF))

	dataLen := byte(buf.Len() - 3)
	data := buf.Bytes()
	data[1] = dataLen
	buf.WriteByte(crc8(data, buf.Len()))
	buf.WriteByte(0x16)

	_, err = statefulDevice.Conn.Write(buf.Bytes())
	if err != nil {
		log.Print(err)
		return
	}
	log.Printf("[%s]: 关井", statefulDevice.Device.Sn)

	_ = statefulDevice.Conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	recvBuffer := make([]byte, 512)
	n, err := statefulDevice.Conn.Read(recvBuffer)
	if err != nil {
		log.Println(err)
		return
	}

	dlt645RecvProcess(recvBuffer[:n], statefulDevice)
}
