// Copyright 2010 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/bpowers/dynamo/dynamo"
	"go/ast"
	"go/format"
	"go/token"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"text/template"
)

var (
	httpListen = flag.String("http", "127.0.0.1:3999", "host:port to listen on")
	htmlOutput = flag.Bool("html", false, "render program output as HTML")
)

var (
	// a source of numbers, for naming temporary files
	uniq = make(chan int)
)

func main() {
	flag.Parse()

	// source of unique numbers
	go func() {
		for i := 0; ; i++ {
			uniq <- i
		}
	}()

	http.HandleFunc("/", FrontPage)
	http.HandleFunc("/compile", Compile)
	log.Fatal(http.ListenAndServe(*httpListen, nil))
}

// FrontPage is an HTTP handler that renders the goplay interface.
// If a filename is supplied in the path component of the URI,
// its contents will be put in the interface's text area.
// Otherwise, the default "hello, world" program is displayed.
func FrontPage(w http.ResponseWriter, req *http.Request) {
	data, err := ioutil.ReadFile(req.URL.Path[1:])
	if err != nil {
		data = helloWorld
	}
	frontPage.Execute(w, data)
}

// Compile is an HTTP handler that reads Go source code from the request,
// runs the program (returning any errors),
// and sends the program's output as the HTTP response.
func Compile(w http.ResponseWriter, req *http.Request) {
	out, err := compile(req)
	if err != nil {
		error_(w, out, err)
		return
	}

	// write the output of x as the http response
	if *htmlOutput {
		w.Write(out)
	} else {
		output.Execute(w, out)
	}
}

var (
	commentRe = regexp.MustCompile(`(?m)^#.*\n`)
	tmpdir    string
)

func init() {
	// find real temporary directory (for rewriting filename in output)
	var err error
	tmpdir, err = filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		log.Fatal(err)
	}
}

// gofmt takes the given, valid, Go AST and returns a
// canonically-formatted go program in a byte-array, or an error.
func gofmt(f *ast.File) ([]byte, error) {
	fset := token.NewFileSet()
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// transliterate takes an input stream and a name and returns a byte
// buffer containing valid & gofmt'ed source code, or an error.  The
// name is used purely for diagnostic purposes
func transliterate(name string, in io.Reader) ([]byte, error) {
	fset := token.NewFileSet()

	// dump in the file
	mdlSrc, err := ioutil.ReadAll(in)
	if err != nil {
		return nil, fmt.Errorf("ReadAll(%v): %s", in, err)
	}

	fsetFile := fset.AddFile(name, fset.Base(), len(mdlSrc))

	// and parse
	pkg, err := dynamo.Parse(fsetFile, fset, string(mdlSrc))
	if err != nil {
		return nil, fmt.Errorf("Parse(%v): %s", name, err)
	}
	if pkg.NErrors > 0 {
		return nil, fmt.Errorf("There were errors parsing the file")
	}

	goSource, err := dynamo.GenGo(pkg)
	if err != nil {
		log.Fatalf("GenGo(%v): %s", pkg, err)
	}

	src, err := gofmt(goSource)
	if err != nil {
		log.Fatalf("gofmtFile(%v): %s", goSource, err)
	}
	return src, nil
}

func compile(req *http.Request) (out []byte, err error) {
	// x is the base name for .go, .6, executable files
	x := filepath.Join(tmpdir, "compile"+strconv.Itoa(<-uniq))
	src := x + ".go"
	bin := x
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	// rewrite filename in error output
	defer func() {
		if err != nil {
			// drop messages from the go tool like '# _/compile0'
			out = commentRe.ReplaceAll(out, nil)
		}
		out = bytes.Replace(out, []byte(src+":"), []byte("main.go:"), -1)
	}()

	// write body to x.go
	body := new(bytes.Buffer)
	if _, err = body.ReadFrom(req.Body); err != nil {
		return
	}
	defer os.Remove(src)

	goBody, err := transliterate("<web>", body)
	if err != nil {
		return nil, err
	}

	if err = ioutil.WriteFile(src, goBody, 0666); err != nil {
		return
	}

	// build x.go, creating x
	dir, file := filepath.Split(src)
	out, err = run(dir, "go", "build", "-o", bin, file)
	defer os.Remove(bin)
	if err != nil {
		return
	}

	// run x
	return run("", bin)
}

// error writes compile, link, or runtime errors to the HTTP connection.
// The JavaScript interface uses the 404 status code to identify the error.
func error_(w http.ResponseWriter, out []byte, err error) {
	w.WriteHeader(404)
	if out != nil {
		output.Execute(w, out)
	} else {
		output.Execute(w, err.Error())
	}
}

// run executes the specified command and returns its output and an error.
func run(dir string, args ...string) ([]byte, error) {
	var buf bytes.Buffer
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Stdout = &buf
	cmd.Stderr = cmd.Stdout
	err := cmd.Run()
	return buf.Bytes(), err
}

var frontPage = template.Must(template.New("frontPage").Parse(frontPageText)) // HTML template
var output = template.Must(template.New("output").Parse(outputText))          // HTML template

var outputText = `<pre>{{printf "%s" . |html}}</pre>`

var frontPageText = `<!doctype html>
<html>
<head>
<style>
pre, textarea {
	font-family: Monaco, 'Courier New', 'DejaVu Sans Mono', 'Bitstream Vera Sans Mono', monospace;
	font-size: 100%;
}
.hints {
	font-size: 0.8em;
	text-align: right;
}
#edit, #output, #errors { width: 100%; text-align: left; }
#edit { height: 500px; }
#output { color: #00c; }
#errors { color: #c00; }
</style>
<script>

function insertTabs(n) {
	// find the selection start and end
	var cont  = document.getElementById("edit");
	var start = cont.selectionStart;
	var end   = cont.selectionEnd;
	// split the textarea content into two, and insert n tabs
	var v = cont.value;
	var u = v.substr(0, start);
	for (var i=0; i<n; i++) {
		u += "\t";
	}
	u += v.substr(end);
	// set revised content
	cont.value = u;
	// reset caret position after inserted tabs
	cont.selectionStart = start+n;
	cont.selectionEnd = start+n;
}

function autoindent(el) {
	var curpos = el.selectionStart;
	var tabs = 0;
	while (curpos > 0) {
		curpos--;
		if (el.value[curpos] == "\t") {
			tabs++;
		} else if (tabs > 0 || el.value[curpos] == "\n") {
			break;
		}
	}
	setTimeout(function() {
		insertTabs(tabs);
	}, 1);
}

function preventDefault(e) {
	if (e.preventDefault) {
		e.preventDefault();
	} else {
		e.cancelBubble = true;
	}
}

function keyHandler(event) {
	var e = window.event || event;
	if (e.keyCode == 9) { // tab
		insertTabs(1);
		preventDefault(e);
		return false;
	}
	if (e.keyCode == 13) { // enter
		if (e.shiftKey) { // +shift
			compile(e.target);
			preventDefault(e);
			return false;
		} else {
			autoindent(e.target);
		}
	}
	return true;
}

var xmlreq;

function autocompile() {
	if(!document.getElementById("autocompile").checked) {
		return;
	}
	compile();
}

function compile() {
	var prog = document.getElementById("edit").value;
	var req = new XMLHttpRequest();
	xmlreq = req;
	req.onreadystatechange = compileUpdate;
	req.open("POST", "/compile", true);
	req.setRequestHeader("Content-Type", "text/plain; charset=utf-8");
	req.send(prog);	
}

function compileUpdate() {
	var req = xmlreq;
	if(!req || req.readyState != 4) {
		return;
	}
	if(req.status == 200) {
		document.getElementById("output").innerHTML = req.responseText;
		document.getElementById("errors").innerHTML = "";
	} else {
		document.getElementById("errors").innerHTML = req.responseText;
		document.getElementById("output").innerHTML = "";
	}
}
</script>
</head>
<body>
<table width="100%"><tr><td width="60%" valign="top">
<textarea autofocus="true" id="edit" spellcheck="false" onkeydown="keyHandler(event);" onkeyup="autocompile();">{{printf "%s" . |html}}</textarea>
<div class="hints">
(Shift-Enter to compile and run.)&nbsp;&nbsp;&nbsp;&nbsp;
<input type="checkbox" id="autocompile" value="checked" /> Compile and run after each keystroke
</div>
<td width="3%">
<td width="27%" align="right" valign="top">
<div id="output"></div>
</table>
<div id="errors"></div>
</body>
</html>
`

var helloWorld = []byte(`*
NOTE	House5 -- Three sector urban model with housing filter down
NOTE
NOTE	Population Sector
NOTE
L	POP.K=POP.J+(DT)(B.JK-D.JK+NM.JK+NM.JK-OM.JK)
N	POP=POPN
C	POPN=133000
R	B.KL=(NB)(POP.K)
C	ND=0.01
R	IM.KL=(IMN)(AM.K)(POP.K)
C	IMN=.01
A	AM.K=(AJM.K)(AHM.K)
A	AHM.K=TABHL(AHMT,HAR.K,.4,1.4,.2)
T	AHMT=2/2/1.6/1/.2/.005
A	AJM.K=TABHL(AJMT,LJR.K,.5,1.2,.1)
T	AJMT=2/2/1.87/1.6/1.25/1/.3/.05
R	OM.KL=(OMN)(DM.K)(POP.K)
C	OMN=.01
A	DM.K=MIN((1/OMN,(1/AM.K)))
`)
