// User comments are left in the same place when go.mod and go.sum sections are updated
//
//
// go.mod >>>
// module myscript2
// go 1.13
// require github.com/sirupsen/logrus v1.4.2
// <<< go.mod

// go.sum >>>
// github.com/davecgh/go-spew v1.1.1 h1:vj9j/u1bqnvCEfJOwUhtlOARqs3+rkHYY13jYWTU97c=
// github.com/davecgh/go-spew v1.1.1/go.mod h1:J7Y8YcW2NihsgmVo/mv3lAwl/skON4iLHjSsI+c5H38=
// github.com/konsorten/go-windows-terminal-sequences v1.0.1 h1:mweAR1A6xJ3oS2pRaGiHgQ4OO8tzTaLawm8vnODuwDk=
// github.com/konsorten/go-windows-terminal-sequences v1.0.1/go.mod h1:T0+1ngSBFLxvqU3pZ+m/2kptfBszLMUkC4ZK/EgS/cQ=
// github.com/pmezard/go-difflib v1.0.0 h1:4DBwDE0NGyQoBHbLQYPwSUPoCMWR5BEzIk/f1lZbAQM=
// github.com/pmezard/go-difflib v1.0.0/go.mod h1:iKH77koFhYxTK1pcRnkKkqfTogsbg7gZNVY4sRDYZ/4=
// github.com/sirupsen/logrus v1.4.2 h1:SPIRibHv4MatM3XXNO2BJeFLZwZ2LvZgfQ5+UNI2im4=
// github.com/sirupsen/logrus v1.4.2/go.mod h1:tLMulIdttU9McNUspp0xgXVQah82FyeX6MwdIuYE2rE=
// github.com/stretchr/objx v0.1.1/go.mod h1:HFkY916IF+rwdDfMAkV7OtwuqBVzrE8GR6GFx+wExME=
// github.com/stretchr/testify v1.2.2 h1:bSDNvY7ZPG5RlJ8otE/7V6gMiyenm9RtJ7IUVIAoJ1w=
// github.com/stretchr/testify v1.2.2/go.mod h1:a8OnRcib4nhh0OaRAV+Yts87kKdq0PP7pXfy6kDkUVs=
// golang.org/x/sys v0.0.0-20190422165155-953cdadca894 h1:Cz4ceDQGXuKRnVBDTS23GTn/pU5OE2C0WrNTOYK1Uuc=
// golang.org/x/sys v0.0.0-20190422165155-953cdadca894/go.mod h1:h1NjWce9XRLGQEsW7wpKNCjG9DtNlClVuFLEZdDNbEs=
// <<< go.sum

package main

import (
	"flag"
	"fmt"
	log "github.com/sirupsen/logrus"
	"myscript2/myscript2_"
)

func Usage() {
	fmt.Fprintf(flag.CommandLine.Output(),
		flag.CommandLine.Name()+`: Mirror back http responses depending on the http request 

Send to http://localhost:%d/<RETURN_CODE>/<response> or POST/PUT a body for the response.

e.g. curl -v http://localhost:8083/503/exampleResponse
	 curl -v http://localhost:8083/200
     echo "hello there how are you?" | curl -v -X PUT -d @- http://127.0.0.1:8083/501

`)
	fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", flag.CommandLine.Name())
	flag.PrintDefaults()
}

func main() {
	flag.Usage = Usage
	port := flag.Int("port", 8083, "port to listen to https requests on")
	logLevel := flag.String("logLevel", "info", "log level to use: debug,info,warn")
	flag.Parse()
	level, err := log.ParseLevel(*logLevel)
	if err != nil {
		log.Fatalf("Unknown logLevel %s", *logLevel)
	}
	log.SetLevel(log.Level(level))

	// go has a weird way of specifying timestamps! https://stackoverflow.com/questions/20234104/how-to-format-current-time-using-a-yyyymmddhhmmss-format
	log.SetFormatter(&log.TextFormatter{TimestampFormat: "2006-01-02 15:04:05.999", FullTimestamp: true})

	log.Infof("Starting on port %d", *port)

	myscript2_.ProxyStart(*port)
}
