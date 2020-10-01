package myscript2_

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

func ProxyStart(port int) {
	tr := &http.Transport{
		DisableKeepAlives: false,
	}
	client := &http.Client{Transport: tr}

	http.HandleFunc("/",
		func(w http.ResponseWriter, r *http.Request) {
			resp, code := processRequest(r, client)
			w.WriteHeader(code)
			fmt.Fprintf(w, resp)
		})
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func processRequest(r *http.Request, client *http.Client) (response string, code int) {
	log.Infof("Src %s Method %s URL %s", r.RemoteAddr, r.Method, r.URL.Path, r)
	body, _ := ioutil.ReadAll(r.Body)

	splits := strings.SplitN(r.URL.Path, "/", 3)
	if len(splits) >= 3 {
		response = splits[2]
	}
	if len(splits) >= 2 {
		code, _ = strconv.Atoi(splits[1])
	}
	if code < 100 || code > 599 {
		code = 200
	}
	if len(body) > 0 {
		response = response + string(body)
		log.Infof("Body %s", string(body))
	}

	return response, code
}
