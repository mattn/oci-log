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
	"github.com/oracle/oci-go-sdk/logging"
	"github.com/oracle/oci-go-sdk/loggingingestion"
)

var _ = common.MakeDefaultHTTPRequest
var _ = logging.ActionTypesCreated

func fatalIf(err error) {
	if err != nil {
		log.Fatalln(err.Error())
	}
}

func lineReader(wg *sync.WaitGroup, r io.Reader, ch chan string) {
	defer wg.Done()
	defer close(ch)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		ch <- scanner.Text()
	}
}

func makeBatch(lines []string, tee bool, asjson bool) loggingingestion.LogEntryBatch {
	now := time.Now()
	batch := loggingingestion.LogEntryBatch{
		Defaultlogentrytime: &common.SDKTime{Time: now},
		Entries:             []loggingingestion.LogEntry{},
	}
	for _, line := range lines {
		if tee {
			fmt.Println(line)
		}
		var entry loggingingestion.LogEntry
		if asjson {
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

func reader(wg *sync.WaitGroup, r io.Reader, ch chan loggingingestion.LogEntryBatch, tee bool, asjson bool, max int) {
	defer wg.Done()
	defer close(ch)

	wg.Add(1)

	lch := make(chan string, max)
	go lineReader(wg, r, lch)

	var lines []string

loop:
	for {
		select {
		case line, ok := <-lch:
			if !ok {
				if len(lines) > 0 {
					ch <- makeBatch(lines, tee, asjson)
					lines = nil
				}
				break loop
			}
			lines = append(lines, line)
		default:
			if len(lines) > 0 {
				ch <- makeBatch(lines, tee, asjson)
				lines = nil
			}
		}
	}
}

func main() {
	var logId string
	var source string
	var subject string
	var ltype string
	var tee bool
	var asjson bool
	var max int
	flag.StringVar(&source, "source", os.Getenv("OCI_LOG_SOURCE"), "OCI Log Source ($OCI_LOG_SOURCE)")
	flag.StringVar(&subject, "subject", os.Getenv("OCI_LOG_SUBJECT"), "OCI Log Subject ($OCI_LOG_SUBJECT)")
	flag.StringVar(&ltype, "type", os.Getenv("OCI_LOG_TYPE"), "OCI Log Type ($OCI_LOG_TYPE)")
	flag.StringVar(&logId, "logid", os.Getenv("OCI_LOG_ID"), "OCI Log ID ($OCI_LOG_ID)")
	flag.BoolVar(&tee, "tee", false, "Tee output")
	flag.BoolVar(&asjson, "asjson", false, "Parse as JSON")
	flag.IntVar(&max, "max", 100, "Buffer size")
	flag.Parse()

	if source == "" || subject == "" || ltype == "" || logId == "" {
		flag.Usage()
		os.Exit(2)
	}
	if max < 0 {
		max = 0
	}

	var wg sync.WaitGroup
	wg.Add(1)

	ch := make(chan loggingingestion.LogEntryBatch)
	go reader(&wg, os.Stdin, ch, tee, asjson, max)

	client, err := loggingingestion.NewLoggingClientWithConfigurationProvider(common.DefaultConfigProvider())
	fatalIf(err)

	for {
		batch, ok := <-ch
		if !ok {
			break
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
		fatalIf(err)
	}
	wg.Wait()
}
