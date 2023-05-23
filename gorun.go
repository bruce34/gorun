//
// go.mod >>>
// module gorun
// go 1.18
// require golang.org/x/mod v0.10.0
// <<< go.mod

// go.sum >>>
// golang.org/x/mod v0.10.0 h1:lFO9qtOdlre5W1jxS3r/4szv2/6iXxScdzjoBMXNhYk=
// golang.org/x/mod v0.10.0/go.mod h1:iBbtSCu2XBx23ZKBPSOrRkjjQPZFPuis4dIYUhu/chs=
// <<< go.sum

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"golang.org/x/mod/modfile"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// BuildInfoString returns the build information stored within the compiled binary, git sha etc.
func BuildInfoString() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.String()
	}
	return "(unknown)"
}

func Usage() {
	fmt.Fprintf(flag.CommandLine.Output(),
		flag.CommandLine.Name()+`: Compile and run a go "script" in a single command.

Options can be provided via GORUN_ARGS environment variable, or on the command line.
If there exists a directory of the same base name as the .go file, plus a trailing '_', that
too will be copied and included in the build of the go program.

`)
	fmt.Fprintf(flag.CommandLine.Output(), "%s [options] <sourceFile.go>:\n", flag.CommandLine.Name())
	flag.PrintDefaults()
}

const (
	GOMOD     = "go.mod"
	GOSUM     = "go.sum"
	GOWORK    = "go.work"
	GOWORKSUM = "go.work.sum"
)

type Script struct {
	debug             bool     // more output, don't delete temporary files
	content           []byte   // contents of the primary script.go file
	scriptPath        string   // full path to the primary script.go file
	scriptExtraDir    string   // full path to any extra script dir
	scriptRelWorkDirs []string // path to any local referenced (../* only) go.work directories
	scriptWorkDirs    []string // path to any local referenced (../* only) go.work directories, full path
	args              []string
	// where to write subdirectories to
	tmpDirBase string
	// subdirectory containing all this user's commands (a sub of tmpDirBase)
	perUserTmpDir string
	// subdirectory containing this user's version of the command (a sub of perUserTmpDir)
	tmpDir string
	// per PID version of this user's version of the command (deleted after build)
	perRunTmpDirBase string
	// copy everything down to a completely unique tmp directory and delete it afterwards
	perRunTmpDir string
	// moving the binary to <tmpDir>/script.go.bin just before
	// compiled binary final resting place, lives under tmpDir
	binary string
	// file showing the binary was run lately (for filesystems not running atime)
	binaryLastRun string
	// any binaries not accessed within this number of seconds get deleted (and rebuilt)
	cleanSecs int64
}

// realPath returns the real absolute path, resolving symlinks
func realPath(sourceFile string) (realPath string, err error) {
	sourceFile, err = filepath.Abs(sourceFile)
	if err != nil {
		return
	}
	realPath, err = filepath.EvalSymlinks(sourceFile)
	return
}

func main() {
	flag.Usage = Usage

	// gather all args, command line and GORUN_ARGS in to one array
	gorunArgsEnv, _ := os.LookupEnv("GORUN_ARGS")
	gorunArgs := strings.Fields(gorunArgsEnv)
	args := append(gorunArgs, os.Args[1:]...)

	var diff, embed, extract, extractIfMissing, version bool
	var cleanDays int64

	s := Script{}

	flag.Int64Var(&cleanDays, "cleanDays", 7, "clean all binaries from this user older than N days")
	flag.BoolVar(&diff, "diff", false, "show diff between embedded comments and filesystem go.mod/go.sum/go.work/go.work.sum")
	flag.BoolVar(&embed, "embed", false, "embed filesystem go.mod/go.sum/go.work/go.work.sum as comments in source file")
	flag.BoolVar(&extract, "extract", false, "extract the comments to filesystem go.mod/go.sum/go.work/go.work.sum")
	flag.BoolVar(&extractIfMissing, "extractIfMissing", false, "extract the comments to filesystem go.mod/go.sum/go.work/go.work.sum only if BOTH files do not exist on disc")
	flag.BoolVar(&s.debug, "debug", false, "provide more debug, don't delete temporary files under /tmp")
	flag.StringVar(&s.tmpDirBase, "targetDirBase", "/tmp", "directory to copy script and extract go.mod etc. to before building")
	flag.BoolVar(&version, "version", false, "Print version info and exit")
	flag.CommandLine.Parse(args)

	if version {
		fmt.Printf("BuildInfo: %v\n", BuildInfoString())
		os.Exit(0)
	}

	if len(args) == flag.NFlag() {
		Usage()
		os.Exit(1)
	}

	s.args = flag.Args()
	s.cleanSecs = cleanDays * 24 * 3600

	sourceFile, err := realPath(flag.Arg(0))
	if err != nil {
		fmt.Printf("Failed to find source file %v\n", err.Error())
		return
	}
	s.scriptPath = sourceFile

	if diff {
		err = s.diffEmbedded()
	} else if extract {
		err = s.extractEmbedded()
	} else if extractIfMissing {
		err = s.extractIfMissingEmbedded()
	} else if embed {
		err = s.embedEmbedded()
	} else {
		err = s.runScript()
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error: "+err.Error())
		os.Exit(1)
	}
}

// initVars fills in commonly used variables (paths), e.g. what is the path to the script binary.
// It reads the contents of the go script, to be able to extract the go.work section and also allow
// any go.work "shared libraries" to be copied over to the temporary build area too.
func (s *Script) initVars() (err error) {
	hostname, err := os.Hostname()
	if err != nil {
		return
	}
	s.content, err = os.ReadFile(s.scriptPath)
	if err != nil {
		return
	}

	perUserTmpDir := fmt.Sprintf("gorun-%v-%v", hostname, os.Getuid())
	tmpDir := filepath.Join(perUserTmpDir,
		strings.ReplaceAll(s.scriptPath, string(filepath.Separator), "_"))

	s.perUserTmpDir = filepath.Join(s.tmpDirBase, perUserTmpDir)
	s.tmpDir = filepath.Join(s.tmpDirBase, tmpDir)
	if strings.HasSuffix(s.scriptPath, ".go") {
		s.scriptExtraDir = s.scriptPath[:len(s.scriptPath)-3] + "_"
	} else {
		s.scriptExtraDir = s.scriptPath + "_"
	}
	fileinfo, err := os.Stat(s.scriptExtraDir)
	if err != nil || !fileinfo.IsDir() {
		err = nil
		s.scriptExtraDir = ""
	}
	s.perRunTmpDirBase = filepath.Join(s.tmpDir, strconv.Itoa(os.Getpid()))
	s.perRunTmpDir = filepath.Join(s.perRunTmpDirBase, filepath.Dir(s.scriptPath))
	s.binary = filepath.Join(s.tmpDir, filepath.Base(s.scriptPath)+".bin")
	s.binaryLastRun = filepath.Join(s.tmpDir, ".lastRun")

	// deal with a go.work file
	gowork := getSection(s.content, GOWORK)

	// for each WorkFile.Path, we want to ignore ./* and copy any ../.* across
	wf, err := modfile.ParseWork(GOWORK, gowork, nil)
	for _, w := range wf.Use {
		if strings.HasPrefix(w.Path, "../") {
			s.scriptRelWorkDirs = append(s.scriptRelWorkDirs, w.Path)
			s.scriptWorkDirs = append(s.scriptWorkDirs, filepath.Join(filepath.Dir(s.scriptPath), w.Path))
		}
	}

	return
}

// simplistic copy files from one directory to another, deleting files that no longer exist
// given /tmp/path/<goscript>_ directory as dstDir and /path/<goscript>_ directory as srcDir
func copyDir(dstDir string, srcDir string) (err error) {
	err = filepath.Walk(srcDir, func(srcPath string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(srcDir, srcPath)
		switch f.Mode() & os.ModeType {
		case 0: // Regular file
			content, err := os.ReadFile(srcPath)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to read (while copying) "+relPath+" to "+dstDir)
				return err
			}
			err = os.WriteFile(filepath.Join(dstDir, relPath), content, 0600)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to write (while copying) "+relPath+" to "+dstDir)
				return err
			}
		case os.ModeDir:
			os.Mkdir(filepath.Join(dstDir, relPath), 0700)
			return nil
		default:
			return fmt.Errorf("We only handle regular files, not %s", relPath)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return
}

// writeFileFromCommentsOrDir uses either the parsed commented section or the file on disc and copies it to the target dir
func (s *Script) writeFileFromCommentsOrDir(content []byte, sectionName string) (err error) {
	file := filepath.Join(s.perRunTmpDir, sectionName)
	written, err := writeFileFromComments(content, sectionName, file)
	if err != nil {
		return
	}
	if !written {
		_ = copyDir(filepath.Join(s.perRunTmpDir, sectionName), filepath.Join(filepath.Dir(s.scriptPath), sectionName))
	}
	return
}

// updateTarget copies all needed files to build the script binary to the target area
func (s *Script) updateTarget() (err error) {
	os.RemoveAll(s.perRunTmpDirBase) // just in case it still exists
	os.MkdirAll(s.perRunTmpDirBase, 0700)

	checkDirs := []string{}
	if s.scriptExtraDir != "" {
		checkDirs = append(checkDirs, s.scriptExtraDir)
	}
	checkDirs = append(checkDirs, s.scriptWorkDirs...)
	for _, dir := range checkDirs {
		src := dir
		dest := filepath.Join(s.perRunTmpDirBase, dir)
		os.MkdirAll(dest, 0700)
		err = copyDir(dest, src)
		if err != nil {
			fmt.Printf("%v\n", err.Error())
		}
	}

	// The go script must be made to end in ".go" to allow go build to work with it
	dstScriptPath := filepath.Join(s.perRunTmpDir, filepath.Base(s.scriptPath))
	if !strings.HasSuffix(s.scriptPath, ".go") {
		dstScriptPath += ".go"
	}

	if len(s.content) > 2 && s.content[0] == '#' && s.content[1] == '!' {
		s.content[0] = '/'
		s.content[1] = '/'
	}
	os.MkdirAll(filepath.Dir(dstScriptPath), 0700)
	err = os.WriteFile(dstScriptPath, s.content, 0600)
	if err != nil {
		return
	}

	// Write a go.mod file from inside the comments
	err = s.writeFileFromCommentsOrDir(s.content, GOMOD)
	if err != nil {
		return
	}

	// Write a go.sum file from inside the comments
	err = s.writeFileFromCommentsOrDir(s.content, GOSUM)
	if err != nil {
		return
	}

	// Write a go.sum file from inside the comments
	err = s.writeFileFromCommentsOrDir(s.content, GOWORK)
	if err != nil {
		return
	}

	// Write a go.sum file from inside the comments
	err = s.writeFileFromCommentsOrDir(s.content, GOWORKSUM)
	return
}

// run a command sending its output to stderr,stdout directly. Not used to run the script
func runCommand(dir string, env []string, command string, args ...string) (err error) {
	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = dir
	cmd.Env = env
	err = cmd.Run()
	if err != nil {
		fmt.Printf("Run command %v %v failed with %s\n", command, args, err)
	}
	return
}

func getEnvVar(env []string, key string) string {
	for _, line := range env {
		if strings.HasPrefix(line, key+"=") {
			return strings.SplitAfterN(line, key+"=", 2)[1]
		}
	}
	return ""
}

// compile copies the script and its dependencies to a "per run" tmp directory and compiles it there.
// The binary is kept, but the "per run" tmp directory is removed at the end
func (s *Script) compile() (err error) {
	if !s.debug {
		defer os.RemoveAll(s.perRunTmpDirBase)
	}
	err = s.updateTarget()
	if err != nil {
		return
	}

	// use the default environment before adding our overrides, this allows GOPRIVATE etc. to be used in the build
	var env []string
	section := getSection(s.content, "go.env")
	env = os.Environ()
	if len(section) > 0 {
		env = append(env, strings.Split(string(section), "\n")...)
	}

	// if $HOME/.cache can't be built and $GOCACHE is not set, then use a temp home dir
	if getEnvVar(env, "GOCACHE") == "" {
		home := getEnvVar(env, "HOME")
		if home == "" || home == "/" {
			env = append(env, "HOME="+s.perRunTmpDir)
		} else if _, err := os.Stat(filepath.Join(home, ".cache")); os.IsNotExist(err) {
			err = os.Mkdir(filepath.Join(home, ".cache"), 0755)
			if err != nil && !os.IsExist(err) {
				// unable to create the .cache directory - give this process a temp home (env will likely contain HOME twice)
				env = append(env, "HOME="+s.perRunTmpDir)
			}
		}
	}

	// find the go binary to call via env var, std location, or the PATH
	gobin := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(gobin); err != nil {
		if gobin, err = exec.LookPath("go"); err != nil {
			return errors.New("can't find go tool")
		}
	}

	out := filepath.Join(s.perRunTmpDir, filepath.Base(s.scriptPath)+".bin")

	err = runCommand(s.perRunTmpDir, env,
		gobin, "build", "-o", out, ".")
	if err != nil {
		return err
	}
	err = os.Rename(out, s.binary)
	// os.RemoveAll mode 444 files (from go build cache being here when no HOME dir set) on Unix don't allow unlink
	// so let's chmod all files/dirs to allow the deferred RemoveAll to work
	_ = filepath.Walk(s.perRunTmpDirBase, func(name string, info os.FileInfo, err error) error {
		if err == nil {
			err = os.Chmod(name, 0755)
		}
		return err
	})
	return
}

func touchFile(file string) (err error) {
	_, err = os.Stat(file)
	if os.IsNotExist(err) {
		var f *os.File
		f, err = os.Create(file)
		defer f.Close()
	} else {
		currentTime := time.Now().Local()
		err = os.Chtimes(file, currentTime, currentTime)
	}
	return
}

// run runs the binary and marks when it was last run (by touching a file) if set to clean up other scripts by the same user
func (s *Script) run() (err error) {
	if s.cleanSecs >= 0 {
		_ = touchFile(s.binaryLastRun)
	}
	err = syscall.Exec(s.binary, s.args, os.Environ())
	return
}

// remove binaries that haven't been accessed for a while.
// Check a file in each directory to see when it was last touched (last run)
func (s *Script) clean() (err error) {
	perUserDir, err := os.Open(s.perUserTmpDir)
	if err != nil {
		return
	}
	infos, err := perUserDir.Readdir(-1)
	if err != nil {
		return
	}
	cutoffTime := time.Now().Add(time.Duration(-s.cleanSecs) * time.Second)
	for _, info := range infos {
		if info.IsDir() {
			st, err := os.Stat(filepath.Join(s.perUserTmpDir, info.Name(), ".lastRun"))
			if !os.IsNotExist(err) && st.ModTime().Before(cutoffTime) {
				os.RemoveAll(filepath.Join(s.perUserTmpDir, info.Name()))
			}
		}
	}
	return nil
}

// targetOutOfDate decides whether the target needs recompiled, i.e. is the source newer than the binary.
func (s *Script) targetOutOfDate() (outOfDate bool, err error) {
	oldestSrcInfo, err := os.Stat(s.scriptPath)
	if err != nil {
		return
	}
	// if we have any extra source directories, check whether any are newer than the binary.
	checkDirs := []string{}
	if s.scriptExtraDir != "" {
		checkDirs = append(checkDirs, s.scriptExtraDir)
	}
	checkDirs = append(checkDirs, s.scriptWorkDirs...)
	for _, checkDir := range checkDirs {
		filepath.Walk(checkDir, func(path string, info os.FileInfo, err error) error {
			if info.ModTime().After(oldestSrcInfo.ModTime()) {
				oldestSrcInfo = info
			}
			return nil
		})
	}
	binaryInfo, err := os.Stat(s.binary)
	outOfDate = err != nil || binaryInfo.IsDir() || binaryInfo.ModTime().Before(oldestSrcInfo.ModTime())
	return outOfDate, nil
}

// runScript compiles if required, and then runs the binary created from the script
func (s *Script) runScript() (err error) {
	err = s.initVars()

	if s.cleanSecs >= 0 {
		s.clean()
	}
	if err != nil {
		return
	}
	// TODO if go.mod exists and --autoEmbed set
	// we could be getting called multiple times simultaneously, with source code changing under
	// our feet too. We could also get our directory deleted entirely from under us as part of
	// a clean up, so let's try multiple times
	var outOfDate bool
	for i := 0; i < 5; i++ {
		outOfDate, err = s.targetOutOfDate()

		if err != nil {
			return // can't find the source file - let's bail
		}
		if outOfDate {
			err = s.compile() // can't compile, well, it could be inconsistent source, let's bail
			if err != nil {
				return
			}
		}
		err = s.run()
		if !os.IsNotExist(err) {
			break // we ran, must be a real error
		}
	}
	return
}

//
// Helpers to embed, extract and diff go.mod/go.sum files between filesystem and comments within the script.
//

// loadFile loads a file from disc, removing extra new lines and spaces
func loadFile(filename string) (found bool, content []byte, err error) {
	_, err = os.Stat(filename)
	if err != nil {
		return false, nil, nil // no error if file not there
	}
	content, err = os.ReadFile(filename)
	if err != nil {
		return // error if file there but can't be read
	}
	found = true
	// get rid of extra new lines and whitespace
	content = bytes.TrimSpace(content)
	content = bytes.Replace(content, []byte("\n\n"), []byte("\n"), -1)
	return
}

func diffBytes(content []byte, dir string, sectionName string) (diff string, err error) {
	section := getSection(content, sectionName)
	section = bytes.TrimSpace(section)
	section = bytes.Replace(section, []byte("\n\n"), []byte("\n"), -1)

	foundOnDisc, sectionFromFile, err := loadFile(filepath.Join(dir, sectionName))
	if err != nil { // file exists but unable to read
		return
	}
	if !foundOnDisc && len(section) == 0 {
		_, _ = fmt.Fprintf(os.Stderr, "OK: section %q not embedded or on disc\n", sectionName)
		return "", nil
	}
	if !foundOnDisc {
		_, _ = fmt.Fprintf(os.Stderr, "WARN: embedded %q exists but nothing on disc\n", sectionName)
		return "embeddedExists", nil
	}
	if len(section) == 0 && len(sectionFromFile) > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "WARN: on disc %q exists but embedded doesn't\n", sectionName)
		return "discExists", nil
	}
	if bytes.Equal(sectionFromFile, section) {
		_, _ = fmt.Fprintf(os.Stderr, "OK: embedded %q exists and same as on disc\n", sectionName)
		return "", nil
	}
	_, _ = fmt.Fprintf(os.Stderr, "WARN: embedded %q exists and different to on disc\n", sectionName)
	return "diff", nil
}

func (s *Script) diffEmbedded() (err error) {
	content, err := os.ReadFile(s.scriptPath)
	if err != nil {
		return
	}
	diff1, err := diffBytes(content, filepath.Dir(s.scriptPath), GOMOD)
	if err != nil {
		return
	}
	diff2, err := diffBytes(content, filepath.Dir(s.scriptPath), GOSUM)
	if err != nil {
		return
	}
	diff3, err := diffBytes(content, filepath.Dir(s.scriptPath), GOWORK)
	if err != nil {
		return
	}
	diff4, err := diffBytes(content, filepath.Dir(s.scriptPath), GOWORKSUM)
	if err != nil {
		return
	}

	if diff1 != "" || diff2 != "" || diff3 != "" || diff4 != "" {
		_, _ = fmt.Fprintln(os.Stderr, "Diffs found\n")
		os.Exit(1)
	}
	return
}

func (s *Script) extractEmbedded() (err error) {
	content, err := os.ReadFile(s.scriptPath)
	if err != nil {
		return
	}
	_, err = writeFileFromComments(content, GOSUM, filepath.Join(filepath.Dir(s.scriptPath), GOSUM))

	if err != nil {
		return
	}
	_, err = writeFileFromComments(content, GOMOD, filepath.Join(filepath.Dir(s.scriptPath), GOMOD))
	if err != nil {
		return
	}
	if len(getSection(content, GOWORK)) != 0 {
		_, err = writeFileFromComments(content, GOWORK, filepath.Join(filepath.Dir(s.scriptPath), GOWORK))
		if err != nil {
			return
		}
	}
	if len(getSection(content, GOWORKSUM)) != 0 {
		_, err = writeFileFromComments(content, GOWORKSUM, filepath.Join(filepath.Dir(s.scriptPath), GOWORKSUM))
		if err != nil {
			return
		}
	}
	return
}

func commentSection(content []byte, header string, trailer string) (commented []byte) {
	commented = bytes.ReplaceAll(content, []byte("\n"), []byte("\n// "))
	commented = append(commented, []byte("\n")...)
	commented = append([]byte("// "), commented...)
	commented = append([]byte(header), commented...)
	commented = append(commented, []byte(trailer)...)
	return
}

// header transforms a section name, e.g. 'go.mod' in to a header comment, e.g. '// go.mod >>>\n'
func header(section string) (header string) {
	return "// " + section + " >>>\n"
}

// trailer transforms a section name, e.g. 'go.mod' in to a trailer comment, e.g. '// <<< go.mod\n'
func trailer(section string) (trailer string) {
	return "// <<< " + section + "\n"
}

// extract the files go.sum, go.mod from the comments at the top of the script and put on disc
// ONLY if they both don't already exist on disc.
func (s *Script) extractIfMissingEmbedded() (err error) {
	foundSumOnDisc, _, err := loadFile(filepath.Join(filepath.Dir(s.scriptPath), GOSUM))
	if err != nil {
		return
	}
	foundModOnDisc, _, err := loadFile(filepath.Join(filepath.Dir(s.scriptPath), GOMOD))
	if err != nil {
		return
	}
	foundWorkOnDisc, _, err := loadFile(filepath.Join(filepath.Dir(s.scriptPath), GOWORK))
	if err != nil {
		return
	}
	foundWorkSumOnDisc, _, err := loadFile(filepath.Join(filepath.Dir(s.scriptPath), GOWORKSUM))
	if err != nil {
		return
	}

	if !foundModOnDisc && !foundSumOnDisc && !foundWorkOnDisc && !foundWorkSumOnDisc {
		s.extractEmbedded()
	}
	return
}

// embed the files go.sum, go.mod in the comments at the top of the script (go.work is optional)
func (s *Script) embedEmbedded() (err error) {
	content, err := os.ReadFile(s.scriptPath)
	if err != nil {
		return
	}
	foundSumOnDisc, sumContent, err := loadFile(filepath.Join(filepath.Dir(s.scriptPath), GOSUM))
	if err != nil {
		return
	}
	foundModOnDisc, modContent, err := loadFile(filepath.Join(filepath.Dir(s.scriptPath), GOMOD))
	if err != nil {
		return
	}
	foundWorkOnDisc, workContent, _ := loadFile(filepath.Join(filepath.Dir(s.scriptPath), GOWORK))
	foundWorkSumOnDisc, workSumContent, _ := loadFile(filepath.Join(filepath.Dir(s.scriptPath), GOWORKSUM))

	// let's only delete an embedded section if there is a section file (e.g. go.sum) on disc alongside
	if foundModOnDisc {
		_, content = embedSection(content, modContent, GOMOD, []string{})
	}

	if foundSumOnDisc {
		_, content = embedSection(content, sumContent, GOSUM, []string{GOMOD})
	}

	if foundWorkOnDisc {
		_, content = embedSection(content, workContent, GOWORK, []string{GOMOD, GOSUM})
	}

	if foundWorkSumOnDisc {
		_, content = embedSection(content, workSumContent, GOWORKSUM, []string{GOMOD, GOSUM, GOWORK})
	}

	err = os.WriteFile(s.scriptPath, content, 0600)
	return
}

// replace a commented section of bytes with another commented section of bytes, returning the new entire file contents
// return idx = -1 if not found
func embedSection(origContent []byte, sectionBytes []byte, section string, previousSections []string) (foundIdx int, content []byte) {
	addNewline := false
	// if we found the section, put the new one where the old one was
	foundIdx, content = removeSection(origContent, section)
	idx := foundIdx
	if foundIdx < 0 { // if we failed to find the section, place it after any sections we want before it
		idx = 0
		for _, prevSection := range previousSections {
			found, _, _, _, foundIdx := sectionIndexes(content, prevSection)
			if found && foundIdx > idx {
				idx = foundIdx
				addNewline = true
			}
		}
	}
	var contentStart, contentTrailer []byte

	contentStart = append(contentStart, content[0:idx]...)
	// only add a newline between sections go.sum and go.mod sections if we've added a new (e.g. go.sum) section
	// after an existing section (e.g. go.mod), otherwise leave it as the user had it
	if addNewline {
		contentStart = append(contentStart, []byte("\n")...)
	}
	contentTrailer = append(contentTrailer, content[idx:]...)
	content = append(contentStart, commentSection(sectionBytes, header(section), trailer(section))...)
	content = append(content, contentTrailer...)
	return
}

// sectionIndexes returns whether a section is found and if so, the indexes of start, end, etc.
// found true iff a section called sectionName is found
// startIdx is the first byte of the header for this section
// endIdx is the byte after the trailer for this section.
// InnerIdx mark the start and end of the real content for this section
func sectionIndexes(content []byte, sectionName string) (found bool, startIdx int, startInnerIdx int, endInnerIdx int, endIdx int) {
	start := header(sectionName)
	end := trailer(sectionName)
	startIdx = bytes.Index(content, []byte(start))
	startInnerIdx = startIdx + len(start)
	endInnerIdx = bytes.Index(content, []byte(end))
	endIdx = endInnerIdx + len(end)
	found = startIdx >= 0 && endIdx > startIdx
	return
}

// getSection finds, removes comments, and returns the comment section embedded in a file, or empty if not found
func getSection(content []byte, sectionName string) (section []byte) {
	found, _, startInnerIdx, endInnerIdx, _ := sectionIndexes(content, sectionName)
	if found {
		goMod := "\n" + string(content[startInnerIdx:endInnerIdx])
		goMod = strings.ReplaceAll(goMod, "\n// ", "\n")
		goMod = strings.ReplaceAll(goMod, "\n//", "\n")
		return []byte(goMod)
	}
	return []byte("")
}

// removeSection removes a commented section from the contents of the entire file, returning the new contents and where it was removed from
func removeSection(content []byte, sectionName string) (startIdx int, newContent []byte) {
	found, startIdx, _, _, endIdx := sectionIndexes(content, sectionName)
	if found {
		newContent = content[0:startIdx]
		newContent = append(newContent, content[endIdx:]...)
	} else {
		newContent = content
	}
	return
}

// writeFileFromComments write out a particular commented section of a goscript file to a file
func writeFileFromComments(content []byte, sectionName string, file string) (written bool, err error) {
	// Write a go.mod file from inside the comments
	section := getSection(content, sectionName)
	if len(section) > 0 {
		err = os.WriteFile(file, section, 0600)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to write "+sectionName+" to "+file)
			return
		}
		written = true
	}
	return
}
