// User comments are left in the same place when go.mod and go.sum sections are updated
//
//
// go.mod >>>
// module myscript2
// go 1.18
// <<< go.mod

// go.sum >>>
// 
// <<< go.sum

package main

import (
	"flag"
	"fmt"
	. "myscript2/myscript2_"
)

func main() {
	var port int
	flag.IntVar(&port, "port", 8083, "port to listen to https requests on")
	flag.Parse()

	fmt.Printf("Starting on port %d", port)
	ProxyStart(port)
}
