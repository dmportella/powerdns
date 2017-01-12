package powerdns

//Based off of github.com/waynz0r/powerdns

import (
	"fmt"
	"log"
	"net/url"
	"net/http"
	"strings"
	"errors"
	"bytes"
	"time"
	"path"
	"io/ioutil"
	"encoding/json"
)

// Error strct
type Error struct {
	Message string `json:"error"`
}

// Error Returns
func (e Error) Error() string {
	return fmt.Sprintf("%v", e.Message)
}

// CombinedRecord strct
type CombinedRecord struct {
	Name    string
	Type    string
	TTL     int
	Records []string
}

// Zone struct
type Zone struct {
	
	
	Account        string `json:"account"`
	DNSsec         bool   `json:"dnssec"`
	ID             string `json:"id"`
	Kind        string `json:"kind"`
	LastCheck      int    `json:"last_check"`
//missing masters
	Name           string `json:"name"`
	Type           string `json:"type"`
	
	NotifiedSerial int64    `json:"notified_serial"`
	
	Records        []struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		TTL      int    `json:"ttl"`
		Records    []Record `json:"records"`
	} `json:"rrsets"`

	Serial         int64    `json:"serial"`
	SOAEdit        string `json:"soa_edit"`
	SOAEditApi     string `json:"soa_edit_api"`
	URL            string `json:"url"`
}

// Record struct
type Record struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	TTL      int    `json:"ttl"`
	Priority int    `json:"priority"`
	Disabled bool   `json:"disabled"`
	Content  string `json:"content"`
}

// RRset struct
type RRset struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	TTL        int      `json:"ttl"`
	ChangeType string   `json:"changetype"`
	Records    []Record `json:"records"`
}

// RRsets struct
type RRsets struct {
	Sets []RRset `json:"rrsets"`
}

// PowerDNS struct
type PowerDNS struct {
	scheme   string
	hostname string
	basePath string
	port     string
	vhost    string
	domain   string
	apikey   string
}

// New returns a new PowerDNS
func New(baseURL string, vhost string, domain string, apikey string) *PowerDNS {
	if vhost == "" {
		vhost = "localhost"
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		log.Fatalf("%s is not a valid url: %v", baseURL, err)
	}
	hp := strings.Split(u.Host, ":")
	hostname := hp[0]
	var port string
	if len(hp) > 1 {
		port = hp[1]
	} else {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	if u.Path == "" {
		u.Path = "/"
	}

	return &PowerDNS{
		scheme:   u.Scheme,
		hostname: hostname,
		basePath: u.Path,
		port:     port,
		vhost:    vhost,
		domain:   domain,
		apikey:   apikey,
	}
}

// AddRecord ...
func (p *PowerDNS) AddRecord(name string, recordType string, ttl int, content []string) (error) {

	err := p.ChangeRecord(name, recordType, ttl, content, "UPSERT")

	return err
}

// DeleteRecord ...
func (p *PowerDNS) DeleteRecord(name string, recordType string, ttl int, content []string) (error) {

	err := p.ChangeRecord(name, recordType, ttl, content, "DELETE")

	return err
}

// ChangeRecord ...
func (p *PowerDNS) ChangeRecord(name string, recordType string, ttl int, content []string, action string) (error) {

	Record := new(CombinedRecord)
	Record.Name = name
	Record.Type = recordType
	Record.TTL = ttl
	Record.Records = content

	err := p.patchRRset(*Record, action)

	return err
}

func fqdn(name string) string {
	n := len(name)
	if n == 0 || name[n-1] == '.' {
		return name
	}
	return name + "."
}

func (p *PowerDNS) patchRRset(record CombinedRecord, action string) (error) {
	Set := RRset{Name: fqdn(record.Name), Type: record.Type, ChangeType: "REPLACE", TTL: record.TTL}

	if action == "DELETE" {
		Set.ChangeType = "DELETE"
	}

	var R Record

	for _, rec := range record.Records {
		R = Record{Name: record.Name, Type: record.Type, TTL: record.TTL, Content: rec}
		Set.Records = append(Set.Records, R)
	}

	dataObject := RRsets{}
	dataObject.Sets = append(dataObject.Sets, Set)

	data, _ := json.Marshal(dataObject)

	_, err := p.request("PATCH", p.getUrl(), data)

	if err != nil {
		return fmt.Errorf("PowerDNS API call has failed: %v", err)
	}

	return err
}

func (p *PowerDNS) getUrl() string {

	u := new(url.URL)
	u.Host = p.hostname + ":" + p.port
	u.Scheme = p.scheme

	childPath := "/servers/" + p.vhost + "/zones/" + fqdn(p.domain)

	u.Path = path.Join(p.basePath, childPath)

	return u.String()
}



func (p *PowerDNS) request(method string, url string, b []byte) (response []byte, err error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(b))

	req.Header.Set("X-API-Key", p.apikey)
	req.Header.Set("content-type", "application/json; charset=utf-8")
	req.Header.Set("accept", "application/json; charset=utf-8")
	req.Header.Set("user-agent", "PowerDNS-Integration Plugin")

	httpClient := &http.Client{Timeout: (120 * time.Second)}

	res, err := httpClient.Do(req)

	if err != nil {
		err = errors.New("Http request returned an error")
		return
	}

	defer res.Body.Close()

	response, err = ioutil.ReadAll(res.Body)

	if err != nil {
		err = errors.New("Error while reading body")
		return
	}

	return
}

// GetRecords ...
func (p *PowerDNS) GetRecords() ([]Record, error) {

	var records []Record

	zone := new(Zone)

	data, err := p.request("GET", p.getUrl(), nil)

	if err != nil {
		return records, fmt.Errorf("PowerDNS API call has failed: %v", err)
	}

	err = json.Unmarshal(data, &zone)

	if err != nil {
		return records, fmt.Errorf("PowerDNS API call has failed: %v", err)
	}
	
	for _, rec := range zone.Records {
		record := Record{Name: rec.Name, Type: rec.Type, TTL: rec.TTL, Priority: rec.Records[0].Priority, Disabled: rec.Records[0].Disabled, Content: rec.Records[0].Content}
		records = append(records, record)
	}

	return records, err
}

// GetCombinedRecords ...
func (p *PowerDNS) GetCombinedRecords() ([]CombinedRecord, error) {
	var records []CombinedRecord
	var uniqueRecords []CombinedRecord

	//- Plain records from the zone
	Records, err := p.GetRecords()

	if err != nil {
		return records, err
	}

	//- Iterate through records to combine them by name and type
	for _, rec := range Records {
		record := CombinedRecord{Name: rec.Name, Type: rec.Type, TTL: rec.TTL}
		found := false
		for _, uRec := range uniqueRecords {
			if uRec.Name == rec.Name && uRec.Type == rec.Type {
				found = true
				continue
			}
		}

		//- append them only if missing
		if found == false {
			uniqueRecords = append(uniqueRecords, record)
		}
	}

	//- Get all values from the unique records
	for _, uRec := range uniqueRecords {
		for _, rec := range Records {
			if uRec.Name == rec.Name && uRec.Type == rec.Type {
				uRec.Records = append(uRec.Records, rec.Content)
			}
		}
		records = append(records, uRec)
	}

	return records, nil
}

func init() {

}
