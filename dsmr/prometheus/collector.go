/*
Package prometheus implements a collector of DSMR metrics for Prometheus.
*/
package prometheus

import (
	"bufio"
	"fmt"
	"github.com/basvdlei/gotsmart/crc16"
	"github.com/basvdlei/gotsmart/dsmr"
	"github.com/prometheus/client_golang/prometheus"
	"log"
	"net"
	"strconv"
	"strings"
	"time"
)

// DSMRCollector implements the Prometheus Collector interface.
type DSMRCollector struct {
	metrics      []prometheus.Metric
	TcpFlag      bool
	TcpAddr      string
	SerialReader *bufio.Reader
}

// Collect implements part of the prometheus.Collector interface.
func (dc *DSMRCollector) Collect(ch chan<- prometheus.Metric) {
	dc.Process()
	for _, m := range dc.metrics {
		ch <- m
	}
}

// Describe implements part of the prometheus.Collector interface.
func (dc *DSMRCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, mb := range metricBuilders {
		ch <- mb.Desc
	}
}

// Update all the metrics to the values of the given frame.
func (dc *DSMRCollector) Update(f dsmr.Frame) {
	var metrics []prometheus.Metric
	for _, obj := range f.Objects {
		if mb, found := metricBuilders[obj.ID]; found {
			if !mb.CheckUnit(obj.Unit) {
				log.Printf("unit in object does not meet spec: %s\n", obj)
				continue
			}
			value, err := strconv.ParseFloat(obj.Value, 64)
			if err != nil {
				log.Printf("could not parse value to float64 for %s\n", obj)
				continue
			}
			//if obj.ID == "1-0:1.7.0" {
			//	log.Printf("%f", value)
			//}
			m, err := prometheus.NewConstMetric(
				mb.Desc,
				mb.ValueType,
				value,
				f.EquipmentID, f.Version, //labels
			)
			if err != nil {
				log.Printf("could not create prometheus metric for %s\n", obj)
				continue
			}
			metrics = append(metrics, m)
		} else {
			continue
		}
	}
	dc.metrics = metrics
}

func (dc *DSMRCollector) Process() {
	var br *bufio.Reader
	if !dc.TcpFlag {
		br = dc.SerialReader
	} else {
		conn, err := net.Dial("tcp", dc.TcpAddr)
		conn.SetReadDeadline(time.Now().Add(time.Second))
		defer conn.Close()
		if err != nil {
			log.Fatal(err)
		}
		br = bufio.NewReader(conn)
	}

	for {
		if b, err := br.Peek(1); err == nil {
			if string(b) != "/" {
				fmt.Printf("Ignoring garbage character: %c\n", b)
				br.ReadByte()
				continue
			}
		} else {
			return
		}
		frame, err := br.ReadBytes('!')
		if err != nil {
			//fmt.Printf("Error: %v\n", err)
			continue
		}

		bcrc, err := br.ReadBytes('\n')
		if err != nil {
			//fmt.Printf("Error: %v\n", err)
			continue
		}

		// Check CRC
		mcrc := strings.ToUpper(strings.TrimSpace(string(bcrc)))
		crc := fmt.Sprintf("%04X", crc16.Checksum(frame))
		if mcrc != crc {
			fmt.Printf("CRC mismatch: %q != %q\n", mcrc, crc)
			continue
		}

		dsmrFrame, err := dsmr.ParseFrame(string(frame))
		if err != nil {
			log.Printf("could not parse frame: %v\n", err)
			continue
		}

		dc.Update(dsmrFrame)
		break
	}
}
