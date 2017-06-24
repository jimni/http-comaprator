package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/lucas-clemente/quic-go/h2quic"
	"github.com/lucas-clemente/quic-go/utils"
	"github.com/patrickmn/go-cache"
	"github.com/satori/go.uuid"
)

var inmem_cache *cache.Cache

const json_delay_ms = 1000
const cache_seconds = "60"
const json_size_strings = 500

type Page struct {
	Title string
	Body  []byte
}

func (p *Page) save() error {
	filename := p.Title + ".txt"
	return ioutil.WriteFile(filename, p.Body, 0600)
}

func schemeResolver(request *http.Request) (scheme string) {
	scheme = "http"
	if request.TLS != nil {
		scheme = "https"
	}
	return
}

func protocolResolver(request *http.Request) (transport, port, protocol string) {

	protocols := map[string]string{
		"8080": "http/1.1",
		"8081": "http/1.1 + TLS",
		"8082": "http/2",
		"8083": "QUIC",
	}

	_, port, portError := net.SplitHostPort(request.Host)
	if portError != nil {
		port = "80"
	}
	protocol = protocols[port]
	transport = "tcp"
	if protocol == "QUIC" {
		transport = "udp"
	}
	return
}

func simpleHandler(response http.ResponseWriter, request *http.Request) {
	log.Printf("%s %s%s", request.Method, request.Host, request.RequestURI)
	response.Header().Add("Link", "</cookie-test>; rel=prefetch")
	scheme := schemeResolver(request)
	fmt.Fprintf(
		response, "<html><h4>request:</h4><div>%s://%s%s</div>"+
			"<h4>headers:</h4><div>%s</div></html>",
		scheme, request.Host, request.URL.Path, request.Header)
}

func cookieTestHandler(response http.ResponseWriter, request *http.Request) {
	log.Printf("%s %s%s", request.Method, request.Host, request.RequestURI)
	response.Header().Add(
		"Set-Cookie",
		"test-cookie-timestamp="+time.Now().Format(time.Stamp))
	fmt.Fprintf(response, "ok")
}

func standaloneDemoPageHandler(response http.ResponseWriter, request *http.Request) {
	log.Printf("%s %s%s", request.Method, request.Host, request.RequestURI)
	page, loadStatus := loadPage(request.RequestURI[1:] + ".html")
	if loadStatus != nil {
		fmt.Println("load failed with err: ", loadStatus)
	} else {
		page := string(page.Body)
		transport, port, protocol := protocolResolver(request)

		page = strings.Replace(page, "$transport$", transport, -1)
		page = strings.Replace(page, "$port$", port, -1)
		page = strings.Replace(page, "$protocol$", protocol, -1)
		page = strings.Replace(page, "$hostname$", request.Host, -1)
		page = strings.Replace(page, "$scheme$", schemeResolver(request), -1)
		page = strings.Replace(page, "$data_delay$", strconv.Itoa(json_delay_ms), -1)
		fmt.Fprintf(response, page)
	}
}

func loadPage(title string) (*Page, error) {
	filename := title
	if !strings.Contains(title, ".") {
		filename += ".txt"
	}
	body, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return &Page{Title: title, Body: body}, nil
}

func getCertBody(filename string) []byte {
	body, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	return body
}

func makeFakeJson(line_number int) string {
	type Data struct {
		Name    string
		Email   string
		Address string
	}

	object := make(map[string]Data)

	time.Sleep(time.Duration(json_delay_ms) * time.Millisecond)

	for i := 0; i < line_number; i++ {
		object[uuid.NewV4().String()] = Data{Name: randomdata.SillyName(), Email: randomdata.Email(), Address: randomdata.Address()}
	}
	json_response, _ := json.Marshal(object)
	return string(json_response)
}

func jsonStubber(response http.ResponseWriter, request *http.Request) {
	log.Printf("%s %s%s", request.Method, request.Host, request.RequestURI)
	var page string
	json, cached := inmem_cache.Get(request.RequestURI)
	if cached {
		page = json.(string)
		log.Printf("Serving %s from cache", request.RequestURI)
	} else {
		page = makeFakeJson(json_size_strings)
		log.Printf(
			"%s not found in cache. Generating new data",
			request.RequestURI)
	}
	if request.Header.Get("Purpose") == "prefetch" {
		response.Header().Add("Cache-Control", "private, max-age="+cache_seconds)
		log.Printf("Sent asset for prefetching with max-age = %s second", cache_seconds)
	}
	fmt.Fprintf(response, page)
}

func jsonCacher(response http.ResponseWriter, request *http.Request) {
	log.Printf("%s %s%s", request.Method, request.Host, request.RequestURI)
	page := makeFakeJson(json_size_strings)
	assetURI := strings.TrimPrefix(request.RequestURI, "/cache")
	inmem_cache.Set(assetURI, page, cache.DefaultExpiration)
	log.Printf(
		"Putting %s to cache. Items cached: %d",
		assetURI, inmem_cache.ItemCount())
	fmt.Fprintf(response, "cached %s", assetURI)
}

func modifiedFileServer(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		log.Printf("%s %s%s", r.Method, r.Host, r.RequestURI)
		h.ServeHTTP(w, r)
	}
}

// func getSslFiles() (string, string) {
// 	if runtime.GOOS == "linux" {
// 		return "/home/jim/ssl/vps.nikitin.su/fullchain1.pem",
// 			"/home/jim/ssl/vps.nikitin.su/privkey1.pem"
// 	}
// 	return "ssl/server.crt", "ssl/server.key"
// }

func getSslFiles() (string, string) {
	return "ssl/server.crt", "ssl/server.key"
}

func logCache(key string, value interface{}) {
	log.Printf(
		"Evicted from cache: {{%s}} . Items in cache: %d",
		key, inmem_cache.ItemCount())
}

func main() {
	inmem_cache = cache.New(5*time.Minute, 10*time.Minute)
	inmem_cache.OnEvicted(logCache)
	fmt.Println("Listening on ports: 8080-8083")
	var sslCert, sslKey = getSslFiles()
	fmt.Println(
		"Using cert from:", sslCert, "\n",
		string(getCertBody(sslCert)[:50]), "...")
	http.HandleFunc("/", simpleHandler)
	http.HandleFunc("/json", jsonStubber)
	http.HandleFunc("/cache/", jsonCacher)
	http.HandleFunc("/cookie-test", cookieTestHandler)
	http.HandleFunc("/benchmark", standaloneDemoPageHandler)
	http.HandleFunc("/demo", standaloneDemoPageHandler)
	http.HandleFunc("/demo_sharding", standaloneDemoPageHandler)
	http.HandleFunc("/ui", standaloneDemoPageHandler)
	http.Handle("/static/", modifiedFileServer(http.FileServer(http.Dir("."))))
	go http.ListenAndServe(":8080", nil)
	go http.ListenAndServeTLS(":8082", sslCert, sslKey, nil)
	https := &http.Server{
		Addr:         ":8081",
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
	}
	go https.ListenAndServeTLS(sslCert, sslKey)
	// utils.SetLogLevel(utils.LogLevelDebug)
	// utils.SetLogLevel(utils.LogLevelInfo)
	utils.SetLogLevel(utils.LogLevelError)
	h2quic.ListenAndServeQUIC(":8083", sslCert, sslKey, nil)
}
