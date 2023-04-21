// User comments are left in the same place when go.mod and go.sum sections are updated
//
//
// go.mod >>>
// module myScriptUsingLibrary
// go 1.18
// <<< go.mod

// go.sum >>>
// 
// <<< go.sum

package main

import (
	"exampleLibrary"
	"flag"
	"fmt"
	. "myScriptUsingLibrary/myScriptUsingLibrary_"
)

func main() {
	var port int
	flag.IntVar(&port, "port", 8083, "port to listen to https requests on")
	flag.Parse()

	port = exampleLibrary.Add1000(port)
	fmt.Printf("Starting on port %d", port)
	ProxyStart(port)
}
