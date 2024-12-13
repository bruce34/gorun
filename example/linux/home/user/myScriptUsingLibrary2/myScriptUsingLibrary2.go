// User comments are left in the same place when go.mod and go.sum sections are updated
//
// go.mod >>>
// :module myScriptUsingLibrary2
// :go 1.18
// <<< go.mod

// go.sum >>>
// :
// <<< go.sum

// go.work >>>
// :go 1.18
// :use (
// :	.
// :	../_goCommonLibs/exampleLibrary
// :)
// <<< go.work

// go.work.sum >>>
// :github.com/stretchr/objx v0.1.0 h1:4G4v2dO3VZwixGIRoQ5Lfboy6nUhCyYzaqnIAPPhYs4=
// :gopkg.in/check.v1 v0.0.0-20161208181325-20d25e280405 h1:yhCVgyC4o1eVCa2tZl7eS0r+SDo693bJlVdllGtEeKM=
// <<< go.work.sum

package main

import (
	// notice the exampleLibrary is available due to the go.work file, and the go.work is embedded in the comments
	// above after running the Makefile. This is just a repeat to show that two separate scripts can have the same
	// shared code available to them.
	"exampleLibrary"
	"flag"
	"fmt"
	. "myScriptUsingLibrary2/myScriptUsingLibrary2_"
)

func main() {
	var port int
	flag.IntVar(&port, "port", 8083, "port to listen to https requests on")
	flag.Parse()

	port = exampleLibrary.Add1000(port)
	fmt.Printf("Starting on port %d", port)
	ProxyStart(port)
}
