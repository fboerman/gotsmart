package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/basvdlei/gotsmart/crc16"
	"github.com/basvdlei/gotsmart/dsmr"
	dsmrprometheus "github.com/basvdlei/gotsmart/dsmr/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tarm/serial"
)

const version = "0.0.3"

type frameupdate struct {
	mutex sync.Mutex
	Frame string
	Time  time.Time
}

func (f *frameupdate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.Header().Add("Last-Modified", f.Time.Format(http.TimeFormat))
	w.Write([]byte(f.Frame))
}

func (f *frameupdate) Update(frame string) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.Frame = strings.Replace(frame, "\r", "", -1)
	f.Time = time.Now()
}

func (f *frameupdate) Process(br *bufio.Reader, collector *dsmrprometheus.DSMRCollector) {
	for {
		if b, err := br.Peek(1); err == nil {
			if string(b) != "/" {
				fmt.Printf("Ignoring garbage character: %c\n", b)
				br.ReadByte()
				continue
			}
		} else {
			continue
		}
		frame, err := br.ReadBytes('!')
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		bcrc, err := br.ReadBytes('\n')
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		// Check CRC
		mcrc := strings.ToUpper(strings.TrimSpace(string(bcrc)))
		crc := fmt.Sprintf("%04X", crc16.Checksum(frame))
		if mcrc != crc {
			fmt.Printf("CRC mismatch: %q != %q\n", mcrc, crc)
			continue
		}
		f.Update(string(frame))
		dsmrFrame, err := dsmr.ParseFrame(string(frame))
		if err != nil {
			log.Printf("could not parse frame: %v\n", err)
			continue
		}
		collector.Update(dsmrFrame)
	}
}

func main() {
	var (
		addrFlag   = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
		deviceFlag = flag.String("device", "/dev/ttyAMA0", "Serial device to read P1 data from.")
		baudFlag   = flag.Int("baud", 115200, "Baud rate (speed) to use.")
		bitsFlag   = flag.Int("bits", 8, "Number of databits.")
		parityFlag = flag.String("parity", "none", "Parity the use (none/odd/even/mark/space).")

		inputFlag   = flag.String("input-type", "serial", "Which input type to use (serial/tcp)")
		tcpAddrFlag = flag.String("tcp-addr", "", "The address to connect to for tcp input")
	)
	flag.Parse()

	fmt.Printf("GotSmart (%s)\n", version)

	var conn io.Reader
	var err error
	if *inputFlag == "serial" {
		var parity serial.Parity
		switch *parityFlag {
		case "none":
			parity = serial.ParityNone
		case "odd":
			parity = serial.ParityOdd
		case "even":
			parity = serial.ParityEven
		case "mark":
			parity = serial.ParityMark
		case "space":
			parity = serial.ParitySpace
		default:
			log.Fatal("Invalid parity setting")
		}

		c := &serial.Config{
			Name:   *deviceFlag,
			Baud:   *baudFlag,
			Size:   byte(*bitsFlag),
			Parity: parity,
		}
		conn, err = serial.OpenPort(c)
	} else if *inputFlag == "tcp" {
		if *tcpAddrFlag == "" {
			log.Fatal("please specify a tcp connection string in format ip:port")
		}
		conn, err = net.Dial("tcp", *tcpAddrFlag)
	} else {
		log.Fatal("Invalid input type setting")
	}

	if err != nil {
		log.Fatal(err)
	}

	br := bufio.NewReader(conn)
	collector := &dsmrprometheus.DSMRCollector{}
	prom := prometheus.NewRegistry()
	prom.MustRegister(collector)
	promHttpHandler := promhttp.HandlerFor(prom, promhttp.HandlerOpts{})
	f := &frameupdate{mutex: sync.Mutex{}}
	go f.Process(br, collector)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promHttpHandler)
	mux.Handle("/", f)
	srv := &http.Server{
		Addr:         *addrFlag,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
