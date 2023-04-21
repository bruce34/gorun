package myScriptUsingLibrary_

import (
	"fmt"
	"net/http"
)

func ProxyStart(port int) {
	http.HandleFunc("/",
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte("OK"))
		})
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}
