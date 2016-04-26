package main

import (
	"container/list"
	"encoding/json"
	"fmt"
	flag "github.com/oduwsdl/memgator/pkg/mflag"
	"io/ioutil"
	"log"
	"net"
	"net/http"
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
	Version = "1.0-rc4"
	Art     = `
   _____                  _______       __
  /     \  _____  _____  / _____/______/  |___________
 /  Y Y  \/  __ \/     \/  \  ___\__  \   _/ _ \_   _ \
/   | |   \  ___/  Y Y  \   \_\  \/ __ |  | |_| |  | \/
\__/___\__/\____\__|_|__/\_______/_____|__|\___/|__|
`
)

var (
	logProfile *log.Logger
	logInfo    *log.Logger
	logError   *log.Logger
)

var format = flag.String([]string{"f", "-format"}, "Link", "Output format - Link/JSON/CDXJ")
var arcsloc = flag.String([]string{"a", "-arcs"}, "http://oduwsdl.github.io/memgator/archives.json", "Local/remote JSON file path/URL for list of archives")
var logfile = flag.String([]string{"l", "-log"}, "", "Log file location - Defaults to STDERR")
var profile = flag.String([]string{"P", "-profile"}, "", "Profile file location - Defaults to Logfile")
var contact = flag.String([]string{"c", "-contact"}, "@WebSciDL", "Email/URL/Twitter handle - Used in the user-agent")
var agent = flag.String([]string{"A", "-agent"}, fmt.Sprintf("%s:%s <{CONTACT}>", Name, Version), "User-agent string sent to archives")
var host = flag.String([]string{"H", "-host"}, "localhost", "Host name - only used in web service mode")
var servicebase = flag.String([]string{"s", "-service"}, "http://{HOST}[:{PORT}]", "Service base URL - default based on host & port")
var mapbase = flag.String([]string{"m", "-timemap"}, "http://{SERVICE}/timemap", "TimeMap base URL - default based on service URL")
var gatebase = flag.String([]string{"g", "-timegate"}, "http://{SERVICE}/timegate", "TimeGate base URL - default based on service URL")
var port = flag.Int([]string{"p", "-port"}, 1208, "Port number - only used in web service mode")
var topk = flag.Int([]string{"k", "-topk"}, -1, "Aggregate only top k archives based on probability")
var verbose = flag.Bool([]string{"V", "-verbose"}, false, "Show Info and Profiling messages on STDERR")
var version = flag.Bool([]string{"v", "-version"}, false, "Show name and version")
var contimeout = flag.Duration([]string{"t", "-contimeout"}, time.Duration(5*time.Second), "Connection timeout for each archive")
var hdrtimeout = flag.Duration([]string{"T", "-hdrtimeout"}, time.Duration(15*time.Second), "Header timeout for each archive")
var restimeout = flag.Duration([]string{"r", "-restimeout"}, time.Duration(20*time.Second), "Response timeout for each archive")

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
	"dttmstr": regexp.MustCompile(`^(\d{4})(\d{2})?(\d{2})?(\d{2})?(\d{2})?(\d{2})?$`),
	"tmappth": regexp.MustCompile(`^timemap/(link|json|cdxj)/.+`),
	"tgatpth": regexp.MustCompile(`^timegate/.+`),
	"memtpth": regexp.MustCompile(`^memento/(redirect|link|json|cdxj)/(\d{4})(\d{2})?(\d{2})?(\d{2})?(\d{2})?(\d{2})?/.+`),
}

func dialTimeout(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, *contimeout)
}

var transport = http.Transport{
	Dial: dialTimeout,
	ResponseHeaderTimeout: *hdrtimeout,
}

var client = http.Client{
	Transport: &transport,
	Timeout:   *restimeout,
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
	i := 0
	j := 0
	for ; j < strlen; j++ {
		switch lnkstr[j] {
		case '"':
			q = !q
		case ',':
			if !q {
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

func fetchTimemap(urir string, arch Archive, tmCh chan *list.List, wg *sync.WaitGroup, dttmp *time.Time, sess *Session) {
	start := time.Now()
	defer wg.Done()
	url := arch.Timemap + urir
	if dttmp != nil {
		url = arch.Timegate + urir
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		profileTime(arch.ID, "timemapfetch", fmt.Sprintf("Request error in %s", arch.Name), start, sess)
		logError.Printf("%s => Request error: %v", arch.ID, err)
		return
	}
	req.Header.Add("User-Agent", *agent)
	var res *http.Response
	if dttmp == nil {
		res, err = client.Do(req)
	} else {
		req.Header.Add("Accept-Datetime", dttmp.Format(http.TimeFormat))
		res, err = transport.RoundTrip(req)
	}
	if err != nil {
		profileTime(arch.ID, "timemapfetch", fmt.Sprintf("Network error in %s", arch.Name), start, sess)
		logError.Printf("%s => Network error: %v", arch.ID, err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusFound {
		profileTime(arch.ID, "timemapfetch", fmt.Sprintf("Response error in %s, Stutus: %d", arch.Name, res.StatusCode), start, sess)
		logInfo.Printf("%s => Response error: %s", arch.ID, res.Status)
		return
	}
	lnks := res.Header.Get("Link")
	if dttmp == nil {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			profileTime(arch.ID, "timemapfetch", fmt.Sprintf("Response read error in %s", arch.Name), start, sess)
			logError.Printf("%s => Response read error: %v", arch.ID, err)
			return
		}
		lnks = string(body)
	}
	profileTime(arch.ID, "timemapfetch", fmt.Sprintf("TimeMap fethched from %s", arch.Name), start, sess)
	start = time.Now()
	lnkrcvd := make(chan string, 1)
	lnksplt := make(chan string, 128)
	lnkrcvd <- lnks
	go splitLinks(lnkrcvd, lnksplt)
	tml := extractMementos(lnksplt)
	tmCh <- tml
	profileTime(arch.ID, "extractmementos", fmt.Sprintf("%d Mementos extracted from %s", tml.Len(), arch.Name), start, sess)
	logInfo.Printf("%s => Success: %d mementos", arch.ID, tml.Len())
}

func serializeLinks(urir string, basetm *list.List, format string, dataCh chan string, navonly bool, sess *Session) {
	start := time.Now()
	defer profileTime("AGGREGATOR", "serialize", fmt.Sprintf("%d mementos serialized", basetm.Len()), start, sess)
	defer close(dataCh)
	switch strings.ToLower(format) {
	case "link":
		dataCh <- fmt.Sprintf(`<%s>; rel="original",`+"\n", urir)
		if !navonly {
			dataCh <- fmt.Sprintf(`<%s/link/%s>; rel="self"; type="application/link-format",`+"\n", *mapbase, urir)
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
		dataCh <- fmt.Sprintf(`<%s/link/%s>; anchor="%s"; rel="timemap"; type="application/link-format",`+"\n", *mapbase, urir, urir)
		dataCh <- fmt.Sprintf(`<%s/json/%s>; anchor="%s"; rel="timemap"; type="application/json",`+"\n", *mapbase, urir, urir)
		dataCh <- fmt.Sprintf(`<%s/cdxj/%s>; anchor="%s"; rel="timemap"; type="application/cdxj+ors",`+"\n", *mapbase, urir, urir)
		dataCh <- fmt.Sprintf(`<%s/%s>; anchor="%s"; rel="timegate"`+"\n", *gatebase, urir, urir)
	case "json":
		dataCh <- fmt.Sprintf("{\n"+`  "original_uri": "%s",`+"\n", urir)
		if !navonly {
			dataCh <- fmt.Sprintf(`  "self": "%s/json/%s",`+"\n", *mapbase, urir)
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
		dataCh <- fmt.Sprintf(`    "link_format": "%s/link/%s",`+"\n", *mapbase, urir)
		dataCh <- fmt.Sprintf(`    "json_format": "%s/json/%s",`+"\n", *mapbase, urir)
		dataCh <- fmt.Sprintf(`    "cdxj_format": "%s/cdxj/%s"`+"\n  },\n", *mapbase, urir)
		dataCh <- fmt.Sprintf(`  "timegate_uri": "%s/%s"`+"\n}\n", *gatebase, urir)
	case "cdxj":
		dataCh <- fmt.Sprintf(`@context ["http://tools.ietf.org/html/rfc7089"]` + "\n")
		if !navonly {
			dataCh <- fmt.Sprintf(`@id {"uri": "%s/cdxj/%s"}`+"\n", *mapbase, urir)
		}
		dataCh <- fmt.Sprintf(`@keys ["memento_datetime_YYYYMMDDhhmmss"]` + "\n")
		dataCh <- fmt.Sprintf(`@meta {"original_uri": "%s"}`+"\n", urir)
		dataCh <- fmt.Sprintf(`@meta {"timegate_uri": "%s/%s"}`+"\n", *gatebase, urir)
		dataCh <- fmt.Sprintf(`@meta {"timemap_uri": {"link_format": "%s/link/%s", "json_format": "%s/json/%s", "cdxj_format": "%s/cdxj/%s"}`+"\n", *mapbase, urir, *mapbase, urir, *mapbase, urir)
		for e := basetm.Front(); e != nil; e = e.Next() {
			lnk := e.Value.(Link)
			if navonly && lnk.NavRels == nil {
				continue
			}
			rels := "memento"
			if lnk.NavRels != nil {
				rels = strings.Join(lnk.NavRels, " ") + " " + rels
			}
			dataCh <- fmt.Sprintf(`%s {"uri": "%s", "rel"="%s", "datetime"="%s"}`+"\n", lnk.Timestr, lnk.Href, rels, lnk.Datetime)
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
		wg.Add(1)
		go fetchTimemap(urir, arch, tmCh, &wg, dttmp, sess)
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
		profileTime("AGGREGATOR", "aggregate", fmt.Sprintf("%d Mementos accumulated and sorted", basetm.Len()), start, sess)
	}
	return
}

func parseURI(uri string) (urir string, err error) {
	if !regs["isprtcl"].MatchString(uri) {
		uri = "http://" + uri
	}
	u, err := url.Parse(uri)
	if err != nil {
		logError.Printf("URI parsing error (%s): %v", uri, err)
		return
	}
	urir = u.String()
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
	if err != nil {
		logError.Printf("Time parsing error (%s): %v", dttmstr, err)
	}
	dttm = &dtm
	return
}

func profileTime(origin string, role string, info string, start time.Time, sess *Session) {
	end := time.Now()
	begin := sess.Start.UnixNano()
	info += fmt.Sprintf(" - Duration: %v", end.Sub(start))
	logProfile.Printf(`%d {"origin": "%s", "role": "%s", "info": "%s", "start": %d, "end": %d}`, begin, origin, role, info, start.UnixNano()-begin, end.UnixNano()-begin)
}

func setNavRels(basetm *list.List, dttmp *time.Time, sess *Session) (navonly bool, closest string) {
	start := time.Now()
	defer profileTime("AGGREGATOR", "setnav", "Navigational relations annotated", start, sess)
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
	defer profileTime("SESSION", "session", "Complete session", start, sess)
	profileTime("AGGREGATOR", "createsess", "Session created", start, sess)
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
	defer profileTime("SESSION", "setnav", "Complete session", start, sess)
	profileTime("AGGREGATOR", "createsess", "Session created", start, sess)
	logInfo.Printf("Aggregating Mementos for %s", urir)
	basetm := aggregateTimemap(urir, dttmp, sess)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Link, Location, X-Memento-Count, X-Generator")
	w.Header().Set("X-Generator", Name+":"+Version)
	if dttmp == nil {
		w.Header().Set("X-Memento-Count", fmt.Sprintf("%d", basetm.Len()))
	}
	if basetm.Len() == 0 {
		http.NotFound(w, r)
		return
	}
	navonly, closest := setNavRels(basetm, dttmp, sess)
	dataCh := make(chan string, 1)
	if format == "timegate" || format == "redirect" {
		go serializeLinks(urir, basetm, "link", dataCh, navonly, sess)
		lnkhdr := ""
		for dt := range dataCh {
			lnkhdr += dt
		}
		lnkhdr = strings.Replace(lnkhdr, "\n", " ", -1)
		w.Header().Set("Link", lnkhdr)
		if format == "timegate" {
			w.Header().Set("Vary", "accept-datetime")
		}
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

func welcome(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, serviceInfo())
}

func router(w http.ResponseWriter, r *http.Request) {
	var format, urir, rawuri, rawdtm string
	var dttm *time.Time
	var err error
	requri := r.URL.RequestURI()[1:]
	endpoint := strings.SplitN(requri, "/", 2)[0]
	switch endpoint {
	case "timemap":
		if regs["tmappth"].MatchString(requri) {
			p := strings.SplitN(requri, "/", 3)
			format = p[1]
			rawuri = p[2]
		} else {
			err = fmt.Errorf("/timemap/link|json|cdxj/{URI-R}")
		}
	case "memento":
		if regs["memtpth"].MatchString(requri) {
			p := strings.SplitN(requri, "/", 4)
			format = p[1]
			rawdtm = p[2]
			rawuri = p[3]
		} else {
			err = fmt.Errorf("/memento/redirect|link|json|cdxj/{YYYY[MM[DD[hh[mm[ss]]]]]}/{URI-R}")
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
	default:
		logInfo.Printf("Delegated to default ServerMux: %s", r.URL.RequestURI())
		w.Header().Set("X-Generator", Name+":"+Version)
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
		http.Error(w, "Malformed URI-R: "+rawuri, http.StatusBadRequest)
		return
	}
	if rawdtm != "" {
		dttm, err = paddedTime(rawdtm)
		if err != nil {
			http.Error(w, "Malformed datetime: "+rawdtm+"\nExpected format: YYYY[MM[DD[hh[mm[ss]]]]]", http.StatusBadRequest)
			return
		}
	}
	memgatorService(w, r, urir, format, dttm)
}

func overrideFlags() {
	if *agent == fmt.Sprintf("%s:%s <{CONTACT}>", Name, Version) {
		*agent = fmt.Sprintf("%s:%s <%s>", Name, Version, *contact)
	}
	if *servicebase == "http://{HOST}[:{PORT}]" {
		if *port == 80 {
			*servicebase = fmt.Sprintf("http://%s", *host)
		} else {
			*servicebase = fmt.Sprintf("http://%s:%d", *host, *port)
		}
	}
	if *mapbase == "http://{SERVICE}/timemap" {
		*mapbase = fmt.Sprintf("%s/timemap", *servicebase)
	}
	if *gatebase == "http://{SERVICE}/timegate" {
		*gatebase = fmt.Sprintf("%s/timegate", *servicebase)
	}
}

func serviceInfo() (msg string) {
	return fmt.Sprintf("TimeMap             : %s/link|json|cdxj/{URI-R}\nTimeGate            : %s/{URI-R} [Accept-Datetime Header]\nMemento Description : %s/memento/link|json|cdxj/{YYYY[MM[DD[hh[mm[ss]]]]]}/{URI-R}\nMemento Redirect    : %s/memento/redirect/{YYYY[MM[DD[hh[mm[ss]]]]]}/{URI-R}\n", *mapbase, *gatebase, *servicebase, *servicebase)
}

func appInfo() (msg string) {
	return fmt.Sprintf("%s %s%s\n", Name, Version, Art)
}

func usage() {
	fmt.Fprintf(os.Stderr, appInfo())
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  %s [options] {URI-R}                                # TimeMap CLI\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s [options] {URI-R} {YYYY[MM[DD[hh[mm[ss]]]]]}     # Memento Description CLI\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s [options] server                                 # Run as a Web Service\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\n")
}

func initLoggers() {
	errorHandle := os.Stderr
	infoHandle := ioutil.Discard
	profileHandle := ioutil.Discard
	if *logfile != "" {
		lgf, err := os.OpenFile(*logfile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening log file (%s): %v\n", *logfile, err)
			os.Exit(1)
		}
		errorHandle = lgf
		infoHandle = lgf
		profileHandle = lgf
	}
	if *profile != "" {
		prf, err := os.OpenFile(*profile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening profile file (%s): %v\n", *profile, err)
			os.Exit(1)
		}
		profileHandle = prf
	}
	if *verbose {
		errorHandle = os.Stderr
		infoHandle = os.Stderr
		profileHandle = os.Stderr
	}
	logError = log.New(errorHandle, "ERROR: ", log.Ldate|log.Lmicroseconds|log.Lshortfile)
	logInfo = log.New(infoHandle, "INFO: ", log.Ldate|log.Lmicroseconds)
	logProfile = log.New(profileHandle, "PROFILE: ", log.Ldate|log.Lmicroseconds)
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
	logInfo.Printf("Initializing %s:%s...", Name, Version)
	logInfo.Printf("Loading archives from %s", *arcsloc)
	body, err := readArchives()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading list of archives (%s): %s\n", *arcsloc, err)
		os.Exit(1)
	}
	err = json.Unmarshal(body, &archives)
	archives.filterIgnored()
	sort.Sort(archives)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JSON (%s): %s\n", *arcsloc, err)
		os.Exit(1)
	}
	if target == "server" {
		fmt.Printf(appInfo())
		fmt.Printf(serviceInfo())
		addr := fmt.Sprintf(":%d", *port)
		http.HandleFunc("/", welcome)
		http.ListenAndServe(addr, http.HandlerFunc(router))
	} else {
		urir, err := parseURI(target)
		if err != nil {
			os.Exit(1)
		}
		var dttm *time.Time
		if rawdtm := flag.Arg(1); rawdtm != "" {
			if regs["dttmstr"].MatchString(rawdtm) {
				dttm, err = paddedTime(rawdtm)
				if err != nil {
					os.Exit(1)
				}
			} else {
				logError.Printf("Malformed datetime {YYYY[MM[DD[hh[mm[ss]]]]]}: %s", rawdtm)
				os.Exit(1)
			}
		}
		memgatorCli(urir, *format, dttm)
	}
	elapsed := time.Since(start)
	logInfo.Printf("Uptime: %s", elapsed)
}
