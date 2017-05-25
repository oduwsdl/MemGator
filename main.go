package main

import (
	"container/list"
	"encoding/json"
	"fmt"
	flag "github.com/oduwsdl/memgator/pkg/mflag"
	"github.com/oduwsdl/memgator/pkg/sse"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Name consts need explanation, TODO
const (
	Name    = "MemGator"
	Version = "1.0-rc7"
	Art     = `
   _____                  _______       __
  /     \  _____  _____  / _____/______/  |___________
 /  Y Y  \/  __ \/     \/  \  ___\__  \   _/ _ \_   _ \
/   | |   \  ___/  Y Y  \   \_\  \/ __ |  | |_| |  | \/
\__/___\__/\____\__|_|__/\_______/_____|__|\___/|__|
`
)

// Name consts need explanation, TODO
const (
	responseFormats = "link|json|cdxj"
	validDatetimes  = "YYYY[MM[DD[hh[mm[ss]]]]]"
)

var (
	logBenchmark *log.Logger
	logInfo      *log.Logger
	logError     *log.Logger
	logFatal     *log.Logger
	transport    http.Transport
	client       http.Client
	broker       *sse.Broker
	reverseProxy *httputil.ReverseProxy
	baseURL      string
)

var format = flag.String([]string{"f", "-format"}, "Link", "Output format - Link/JSON/CDXJ")
var arcsloc = flag.String([]string{"a", "-arcs"}, "http://git.io/archives", "Local/remote JSON file path/URL for list of archives")
var logfile = flag.String([]string{"l", "-log"}, "", "Log file location - defaults to STDERR")
var benchmark = flag.String([]string{"b", "-benchmark"}, "", "Benchmark file location - Defaults to Logfile")
var contact = flag.String([]string{"c", "-contact"}, "@WebSciDL", "Email/URL/Twitter handle - used in the user-agent")
var agent = flag.String([]string{"A", "-agent"}, fmt.Sprintf("%s:%s <{CONTACT}>", Name, Version), "User-agent string sent to archives")
var host = flag.String([]string{"H", "-host"}, "localhost", "Host name - only used in web service mode")
var proxy = flag.String([]string{"P", "-proxy"}, "http://{HOST}[:{PORT}]{ROOT}", "Proxy URL - defaults to host, port, and root")
var root = flag.String([]string{"R", "-root"}, "/", "Service root path prefix")
var static = flag.String([]string{"D", "-static"}, "", "Directory path to serve static assets from")
var port = flag.Int([]string{"p", "-port"}, 1208, "Port number - only used in web service mode")
var topk = flag.Int([]string{"k", "-topk"}, -1, "Aggregate only top k archives based on probability")
var tolerance = flag.Int([]string{"F", "-tolerance"}, -1, "Failure tolerance limit for each archive")
var verbose = flag.Bool([]string{"V", "-verbose"}, false, "Show Info and Profiling messages on STDERR")
var version = flag.Bool([]string{"v", "-version"}, false, "Show name and version")
var spoof = flag.Bool([]string{"S", "-spoof"}, false, "Spoof each request with a random user-agent")
var monitor = flag.Bool([]string{"m", "-monitor"}, false, "Benchmark monitoring via SSE")
var contimeout = flag.Duration([]string{"t", "-contimeout"}, time.Duration(5*time.Second), "Connection timeout for each archive")
var hdrtimeout = flag.Duration([]string{"T", "-hdrtimeout"}, time.Duration(30*time.Second), "Header timeout for each archive")
var restimeout = flag.Duration([]string{"r", "-restimeout"}, time.Duration(60*time.Second), "Response timeout for each archive")
var dormant = flag.Duration([]string{"d", "-dormant"}, time.Duration(15*time.Minute), "Dormant period after consecutive failures")

// Session struct needs explanation, TODO
type Session struct {
	Start time.Time
}

// Archive struct needs explanation, TODO
type Archive struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Timemap     string  `json:"timemap"`
	Timegate    string  `json:"timegate"`
	Probability float64 `json:"probability"`
	Ignore      bool    `json:"ignore"`
	Dormant     bool    `json:"-"`
	Failures    int     `json:"-"`
}

// Archives struct needs explanation, TODO
type Archives []Archive

func (a Archives) Len() int {
	return len(a)
}

func (a Archives) Less(i, j int) bool {
	return a[i].Probability > a[j].Probability
}

func (a Archives) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a *Archives) filterIgnored() {
	filterPos := 0
	for i := range *a {
		if v := (*a)[i]; !v.Ignore {
			(*a)[filterPos] = v
			filterPos++
		}
	}
	(*a) = (*a)[:filterPos:filterPos]
}

func (a *Archives) sanitize() {
	for i := range *a {
		if !strings.HasSuffix((*a)[i].Timemap, "/") {
			(*a)[i].Timemap += "/"
		}
		if !strings.HasSuffix((*a)[i].Timegate, "/") {
			(*a)[i].Timegate += "/"
		}
	}
}

var archives Archives

// Link struct needs explanation, TODO
type Link struct {
	Href     string
	Datetime string
	Timeobj  time.Time
	Timestr  string
	NavRels  []string
}

var mimeMap = map[string]string{
	"link": "application/link-format",
	"json": "application/json",
	"cdxj": "application/cdxj+ors",
}

var regs = map[string]*regexp.Regexp{
	"isprtcl": regexp.MustCompile(`^https?://`),
	"linkdlm": regexp.MustCompile(`\s*"?\s*,\s*<\s*`),
	"attrdlm": regexp.MustCompile(`\s*>?"?\s*;\s*`),
	"kvaldlm": regexp.MustCompile(`\s*=\s*"?\s*`),
	"memento": regexp.MustCompile(`\bmemento\b`),
	"memdttm": regexp.MustCompile(`/(\d{14})/`),
	"dttmstr": regexp.MustCompile(`^(\d{4})(\d{2})?(\d{2})?(\d{2})?(\d{2})?(\d{2})?$`),
	"tmappth": regexp.MustCompile(`^timemap/(link|json|cdxj)/.+`),
	"tgatpth": regexp.MustCompile(`^timegate/.+`),
	"descpth": regexp.MustCompile(`^(memento|api)/(link|json|cdxj|proxy)/(\d{4})(\d{2})?(\d{2})?(\d{2})?(\d{2})?(\d{2})?/.+`),
	"rdrcpth": regexp.MustCompile(`^memento/(\d{4})(\d{2})?(\d{2})?(\d{2})?(\d{2})?(\d{2})?/.+`),
}

var spoofAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/49.0.2623.112 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_11_4) AppleWebKit/601.5.17 (KHTML, like Gecko) Version/9.1 Safari/601.5.17",
	"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:45.0) Gecko/20100101 Firefox/45.0",
}

func readArchives() (body []byte, err error) {
	if !regs["isprtcl"].MatchString(*arcsloc) {
		body, err = ioutil.ReadFile(*arcsloc)
		return
	}
	res, err := http.Get(*arcsloc)
	if err != nil {
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		err = fmt.Errorf(res.Status)
		return
	}
	body, err = ioutil.ReadAll(res.Body)
	return
}

func splitLinks(lnkrcvd chan string, lnksplt chan string) {
	defer close(lnksplt)
	lnkstr := <-lnkrcvd
	strlen := len(lnkstr)
	q := false
	u := false
	i := 0
	j := 0
	for ; j < strlen; j++ {
		switch lnkstr[j] {
		case '"':
			q = !q
		case '<':
			u = true
		case '>':
			u = false
		case ',':
			if !q && !u {
				lnksplt <- lnkstr[i:j]
				i = j + 1
			}
		}
	}
	if i < j {
		lnksplt <- lnkstr[i:j]
	}
}

func extractMementos(lnksplt chan string) (tml *list.List) {
	tml = list.New()
	for lnk := range lnksplt {
		lnk = strings.Trim(lnk, "<\" \t\n\r")
		parts := regs["attrdlm"].Split(lnk, -1)
		linkmap := map[string]string{"href": parts[0]}
		for _, attr := range parts[1:] {
			kv := regs["kvaldlm"].Split(attr, 2)
			if len(kv) > 1 {
				linkmap[kv[0]] = kv[1]
			}
		}
		dtm, ok := linkmap["datetime"]
		if !ok {
			continue
		}
		rel, ok := linkmap["rel"]
		if !ok {
			continue
		}
		if !regs["memento"].MatchString(rel) {
			continue
		}
		pdtm, err := time.Parse(http.TimeFormat, dtm)
		if err != nil {
			logError.Printf("Error parsing datetime (%s): %v", dtm, err)
			continue
		}
		link := Link{
			Href:     linkmap["href"],
			Datetime: dtm,
			Timeobj:  pdtm,
			Timestr:  pdtm.Format("20060102150405"),
		}
		e := tml.Back()
		for ; e != nil; e = e.Prev() {
			if link.Timestr > e.Value.(Link).Timestr {
				tml.InsertAfter(link, e)
				break
			}
		}
		if e == nil {
			tml.PushFront(link)
		}
	}
	return
}

func fetchTimemap(urir string, arch *Archive, tmCh chan *list.List, wg *sync.WaitGroup, dttmp *time.Time, sess *Session) {
	start := time.Now()
	defer wg.Done()
	url := arch.Timemap + urir
	if dttmp != nil {
		url = arch.Timegate + urir
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		benchmarker(arch.ID, "timemapfetch", fmt.Sprintf("Request error in %s", arch.Name), start, sess)
		logError.Printf("%s => Request error: %v", arch.ID, err)
		return
	}
	if *spoof {
		rand.Seed(time.Now().Unix())
		req.Header.Add("User-Agent", spoofAgents[rand.Intn(len(spoofAgents))])
	} else {
		req.Header.Add("User-Agent", *agent)
	}
	var res *http.Response
	if dttmp == nil {
		res, err = client.Do(req)
	} else {
		req.Header.Add("Accept-Datetime", dttmp.Format(http.TimeFormat))
		res, err = transport.RoundTrip(req)
	}
	if err != nil {
		benchmarker(arch.ID, "timemapfetch", fmt.Sprintf("Network error in %s", arch.Name), start, sess)
		logError.Printf("%s => Network error: %v", arch.ID, err)
		arch.Failures++
		if arch.Failures == *tolerance {
			arch.Dormant = true
			logInfo.Printf("%s => Dormant after %d consecutive failures", arch.ID, arch.Failures)
			go func(arch *Archive) {
				time.Sleep(*dormant)
				arch.Dormant = false
				arch.Failures = 0
				logInfo.Printf("%s => Awake after %s", arch.ID, *dormant)
			}(arch)
		}
		return
	}
	arch.Failures = 0
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusFound {
		benchmarker(arch.ID, "timemapfetch", fmt.Sprintf("Response error in %s, Status: %d", arch.Name, res.StatusCode), start, sess)
		logInfo.Printf("%s => Response error: %s", arch.ID, res.Status)
		return
	}
	lnks := res.Header.Get("Link")
	if dttmp == nil {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			benchmarker(arch.ID, "timemapfetch", fmt.Sprintf("Response read error in %s", arch.Name), start, sess)
			logError.Printf("%s => Response read error: %v", arch.ID, err)
			return
		}
		lnks = string(body)
	}
	benchmarker(arch.ID, "timemapfetch", fmt.Sprintf("TimeMap fethched from %s", arch.Name), start, sess)
	start = time.Now()
	lnkrcvd := make(chan string, 1)
	lnksplt := make(chan string, 128)
	lnkrcvd <- lnks
	go splitLinks(lnkrcvd, lnksplt)
	tml := extractMementos(lnksplt)
	tmCh <- tml
	benchmarker(arch.ID, "extractmementos", fmt.Sprintf("%d Mementos extracted from %s", tml.Len(), arch.Name), start, sess)
	logInfo.Printf("%s => Success: %d mementos", arch.ID, tml.Len())
}

func serializeLinks(urir string, basetm *list.List, format string, dataCh chan string, navonly bool, sess *Session) {
	start := time.Now()
	defer benchmarker("AGGREGATOR", "serialize", fmt.Sprintf("%d mementos serialized", basetm.Len()), start, sess)
	defer close(dataCh)
	switch strings.ToLower(format) {
	case "link":
		dataCh <- fmt.Sprintf(`<%s>; rel="original",`+"\n", urir)
		if !navonly {
			dataCh <- fmt.Sprintf(`<%s/timemap/link/%s>; rel="self"; type="application/link-format",`+"\n", *proxy, urir)
		}
		for e := basetm.Front(); e != nil; e = e.Next() {
			lnk := e.Value.(Link)
			if navonly && lnk.NavRels == nil {
				continue
			}
			rels := "memento"
			if lnk.NavRels != nil {
				rels = strings.Join(lnk.NavRels, " ") + " " + rels
				rels = strings.Replace(rels, "closest ", "", -1)
			}
			dataCh <- fmt.Sprintf(`<%s>; rel="%s"; datetime="%s",`+"\n", lnk.Href, rels, lnk.Datetime)
		}
		dataCh <- fmt.Sprintf(`<%s/timemap/link/%s>; rel="timemap"; type="application/link-format",`+"\n", *proxy, urir)
		dataCh <- fmt.Sprintf(`<%s/timemap/json/%s>; rel="timemap"; type="application/json",`+"\n", *proxy, urir)
		dataCh <- fmt.Sprintf(`<%s/timemap/cdxj/%s>; rel="timemap"; type="application/cdxj+ors",`+"\n", *proxy, urir)
		dataCh <- fmt.Sprintf(`<%s/timegate/%s>; rel="timegate"`+"\n", *proxy, urir)
	case "json":
		dataCh <- fmt.Sprintf("{\n"+`  "original_uri": "%s",`+"\n", urir)
		if !navonly {
			dataCh <- fmt.Sprintf(`  "self": "%s/timemap/json/%s",`+"\n", *proxy, urir)
		}
		dataCh <- fmt.Sprintf(`  "mementos": {` + "\n")
		if !navonly {
			dataCh <- `    "list": [` + "\n"
		}
		navs := ""
		for e := basetm.Front(); e != nil; e = e.Next() {
			lnk := e.Value.(Link)
			if navonly && lnk.NavRels == nil {
				continue
			}
			if lnk.NavRels != nil {
				for _, rl := range lnk.NavRels {
					navs += fmt.Sprintf(`    "%s": {`+"\n"+`      "datetime": "%s",`+"\n"+`      "uri": "%s"`+"\n    },\n", rl, lnk.Timeobj.Format(time.RFC3339), lnk.Href)
				}
			}
			if !navonly {
				dataCh <- fmt.Sprintf(`      {`+"\n"+`        "datetime": "%s",`+"\n"+`        "uri": "%s"`+"\n      }", lnk.Timeobj.Format(time.RFC3339), lnk.Href)
				if e.Next() != nil {
					dataCh <- ",\n"
				}
			}
		}
		if !navonly {
			dataCh <- "\n    ],\n"
		}
		dataCh <- strings.TrimRight(navs, ",\n")
		dataCh <- fmt.Sprintf("\n  },\n" + `  "timemap_uri": {` + "\n")
		dataCh <- fmt.Sprintf(`    "link_format": "%s/timemap/link/%s",`+"\n", *proxy, urir)
		dataCh <- fmt.Sprintf(`    "json_format": "%s/timemap/json/%s",`+"\n", *proxy, urir)
		dataCh <- fmt.Sprintf(`    "cdxj_format": "%s/timemap/cdxj/%s"`+"\n  },\n", *proxy, urir)
		dataCh <- fmt.Sprintf(`  "timegate_uri": "%s/timegate/%s"`+"\n}\n", *proxy, urir)
	case "cdxj":
		dataCh <- fmt.Sprintf(`!context ["http://tools.ietf.org/html/rfc7089"]` + "\n")
		if !navonly {
			dataCh <- fmt.Sprintf(`!id {"uri": "%s/timemap/cdxj/%s"}`+"\n", *proxy, urir)
		}
		dataCh <- fmt.Sprintf(`!keys ["memento_datetime_YYYYMMDDhhmmss"]` + "\n")
		dataCh <- fmt.Sprintf(`!meta {"original_uri": "%s"}`+"\n", urir)
		dataCh <- fmt.Sprintf(`!meta {"timegate_uri": "%s/timegate/%s"}`+"\n", *proxy, urir)
		dataCh <- fmt.Sprintf(`!meta {"timemap_uri": {"link_format": "%s/timemap/link/%s", "json_format": "%s/timemap/json/%s", "cdxj_format": "%s/timemap/cdxj/%s"}}`+"\n", *proxy, urir, *proxy, urir, *proxy, urir)
		for e := basetm.Front(); e != nil; e = e.Next() {
			lnk := e.Value.(Link)
			if navonly && lnk.NavRels == nil {
				continue
			}
			rels := "memento"
			if lnk.NavRels != nil {
				rels = strings.Join(lnk.NavRels, " ") + " " + rels
			}
			dataCh <- fmt.Sprintf(`%s {"uri": "%s", "rel": "%s", "datetime": "%s"}`+"\n", lnk.Timestr, lnk.Href, rels, lnk.Datetime)
		}
	default:
		dataCh <- fmt.Sprintf("Unrecognized format: %s\n", format)
	}
}

func aggregateTimemap(urir string, dttmp *time.Time, sess *Session) (basetm *list.List) {
	var wg sync.WaitGroup
	tmCh := make(chan *list.List, len(archives))
	for i, arch := range archives {
		if i == *topk {
			break
		}
		if arch.Dormant {
			continue
		}
		wg.Add(1)
		go fetchTimemap(urir, &archives[i], tmCh, &wg, dttmp, sess)
	}
	go func() {
		wg.Wait()
		close(tmCh)
	}()
	basetm = list.New()
	for newtm := range tmCh {
		start := time.Now()
		if newtm.Len() == 0 {
			continue
		}
		if basetm.Len() == 0 {
			basetm = newtm
			continue
		}
		if newtm.Len() > basetm.Len() {
			newtm, basetm = basetm, newtm
		}
		m := basetm.Back()
		e := newtm.Back()
		for e != nil {
			if m != nil {
				if e.Value.(Link).Timestr > m.Value.(Link).Timestr {
					basetm.InsertAfter(e.Value, m)
					e = e.Prev()
				} else {
					m = m.Prev()
				}
			} else {
				for e != nil {
					basetm.PushFront(e.Value)
					e = e.Prev()
				}
			}
		}
		benchmarker("AGGREGATOR", "aggregate", fmt.Sprintf("%d Mementos accumulated and sorted", basetm.Len()), start, sess)
	}
	return
}

func parseURI(uri string) (urir string, err error) {
	if !regs["isprtcl"].MatchString(uri) {
		uri = "http://" + uri
	}
	u, err := url.Parse(uri)
	if err == nil {
		urir = u.String()
	}
	return
}

func paddedTime(dttmstr string) (dttm *time.Time, err error) {
	m := regs["dttmstr"].FindStringSubmatch(dttmstr)
	dts := m[1]
	dts += (m[2] + "01")[:2]
	dts += (m[3] + "01")[:2]
	dts += (m[4] + "00")[:2]
	dts += (m[5] + "00")[:2]
	dts += (m[6] + "00")[:2]
	var dtm time.Time
	dtm, err = time.Parse("20060102150405", dts)
	dttm = &dtm
	return
}

func benchmarker(origin string, role string, info string, start time.Time, sess *Session) {
	end := time.Now()
	begin := sess.Start.UnixNano()
	info += fmt.Sprintf(" - Duration: %v", end.Sub(start))
	logBenchmark.Printf(`%d {"origin": "%s", "role": "%s", "info": "%s", "start": %d, "end": %d}`, begin, origin, role, info, start.UnixNano(), end.UnixNano())
	if *monitor {
		event := fmt.Sprintf(`{"session": "%d", "origin": "%s", "role": "%s", "info": "%s", "start": %d, "end": %d}`, begin, origin, role, info, start.UnixNano(), end.UnixNano())
		broker.Notifier <- []byte(event)
	}
}

func setNavRels(basetm *list.List, dttmp *time.Time, sess *Session) (navonly bool, closest string) {
	start := time.Now()
	defer benchmarker("AGGREGATOR", "setnav", "Navigational relations annotated", start, sess)
	itm := basetm.Front().Value.(Link)
	itm.NavRels = append(itm.NavRels, "first")
	basetm.Front().Value = itm
	itm = basetm.Back().Value.(Link)
	itm.NavRels = append(itm.NavRels, "last")
	basetm.Back().Value = itm
	closelm := basetm.Front()
	if dttmp != nil {
		navonly = true
		dttm := *dttmp
		etm := closelm.Value.(Link).Timeobj
		dur := etm.Sub(dttm)
		if dttm.After(etm) {
			dur = dttm.Sub(etm)
		}
		mindur := dur
		for e := closelm.Next(); e != nil; e = e.Next() {
			etm = e.Value.(Link).Timeobj
			dur = etm.Sub(dttm)
			if dttm.After(etm) {
				dur = dttm.Sub(etm)
			}
			if dur > mindur {
				break
			}
			mindur = dur
			closelm = e
		}
		itm = closelm.Value.(Link)
		closest = itm.Href
		itm.NavRels = append(itm.NavRels, "closest")
		closelm.Value = itm
		if closelm.Next() != nil {
			itm = closelm.Next().Value.(Link)
			itm.NavRels = append(itm.NavRels, "next")
			closelm.Next().Value = itm
		}
		if closelm.Prev() != nil {
			itm = closelm.Prev().Value.(Link)
			itm.NavRels = append(itm.NavRels, "prev")
			closelm.Prev().Value = itm
		}
	}
	return
}

func memgatorCli(urir string, format string, dttmp *time.Time) {
	start := time.Now()
	sess := new(Session)
	sess.Start = start
	upsession := "timemap"
	if dttmp != nil {
		upsession = "timegate"
	}
	defer benchmarker("SESSION", upsession, "Complete session", start, sess)
	benchmarker("AGGREGATOR", "createsess", "Session created", start, sess)
	logInfo.Printf("Aggregating Mementos for %s", urir)
	basetm := aggregateTimemap(urir, dttmp, sess)
	if basetm.Len() == 0 {
		return
	}
	navonly, _ := setNavRels(basetm, dttmp, sess)
	dataCh := make(chan string, 1)
	go serializeLinks(urir, basetm, format, dataCh, navonly, sess)
	for dt := range dataCh {
		fmt.Print(dt)
	}
	logInfo.Printf("Total Mementos: %d in %s", basetm.Len(), time.Since(start))
}

func memgatorService(w http.ResponseWriter, r *http.Request, urir string, format string, dttmp *time.Time) {
	start := time.Now()
	sess := new(Session)
	sess.Start = start
	upsession := "timemap"
	if dttmp != nil {
		upsession = "timegate"
	}
	defer benchmarker("SESSION", upsession, "Complete session", start, sess)
	benchmarker("AGGREGATOR", "createsess", "Session created", start, sess)
	logInfo.Printf("Aggregating Mementos for %s", urir)
	basetm := aggregateTimemap(urir, dttmp, sess)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Link, Location, X-Memento-Count, X-Generator")
	if dttmp == nil {
		w.Header().Set("X-Memento-Count", fmt.Sprintf("%d", basetm.Len()))
	}
	if basetm.Len() == 0 {
		http.NotFound(w, r)
		return
	}
	navonly, closest := setNavRels(basetm, dttmp, sess)
	if format == "redirect" {
		http.Redirect(w, r, closest, http.StatusFound)
		return
	}
	if format == "proxy" {
		nr, err := http.NewRequest(http.MethodGet, closest, nil)
		if err != nil {
			logError.Printf("Error creating proxy request (%s): %v", closest, err)
			http.Error(w, "Error creating proxy request for "+closest, http.StatusInternalServerError)
			return
		}
		logInfo.Printf("Serving as proxy for: %s", closest)
		reverseProxy.ServeHTTP(w, nr)
		return
	}
	dataCh := make(chan string, 1)
	if format == "timegate" {
		go serializeLinks(urir, basetm, "link", dataCh, navonly, sess)
		lnkhdr := ""
		for dt := range dataCh {
			lnkhdr += dt
		}
		lnkhdr = strings.Replace(lnkhdr, "\n", " ", -1)
		w.Header().Set("Link", lnkhdr)
		w.Header().Set("Vary", "accept-datetime")
		http.Redirect(w, r, closest, http.StatusFound)
		return
	}
	go serializeLinks(urir, basetm, format, dataCh, navonly, sess)
	mime, ok := mimeMap[strings.ToLower(format)]
	if ok {
		w.Header().Set("Content-Type", mime)
	}
	for dt := range dataCh {
		fmt.Fprint(w, dt)
	}
	logInfo.Printf("Total Mementos: %d in %s", basetm.Len(), time.Since(start))
}

func router(w http.ResponseWriter, r *http.Request) {
	var format, urir, rawuri, rawdtm string
	var dttm *time.Time
	var err error
	w.Header().Set("X-Generator", Name+":"+Version)
	orequri := r.URL.RequestURI()
	requri := strings.TrimPrefix(orequri, *root)
	endpoint := strings.SplitN(requri, "/", 2)[0]
	switch endpoint {
	case "timemap":
		if regs["tmappth"].MatchString(requri) {
			p := strings.SplitN(requri, "/", 3)
			format = p[1]
			rawuri = p[2]
		} else {
			err = fmt.Errorf("/timemap/{FORMAT}/{URI-R} (FORMAT => %s)", responseFormats)
		}
	case "timegate":
		if regs["tgatpth"].MatchString(requri) {
			p := strings.SplitN(requri, "/", 2)
			format = p[0]
			rawuri = p[1]
			gttm := time.Now().UTC()
			if hdtm := r.Header.Get("Accept-Datetime"); hdtm != "" {
				gttm, err = time.Parse(http.TimeFormat, hdtm)
				if err != nil {
					logError.Printf("Error parsing datetime (%s): %v", hdtm, err)
					http.Error(w, "Malformed Accept-Datetime: "+hdtm+"\nExpected in RFC1123 format", http.StatusBadRequest)
					return
				}
			}
			dttm = &gttm
		} else {
			err = fmt.Errorf("/timegate/{URI-R}")
		}
	case "memento", "api":
		if regs["rdrcpth"].MatchString(requri) {
			p := strings.SplitN(requri, "/", 3)
			format = "redirect"
			rawdtm = p[1]
			rawuri = p[2]
		} else if regs["descpth"].MatchString(requri) {
			p := strings.SplitN(requri, "/", 4)
			format = p[1]
			rawdtm = p[2]
			rawuri = p[3]
		} else {
			err = fmt.Errorf("/memento[/{FORMAT}|proxy]/{DATETIME}/{URI-R} (FORMAT => %s, DATETIME => %s)", responseFormats, validDatetimes)
		}
	case "monitor":
		if *monitor {
			logInfo.Printf("Benchmark monitoring client connected")
			broker.ServeHTTP(w, r)
		} else {
			logError.Printf("Benchmark monitoring not enabled, use --monitor flag to enable it")
			http.Error(w, "Benchmark monitoring not enabled", http.StatusNotImplemented)
		}
		return
	default:
		if *static != "" {
			logInfo.Printf("Serving static file: %s", orequri)
			http.StripPrefix(*root, http.FileServer(http.Dir(*static))).ServeHTTP(w, r)
			return
		}
		if endpoint == "" && requri != orequri {
			logInfo.Printf("Service info printed")
			fmt.Fprint(w, serviceInfo())
			return
		}
		logInfo.Printf("Delegated to default ServerMux: %s", orequri)
		http.DefaultServeMux.ServeHTTP(w, r)
		return
	}
	if err != nil {
		logError.Printf("Malformed request: %s", r.URL.RequestURI())
		http.Error(w, "Malformed request: "+r.URL.RequestURI()+"\nExpected: "+err.Error(), http.StatusBadRequest)
		return
	}
	urir, err = parseURI(rawuri)
	if err != nil {
		logError.Printf("URI parsing error (%s): %v", rawuri, err)
		http.Error(w, "Malformed URI-R: "+rawuri, http.StatusBadRequest)
		return
	}
	if rawdtm != "" {
		dttm, err = paddedTime(rawdtm)
		if err != nil {
			logError.Printf("Time parsing error (%s): %v", rawdtm, err)
			http.Error(w, "Malformed datetime: "+rawdtm+"\nExpected format: "+validDatetimes, http.StatusBadRequest)
			return
		}
	}
	memgatorService(w, r, urir, format, dttm)
}

func overrideFlags() {
	if *agent == fmt.Sprintf("%s:%s <{CONTACT}>", Name, Version) {
		*agent = fmt.Sprintf("%s:%s <%s>", Name, Version, *contact)
	}
	if *root = "/" + strings.Trim(*root, "/") + "/"; *root == "//" {
		*root = "/"
	}
	if *port == 80 {
		baseURL = fmt.Sprintf("http://%s%s", *host, *root)
	} else {
		baseURL = fmt.Sprintf("http://%s:%d%s", *host, *port, *root)
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if *proxy == "http://{HOST}[:{PORT}]{ROOT}" {
		*proxy = baseURL
	} else {
		*proxy = strings.TrimRight(*proxy, "/")
	}
}

func serviceInfo() (msg string) {
	msg = fmt.Sprintf("TimeMap   : %s/timemap/{FORMAT}/{URI-R}\nTimeGate  : %s/timegate/{URI-R} [Accept-Datetime]\nMemento   : %s/memento[/{FORMAT}|proxy]/{DATETIME}/{URI-R}\n", baseURL, baseURL, baseURL)
	if *monitor {
		msg += fmt.Sprintf("Benchmark : %s/monitor [SSE]\n", baseURL)
	}
	msg += fmt.Sprintf("\n# FORMAT          => %s\n# DATETIME        => %s\n# Accept-Datetime => Header in RFC1123 format\n", responseFormats, validDatetimes)
	return
}

func appInfo() (msg string) {
	return fmt.Sprintf("%s %s%s\n", Name, Version, Art)
}

func usage() {
	fmt.Fprintf(os.Stderr, appInfo())
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  %s [options] {URI-R}                            # TimeMap from CLI\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s [options] {URI-R} {%s} # Description of the closest Memento from CLI\n", os.Args[0], validDatetimes)
	fmt.Fprintf(os.Stderr, "  %s [options] server                             # Run as a Web Service\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\n")
}

func initLoggers() {
	logFatal = log.New(os.Stderr, "FATAL: ", log.Lshortfile)
	errorHandle := os.Stderr
	infoHandle := ioutil.Discard
	benchmarkHandle := ioutil.Discard
	if *logfile != "" {
		lgf, err := os.OpenFile(*logfile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			logFatal.Fatalf("Error opening log file (%s): %v\n", *logfile, err)
		}
		errorHandle = lgf
		infoHandle = lgf
		benchmarkHandle = lgf
	}
	if *benchmark != "" {
		prf, err := os.OpenFile(*benchmark, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			logFatal.Fatalf("Error opening benchmark file (%s): %v\n", *benchmark, err)
		}
		benchmarkHandle = prf
	}
	if *verbose {
		errorHandle = os.Stderr
		infoHandle = os.Stderr
		benchmarkHandle = os.Stderr
	}
	logError = log.New(errorHandle, "ERROR: ", log.Ldate|log.Lmicroseconds|log.Lshortfile)
	logInfo = log.New(infoHandle, "INFO: ", log.Ldate|log.Lmicroseconds)
	logBenchmark = log.New(benchmarkHandle, "BENCHMARK: ", log.Ldate|log.Lmicroseconds)
}

func initNetwork() {
	transport = http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   *contimeout,
			KeepAlive: *restimeout,
		}).DialContext,
		ResponseHeaderTimeout: *hdrtimeout,
		IdleConnTimeout:       *restimeout,
		MaxIdleConnsPerHost:   5,
	}
	client = http.Client{
		Transport: &transport,
		Timeout:   *restimeout,
	}
	reverseProxy = &httputil.ReverseProxy{
		Transport:     &transport,
		FlushInterval: time.Duration(100*time.Millisecond),
		Director: func(r *http.Request) {
			r.URL.Path = regs["memdttm"].ReplaceAllString(r.URL.Path, "/${1}id_/")
		},
	}
}

func main() {
	start := time.Now()
	flag.Usage = usage
	flag.Parse()
	overrideFlags()
	if *version {
		fmt.Printf("%s %s\n", Name, Version)
		os.Exit(1)
	}
	target := flag.Arg(0)
	if target == "" {
		flag.Usage()
		os.Exit(1)
	}
	initLoggers()
	initNetwork()
	logInfo.Printf("Initializing %s:%s...", Name, Version)
	logInfo.Printf("Loading archives from %s", *arcsloc)
	body, err := readArchives()
	if err != nil {
		logFatal.Fatalf("Error reading list of archives (%s): %s\n", *arcsloc, err)
	}
	err = json.Unmarshal(body, &archives)
	archives.sanitize()
	archives.filterIgnored()
	sort.Sort(archives)
	if err != nil {
		logFatal.Fatalf("Error parsing JSON (%s): %s\n", *arcsloc, err)
	}
	if target == "server" {
		fmt.Printf(appInfo())
		fmt.Printf(serviceInfo())
		if *agent == fmt.Sprintf("%s:%s <@WebSciDL>", Name, Version) && !*spoof {
			fmt.Printf("\nATTENTION: Please consider customizing the contact info or the whole user-agent!\nCurrent user-agent: %s\nCheck CLI help (memgator --help) for options.\n", *agent)
		}
		if *monitor {
			broker = sse.NewServer()
		}
		addr := fmt.Sprintf(":%d", *port)
		err = http.ListenAndServe(addr, http.HandlerFunc(router))
		if err != nil {
			logFatal.Fatalf("Error listening: %s\n", err)
		}
	} else {
		urir, err := parseURI(target)
		if err != nil {
			logFatal.Fatalf("URI parsing error (%s): %v\n", target, err)
		}
		var dttm *time.Time
		if rawdtm := flag.Arg(1); rawdtm != "" {
			if regs["dttmstr"].MatchString(rawdtm) {
				dttm, err = paddedTime(rawdtm)
				if err != nil {
					logFatal.Fatalf("Time parsing error (%s): %v\n", rawdtm, err)
				}
			} else {
				logFatal.Fatalf("Malformed datetime (%s): %s\n", validDatetimes, rawdtm)
			}
		}
		memgatorCli(urir, *format, dttm)
	}
	elapsed := time.Since(start)
	logInfo.Printf("Uptime: %s", elapsed)
}
