package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/oracle/oci-go-sdk/common"
	"github.com/oracle/oci-go-sdk/loggingingestion"
)

type opt struct {
	max     int
	asjson  bool
	tee     bool
	verbose bool
}

func lineReader(wg *sync.WaitGroup, r io.Reader, ch chan string) {
	defer wg.Done()
	defer close(ch)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		ch <- scanner.Text()
	}
}

func makeBatch(lines []string, opt opt) loggingingestion.LogEntryBatch {
	now := time.Now()
	batch := loggingingestion.LogEntryBatch{
		Defaultlogentrytime: &common.SDKTime{Time: now},
		Entries:             []loggingingestion.LogEntry{},
	}
	for _, line := range lines {
		if opt.tee {
			fmt.Println(line)
		}
		var entry loggingingestion.LogEntry
		if opt.asjson {
			if err := json.NewDecoder(strings.NewReader(line)).Decode(&entry); err != nil {
				continue
			}
			if entry.Data == nil {
				continue
			}
			if entry.Id == nil {
				entry.Id = common.String(uuid.NewString())
			}
			if entry.Time == nil {
				entry.Time = &common.SDKTime{Time: now}
			}
		} else {
			entry.Data = common.String(line)
			entry.Id = common.String(uuid.NewString())
			entry.Time = &common.SDKTime{Time: now}
		}
		batch.Entries = append(batch.Entries, entry)
	}
	return batch
}

func reader(wg *sync.WaitGroup, r io.Reader, ch chan loggingingestion.LogEntryBatch, opt opt) {
	defer wg.Done()
	defer close(ch)

	wg.Add(1)

	lch := make(chan string, opt.max)
	go lineReader(wg, r, lch)

	var lines []string

loop:
	for {
		select {
		case line, ok := <-lch:
			if !ok {
				ch <- makeBatch(lines, opt)
				break loop
			}
			lines = append(lines, line)
		default:
			ch <- makeBatch(lines, opt)
			lines = nil
		}
	}
}

func main() {
	var logId string
	var source string
	var subject string
	var ltype string
	var opt opt
	flag.StringVar(&source, "source", os.Getenv("OCI_LOG_SOURCE"), "OCI Log Source ($OCI_LOG_SOURCE)")
	flag.StringVar(&subject, "subject", os.Getenv("OCI_LOG_SUBJECT"), "OCI Log Subject ($OCI_LOG_SUBJECT)")
	flag.StringVar(&ltype, "type", os.Getenv("OCI_LOG_TYPE"), "OCI Log Type ($OCI_LOG_TYPE)")
	flag.StringVar(&logId, "logid", os.Getenv("OCI_LOG_ID"), "OCI Log ID ($OCI_LOG_ID)")
	flag.BoolVar(&opt.tee, "tee", false, "Tee output")
	flag.BoolVar(&opt.asjson, "asjson", false, "Parse as JSON")
	flag.IntVar(&opt.max, "max", 100, "Buffer size")
	flag.BoolVar(&opt.verbose, "verbose", false, "Verbose output")
	flag.Parse()

	if source == "" || subject == "" || ltype == "" || logId == "" {
		flag.Usage()
		os.Exit(2)
	}
	if opt.max < 0 {
		opt.max = 0
	}

	client, err := loggingingestion.NewLoggingClientWithConfigurationProvider(common.DefaultConfigProvider())
	if err != nil {
		log.Fatalln(err.Error())
	}

	var wg sync.WaitGroup
	wg.Add(1)

	ch := make(chan loggingingestion.LogEntryBatch)
	go reader(&wg, os.Stdin, ch, opt)

	for {
		batch, ok := <-ch
		if !ok {
			break
		}
		if len(batch.Entries) == 0 {
			continue
		}
		batch.Source = common.String(source)
		batch.Subject = common.String(subject)
		batch.Type = common.String(ltype)
		req := loggingingestion.PutLogsRequest{
			LogId: common.String(logId),
			PutLogsDetails: loggingingestion.PutLogsDetails{
				LogEntryBatches: []loggingingestion.LogEntryBatch{batch},
				Specversion:     common.String("1.0"),
			},
			TimestampOpcAgentProcessing: &common.SDKTime{Time: time.Now().UTC()},
		}

		_, err := client.PutLogs(context.Background(), req)
		if err != nil {
			log.Println(err)
		} else if opt.verbose {
			log.Printf("Sent %v entry(s)", len(batch.Entries))
		}
	}
	wg.Wait()
}
