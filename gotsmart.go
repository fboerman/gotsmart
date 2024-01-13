package main

import (
	"bufio"
	"flag"
	"fmt"
	dsmrprometheus "github.com/basvdlei/gotsmart/dsmr/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tarm/serial"
	"log"
	"net/http"
	"time"
)

const version = "0.1.0"

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

	var br *bufio.Reader
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
		conn, err := serial.OpenPort(c)
		br = bufio.NewReader(conn)
		if err != nil {
			log.Fatal(err)
		}
	} else if *inputFlag == "tcp" {
		if *tcpAddrFlag == "" {
			log.Fatal("please specify a tcp connection string in format ip:port")
		}
	} else {
		log.Fatal("Invalid input type setting")
	}

	collector := &dsmrprometheus.DSMRCollector{
		TcpFlag:      (*inputFlag == "tcp"),
		TcpAddr:      *tcpAddrFlag,
		SerialReader: br,
	}
	prom := prometheus.NewRegistry()
	prom.MustRegister(collector)
	promHttpHandler := promhttp.HandlerFor(prom, promhttp.HandlerOpts{})

	mux := http.NewServeMux()
	mux.Handle("/metrics", promHttpHandler)

	srv := &http.Server{
		Addr:         *addrFlag,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
