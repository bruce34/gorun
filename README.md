# gorun

## What is it?
gorun is a tool that enables go source code "scripts" to be run much like a shell or python script.

Note, this version is a mostly compatible rewrite of github.com/erning/gorun under a more permissive license but with a
focus more on repeatable builds for a company environment. Development is done on both Mac and Linux, but with
deployments on Linux. It may also still work on Windows, but that is untested and not the main focus of this project at
this time.

## Simple Example
As an example, copy the following content to a file named "hello.go" (or "hello", if you prefer):

```go
#!/usr/bin/env gorun

package main

func main() {
    println("Hello world!")
}
```

Then, run it:

```
$ chmod +x hello.go
$ ./hello.go
Hello world!
```

This relies on the [shebang](https://en.wikipedia.org/wiki/Shebang_(Unix)) to run gorun, which copies and compiles the
hello.go "script". It does have the downside that go build will no longer work directly on the file, IDEs will get
confused etc. We can fix that on Linux (at least) 

## Run go code directly

See the section 'Executable' below on how to make this runnable with just a `chmod a+x`
```go
// my first hello world script

package main

func main() {
    println("Hello world!")
}
```
Note, now this is now just a standard go file, keeping the go tools and the IDEs happy.

## Features
gorun will:

  * write files under a safe directory (e.g. /tmp), so that the actual script location isn't touched (may be read-only)
  * avoid races between parallel executions of the same file
  * automatically clean up old compiled files that remain unused for some time
  * replace the process rather than using a child
  * pass arguments to the compiled application properly
  * handle well GOROOT, GOROOT_FINAL and the location of the toolchain
  * support embedded go.mod, go.sum and environment variables used for compiling to ensure a repeatable build
  * support more complex projects with multiple source files, all under a common executable directory (e.g. /usr/local/bin)

## Is it slow?
No, it's not, thanks to the Go (gc) compiler suite, which compiles code surprisingly fast.

Here is a trivial/non-scientific comparison with Python:

```
$ time ./gorun hello.go
Hello world!
./gorun hello.go  0.03s user 0.00s system 74% cpu 0.040 total

$ time ./gorun hello.go
Hello world!
./gorun hello.go  0.00s user 0.00s system 0% cpu 0.003 total

$ time python -c 'print "Hello world!"'                                                        
Hello world!
python -c 'print "Hello world!"'  0.01s user 0.00s system 63% cpu 0.016 total

$ time python -c 'print "Hello world!"'
Hello world!
python -c 'print "Hello world!"'  0.00s user 0.01s system 64% cpu 0.016 total
```

Note how the second run is significantly faster than the first one. This happens because a cached version of the file is used after the first compilation.

gorun will correctly recompile the file whenever necessary.

## Where are the compiled files kept?
By default they are kept under /tmp/gorun-<HOST>-<UID>, a directory named after the hostname and user id executing the file.

You can remove these files, but there's no reason to do this. These compiled files will be garbage collected by gorun itself after a while once they stop being used.

## How to build and install gorun from source
Use ```go get``` as usual, or clone and ```go build .```

## Example usage
We store go "scripts" in a configuration management repo that is deployed to VMs as required directly in to
/usr/local/bin/scriptA.go, scriptB.go etc. That way the scripts can be inspected and, in a pinch, changed on the VM
and rerun just as python or bash would be.

To support this, we rely upon:

    * Support for making executable scripts run automatically via the Linux kernel (see below)
    * Repeatable builds. The same dependencies delivered to all VMs regardless of when it is built (see below)

### Executable
There are multiple ways of making the "script" executable. The simplest is to add ```#!/usr/bin/env gorun``` to the
top of the file

It is convenient to not have to have a shebang at the top of the file (it doesn't compile!). If running on Linux,
binfmt_misc can be used to instruct the kernel how to deal with executable programs - see [gorun-register.sh](./example/linux/usr/local/bin/gorun-register.sh)
This allows the file to just be a standard go file (no shebang) or to have a special first line comment.

The first line comment of "///bin/env gorun" is useful where the script file name cannot end in ".go", e.g.
     
 ```go
     cat > netdata.plugin << EOF
     ///bin/env gorun
     package main
     func main() {
         println("Hello world!")
     }
     EOF
     chmod a+x netdata.plugin
     ./netdata.plugin
 ```

### Repeatable builds
To protect against changing/different dependencies compiled with the script, it supports embedding 
go.mod, go.sum contents and environment variables in the file as a comment. Fictitious example:
    
    // go.mod >>>
    // module github.com/a/b
    // go 1.18
    // require github.com/c/d v0.0.0-20200225084820-12345affa
    // require mycompany.com/e/f v0.0.0-20200225084120-1849135
    // <<< go.mod
    //
    // go.env >>>
    // GOPRIVATE=mycompany.com
    // <<< go.env
    //
    // go.sum >>>
    // github.com/c v0.0.0-20190308221718-c2843e01d9a2/go.mod h1:djNgcEr1/C05ACkg1iLfiJU5Ep61QUkGW8qpdssI0+w=
    // <<< go.sum
    
    package main
    
    import (
    ...

Note that the go.env environment variables are passed to go build at compile time. That allows in the example
above for GOPRIVATE or other such dependency management options to be set before compilation.

### Way of working

The scripts can be organised in a repo in a directory each, with a [Makefile](example/linux/home/user/Makefile) at
the top level that will automatically extract the go.mod, go.sum and optional go.work/go.work.sum files the first time
it is run, and thereafter take the changes from the filesystem. That way there is only one checked in version of
dependency versions - in the comment at the top of the script.

The individual script files (not the go.mod and go.sum files) and any "extra" source directory can then be deployed to
a single directory already on the PATH, e.g. /usr/local/bin

## Extra source directory/files

gorun supports including any extra source files when the "script" grows a little too large for a single file.

Place any extra source files in a directory with the same name as the source .go, replacing ".go" with "_". e.g.

    ./httpServe.go
    ./httpServe_/net/auth.go
    ./httpServe_/net/reply.go
    ./httpServe_/db/sql.go

Then import "httpServe/httpServe_/net" in httpServe.go etc.

## go.work and "shared libraries"

It is handy to share code between multiple different scripts, and have that shared code in source form that is compiled
at run time too.

To do that, go.work files can be added to the scripts that references the desired ../sharedLibrary. See the
[myScriptUsingLibrary1](example/linux/home/user/myScriptUsingLibrary1) example

## Gotchas

1. To run a script as nobody, normally go build would fail as it couldn't download its dependencies etc. without a valid
$HOME. This is checked for and HOME is set to a per user run directory (by default under /tmp). This does mean that any
time the script needs compiled then it will download all dependencies again, and delete them straight after the build.

## License

gorun is licensed under Apache License Version 2.0.

This document is licensed under Creative Commons Attribution-ShareAlike 3.0 License