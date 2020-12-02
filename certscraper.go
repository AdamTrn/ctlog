package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

var Wd sync.WaitGroup
var Wp sync.WaitGroup
var Wo sync.WaitGroup
var Wg sync.WaitGroup

var httpClient *http.Client

type CTBatchData struct {
	Url string
	StartIndex int64
	StopIndex int64
}

type CTEntry struct {
	LeafInput []byte `json:"leaf_input"`
	ExtraData []byte `json:"extra_data"`
}

type CTEntries struct {
	Entries []CTEntry `json:"entries"`
}

type CTEntriesError struct {
	ErrorMessage string `json:"error_message"`
	Success      bool   `json:"success"`
}

type CTHead struct {
	TreeSize          int64  `json:"tree_size"`
	Timestamp         int64  `json:"timestamp"`
	SHA256RootHash    string `json:"sha256_root_hash"`
	TreeHeadSignature string `json:"tree_head_signature"`
}

// Downloads the entries as JSON.
func downloadJSON(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return []byte{}, err
	}

	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return []byte{}, err
	}

	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, err
	}

	return content, err
}

// Downloads entries and returns them.
func DownloadEntries(url string) (CTEntries, error) {
	var entries CTEntries
	var entriesError CTEntriesError

	data, err := downloadJSON(url)
	if err != nil {
		return entries, err
	}

	if strings.Contains(string(data), "\"error_message\":") {
		err = json.Unmarshal(data, &entriesError)
		if err != nil {
			return entries, err
		}
		return entries, errors.New(entriesError.ErrorMessage)
	}

	err = json.Unmarshal(data, &entries)
	return entries, err
}

// Downloads the CT Head of the log.
func DownloadSTH(logurl string) (CTHead, error) {
	var sth CTHead
	url := fmt.Sprintf("%sct/v1/get-sth", logurl)
	data, err := downloadJSON(url)
	if err != nil {
		return sth, err
	}

	err = json.Unmarshal(data, &sth)
	return sth, err
}

// Updates head index.
func UpdateLogIndex(index int64, logurl string) {
	_, err := db.Exec("UPDATE CTLog SET lastIndex = ? WHERE url = ?", index, logurl)
	if err != nil {
		log.Printf("[-] Failed to update head index of log %s -> %s\n", logurl, err)
		return
	}
}

// Creates HTTP client
func CreateClient() {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	httpClient = &http.Client{Transport: tr}
}


// Download entries and send them to the parsers
// Download in the maximal batch sizes
func newDownloadEntries(start int64, end int64, logurl string, c_parse chan<- CTEntry) {
	defer Wd.Done()
	cur := start
	const RETRY_WAIT = 1
	for cur < end {
		url := fmt.Sprintf("%sct/v1/get-entries?start=%d&end=%d", logurl, cur, end)
		entries, err := DownloadEntries(url)

		attempts := 0
		for err != nil {
			time.Sleep(time.Duration(RETRY_WAIT * attempts) * time.Second)
			// Common error at the start of the scan, don't log it
			if err.Error() != "invalid character '<' looking for beginning of value" {
				log.Printf("[-] (%d) Failed to download entries for %s -> %s\n", attempts, url, err)
			}
			entries, err = DownloadEntries(url)
			attempts++
			if attempts >= 10 {
				log.Printf("[-] Failed to download entries for %s -> %s\n", url, err)
			}
		}

		cur += int64(len(entries.Entries))

		for i := range entries.Entries {
			c_parse <- entries.Entries[i]
		}

		time.Sleep(time.Duration(1) * time.Second)
	}
}

// Launch for each log, split the log into chunks, launch goroutine for each chunk
func distributeWork(oldHeadSize int64, newHeadSize int64, downloaderCount int, logurl string, c_parse chan<- CTEntry) {
	defer Wg.Done()

	chunkSize := (newHeadSize - oldHeadSize) / int64(downloaderCount)

	for start := oldHeadSize; start < newHeadSize; start += chunkSize {
		end := start + chunkSize - 1
		if end >= newHeadSize {
			end = newHeadSize
		}

		if start > end {
			return
		}

		go newDownloadEntries(start, end, logurl, c_parse)
		Wd.Add(1)
	}
}