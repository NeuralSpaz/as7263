package as7263

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/NeuralSpaz/i2cmux"
	// "github.com/NeuralSpaz/i2cmux"
)

type AS7263 struct {
	dev *i2cmux.Device
	// dev   *i2c.Device
	debug bool
}

func (a *AS7263) Close() error {
	a.LEDoff()
	return a.dev.Close()
}

// Spectrum (610nm, 680nm, 730nm, 760nm, 810nm, 860nm)
type Spectrum struct {
	Counts            []Count   `json:"count"`
	Time              time.Time `json:"time"`
	SensorTemperature byte      `json:"sensorTemperature"`
	Unixnano          int64     `json:"unixnano"`
}

type Count struct {
	Wavelength float64 `json:"wavelength"`
	Value      float64 `json:"value"`
	Raw        uint16  `json:"raw"`
}

func NewSensor(mux i2cmux.Multiplexer, port uint8, opts ...func(*AS7263) error) (*AS7263, error) {
	a := new(AS7263)

	for _, option := range opts {
		option(a)
	}
	var err error
	a.dev, err = i2cmux.Open(0x49, mux, port)
	if err != nil {
		log.Panic(err)
	}
	a.setConfig()
	return a, nil
}

func (a *AS7263) virtualRegisterWrite(register, data byte) error {

	const (
		SlaveStatusRegister byte = 0x00
		SlaveWriteRegister  byte = 0x01
		SlaveReadRegister   byte = 0x02
	)
	for {

		rx := make([]byte, 1)
		if err := a.dev.ReadReg(SlaveStatusRegister, rx); err != nil {
			log.Fatalln(err)
		}

		if rx[0]&0x03 == 0x00 {
			break
		}
		if rx[0]&0x03 == 0x01 {
			discard := make([]byte, 1)
			if err := a.dev.ReadReg(SlaveStatusRegister, discard); err != nil {
				log.Fatalln(err)
			}
			// if a.debug {
			// log.Printf("DataLeftInReadBuffer: %02x\n", discard[0])
			// }
		}
		time.Sleep(time.Millisecond)
	}

	if err := a.dev.WriteReg(SlaveWriteRegister, []byte{register | 0x80}); err != nil {
		log.Fatalln(err)
	}

	for {

		rx := make([]byte, 1)
		if err := a.dev.ReadReg(SlaveStatusRegister, rx); err != nil {
			log.Fatalln(err)
		}

		if rx[0]&0x03 == 0x00 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	if err := a.dev.WriteReg(SlaveWriteRegister, []byte{data}); err != nil {
		log.Fatalln(err)
	}

	return nil
}

func (a *AS7263) virtualRegisterRead(register byte) (byte, error) {

	const (
		SlaveStatusRegister byte = 0x00
		SlaveWriteRegister  byte = 0x01
		SlaveReadRegister   byte = 0x02
	)
	for {

		rx := make([]byte, 1)
		if err := a.dev.ReadReg(SlaveStatusRegister, rx); err != nil {
			log.Fatalln(err)
		}

		if rx[0]&0x03 == 0x00 {
			break
		}
		// if there is data pending read it but thats all
		if rx[0]&0x03 == 0x01 {
			discard := make([]byte, 1)
			if err := a.dev.ReadReg(SlaveStatusRegister, discard); err != nil {
				log.Fatalln(err)
			}

		}
		time.Sleep(time.Millisecond)
	}

	if err := a.dev.WriteReg(SlaveWriteRegister, []byte{register}); err != nil {
		log.Fatalln(err)
	}

	for {

		rx := make([]byte, 1)
		if err := a.dev.ReadReg(SlaveStatusRegister, rx); err != nil {
			log.Fatalln(err)
		}

		if rx[0]&0x03 == 0x01 {
			break
		}

		time.Sleep(time.Millisecond)
	}

	data := make([]byte, 1)
	if err := a.dev.ReadReg(SlaveReadRegister, data); err != nil {
		log.Fatalln(err)
	}

	return data[0], nil
}

func (a *AS7263) setConfig() error {
	if a.debug {
		fmt.Println("setConfig")
	}
	// if err := a.virtualRegisterWrite(0x04, 0xE0); err != nil {
	// 	return err
	// }
	// time.Sleep(time.Millisecond0)
	if err := a.virtualRegisterWrite(0x04, 0x3C); err != nil {
		return err
	}
	// if err := a.virtualRegisterWrite(0x06, 0xFF); err != nil {
	// 	return err
	// }
	// LED OFF
	err := a.LEDoff()
	return err
}

func (a *AS7263) LEDoff() error {
	// fmt.Println("ledoff")
	err := a.virtualRegisterWrite(0x07, 0x00)
	return err
}

func (a *AS7263) LEDon() error {
	// fmt.Println("ledon")
	err := a.virtualRegisterWrite(0x07, 0x09)
	return err
}

func (a *AS7263) setMode(mode uint8) error {
	fmt.Println("setmode")

	if mode > 3 {
		mode = 3
	}

	control, err := a.virtualRegisterRead(0x04)
	if err != nil {
		return err
	}
	control &= 0xf3
	control |= (mode << 2)
	if err := a.virtualRegisterWrite(0x04, control); err != nil {
		return err
	}
	return nil
}

func (a *AS7263) dataReady() (bool, error) {
	// fmt.Println("dataReady?")
	var control byte
	err := retry(100, time.Millisecond, func() (err error) {
		control, err = a.virtualRegisterRead(0x04)
		return
	})

	if err != nil {
		log.Println(err)
		return false, err
	}

	ready := hasBit(control, 1)

	return ready, err

}

func retry(attempts int, sleep time.Duration, fn func() error) (err error) {
	for i := 0; ; i++ {
		err = fn()
		if err == nil {
			return
		}

		if i >= (attempts - 1) {
			break
		}

		time.Sleep(sleep)

		log.Println("retrying after error:", err)
	}
	return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

func (a *AS7263) Request() time.Duration {
	if err := a.setConfig(); err != nil {
		log.Println(err)
	}

	if err := a.LEDon(); err != nil {
		log.Println(err)
	}

	if err := a.setMode(3); err != nil {
		log.Println(err)
	}
	return time.Millisecond * 1450
}

func (a *AS7263) ReadAll() (Spectrum, error) {
	fmt.Println("readall")

	ready, err := a.dataReady()
	if err != nil {
		log.Println(err)
	}
	for !ready {
		// time.Sleep(time.Millisecond *10 * 50)
		ready, err = a.dataReady()
		if err != nil {
			log.Println(err)
		}
	}
	now := time.Now()
	rh, err := a.virtualRegisterRead(0x08)
	if err != nil {
		return Spectrum{}, err
	}
	rl, err := a.virtualRegisterRead(0x09)
	if err != nil {
		return Spectrum{}, err
	}
	sh, err := a.virtualRegisterRead(0x0a)
	if err != nil {
		return Spectrum{}, err
	}
	sl, err := a.virtualRegisterRead(0x0b)
	if err != nil {
		return Spectrum{}, err
	}
	th, err := a.virtualRegisterRead(0x0c)
	if err != nil {
		return Spectrum{}, err
	}
	tl, err := a.virtualRegisterRead(0x0d)
	if err != nil {
		return Spectrum{}, err
	}
	uh, err := a.virtualRegisterRead(0x0e)
	if err != nil {
		return Spectrum{}, err
	}
	ul, err := a.virtualRegisterRead(0x0f)
	if err != nil {
		return Spectrum{}, err
	}
	vh, err := a.virtualRegisterRead(0x10)
	if err != nil {
		return Spectrum{}, err
	}
	vl, err := a.virtualRegisterRead(0x11)
	if err != nil {
		return Spectrum{}, err
	}
	wh, err := a.virtualRegisterRead(0x12)
	if err != nil {
		return Spectrum{}, err
	}
	wl, err := a.virtualRegisterRead(0x13)
	if err != nil {
		return Spectrum{}, err
	}

	r := binary.BigEndian.Uint16([]byte{rh, rl})
	s := binary.BigEndian.Uint16([]byte{sh, sl})
	t := binary.BigEndian.Uint16([]byte{th, tl})
	u := binary.BigEndian.Uint16([]byte{uh, ul})
	v := binary.BigEndian.Uint16([]byte{vh, vl})
	w := binary.BigEndian.Uint16([]byte{wh, wl})

	// GET Calibrated Float32

	rcal0, err := a.virtualRegisterRead(0x14)
	if err != nil {
		return Spectrum{}, err
	}
	rcal1, err := a.virtualRegisterRead(0x15)
	if err != nil {
		return Spectrum{}, err
	}
	rcal2, err := a.virtualRegisterRead(0x16)
	if err != nil {
		return Spectrum{}, err
	}
	rcal3, err := a.virtualRegisterRead(0x17)
	if err != nil {
		return Spectrum{}, err
	}
	rcal32 := binary.BigEndian.Uint32([]byte{rcal0, rcal1, rcal2, rcal3})
	rcal := math.Float32frombits(rcal32)

	scal0, err := a.virtualRegisterRead(0x18)
	if err != nil {
		return Spectrum{}, err
	}
	scal1, err := a.virtualRegisterRead(0x19)
	if err != nil {
		return Spectrum{}, err
	}
	scal2, err := a.virtualRegisterRead(0x1A)
	if err != nil {
		return Spectrum{}, err
	}
	scal3, err := a.virtualRegisterRead(0x1B)
	if err != nil {
		return Spectrum{}, err
	}
	scal32 := binary.BigEndian.Uint32([]byte{scal0, scal1, scal2, scal3})
	scal := math.Float32frombits(scal32)

	tcal0, err := a.virtualRegisterRead(0x1C)
	if err != nil {
		return Spectrum{}, err
	}
	tcal1, err := a.virtualRegisterRead(0x1D)
	if err != nil {
		return Spectrum{}, err
	}
	tcal2, err := a.virtualRegisterRead(0x1E)
	if err != nil {
		return Spectrum{}, err
	}
	tcal3, err := a.virtualRegisterRead(0x1F)
	if err != nil {
		return Spectrum{}, err
	}
	tcal32 := binary.BigEndian.Uint32([]byte{tcal0, tcal1, tcal2, tcal3})
	tcal := math.Float32frombits(tcal32)

	ucal0, err := a.virtualRegisterRead(0x20)
	if err != nil {
		return Spectrum{}, err
	}
	ucal1, err := a.virtualRegisterRead(0x21)
	if err != nil {
		return Spectrum{}, err
	}
	ucal2, err := a.virtualRegisterRead(0x22)
	if err != nil {
		return Spectrum{}, err
	}
	ucal3, err := a.virtualRegisterRead(0x23)
	if err != nil {
		return Spectrum{}, err
	}
	ucal32 := binary.BigEndian.Uint32([]byte{ucal0, ucal1, ucal2, ucal3})
	ucal := math.Float32frombits(ucal32)

	vcal0, err := a.virtualRegisterRead(0x24)
	if err != nil {
		return Spectrum{}, err
	}
	vcal1, err := a.virtualRegisterRead(0x25)
	if err != nil {
		return Spectrum{}, err
	}
	vcal2, err := a.virtualRegisterRead(0x26)
	if err != nil {
		return Spectrum{}, err
	}
	vcal3, err := a.virtualRegisterRead(0x27)
	if err != nil {
		return Spectrum{}, err
	}
	vcal32 := binary.BigEndian.Uint32([]byte{vcal0, vcal1, vcal2, vcal3})
	vcal := math.Float32frombits(vcal32)

	wcal0, err := a.virtualRegisterRead(0x28)
	if err != nil {
		return Spectrum{}, err
	}
	wcal1, err := a.virtualRegisterRead(0x29)
	if err != nil {
		return Spectrum{}, err
	}
	wcal2, err := a.virtualRegisterRead(0x2A)
	if err != nil {
		return Spectrum{}, err
	}
	wcal3, err := a.virtualRegisterRead(0x2B)
	if err != nil {
		return Spectrum{}, err
	}
	wcal32 := binary.BigEndian.Uint32([]byte{wcal0, wcal1, wcal2, wcal3})
	wcal := math.Float32frombits(wcal32)

	tempraw, err := a.virtualRegisterRead(0x06)
	if err != nil {
		return Spectrum{}, err
	}
	// Spectrum (610nm, 680nm, 730nm, 760nm, 810nm, 860nm)

	return Spectrum{Time: now, Unixnano: now.UnixNano(), SensorTemperature: tempraw,
		Counts: []Count{
			{Wavelength: 610, Value: float64(rcal), Raw: r},
			{Wavelength: 680, Value: float64(scal), Raw: s},
			{Wavelength: 730, Value: float64(tcal), Raw: t},
			{Wavelength: 760, Value: float64(ucal), Raw: u},
			{Wavelength: 810, Value: float64(vcal), Raw: v},
			{Wavelength: 860, Value: float64(wcal), Raw: w},
		}}, nil

	// return Spectrum{r, s, t, u, v, w, rcal, scal, tcal, ucal, vcal, wcal}, nil
	// return Spectrum{}, nil
}

func clearBit(n byte, pos uint8) byte {
	mask := ^(1 << pos)
	n &= byte(mask)
	return n
}
func setBit(n byte, pos uint8) byte {
	n |= (1 << pos)
	return n
}
func hasBit(n byte, pos uint8) bool {
	val := n & (1 << pos)
	return (val > 0)
}
